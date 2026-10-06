package trainer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"roadpolice-web/internal/bank"
	"roadpolice-web/internal/store"
)

func newService(t *testing.T) *Service {
	t.Helper()
	data, err := os.ReadFile("../../../bank/bank.json")
	if os.IsNotExist(err) {
		t.Skip("банк не собран: скопируйте bank/ из roadpolice-trainer")
	}
	if err != nil {
		t.Fatalf("банк: %v", err)
	}
	b, err := bank.Load(data)
	if err != nil {
		t.Fatalf("банк: %v", err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("база: %v", err)
	}
	svc := New(b, st)
	t.Cleanup(func() { svc.Close() })
	return svc
}

func TestGetState(t *testing.T) {
	svc := newService(t)
	s, err := svc.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta.Total != 1050 || s.Meta.Exam.Questions != 20 || s.Meta.Exam.PassMinCorrect != 18 {
		t.Errorf("параметры экзамена: %+v", s.Meta)
	}
	if len(s.Groups) != 10 {
		t.Errorf("групп %d, ожидалось 10", len(s.Groups))
	}
	for _, g := range s.Groups {
		if g.Title == "" {
			t.Errorf("у группы %d нет названия", g.Group)
		}
	}
	if s.MistakeCount != 0 || s.ActiveExam != nil || s.ResolveAfter != store.ResolveStreak {
		t.Errorf("стартовое состояние: %+v", s)
	}
}

// clock — управляемые часы сервиса.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func withClock(svc *Service) *clock {
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	svc.now = c.now
	return c
}

func TestExamHidesAnswersAndGradesAll(t *testing.T) {
	svc := newService(t)
	clk := withClock(svc)
	ex, err := svc.StartExam()
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Questions) != 20 || ex.TimeMinutes != 30 || ex.RemainingMs != 30*60*1000 || ex.Lease == "" {
		t.Fatalf("билет: %d вопросов, %d минут, осталось %d мс, lease %q",
			len(ex.Questions), ex.TimeMinutes, ex.RemainingMs, ex.Lease)
	}
	for _, q := range ex.Questions {
		if q.Answer != 0 {
			t.Fatalf("%s: правильный ответ утёк во фронтенд", q.ID)
		}
	}

	// отвечаем: первые 18 верно, 19-й неверно, 20-й вообще не отвечаем
	var wrongID, skippedID string
	for i, q := range ex.Questions {
		full, _ := svc.bank.ByID(q.ID)
		pick := full.Answer
		switch {
		case i == 18:
			wrongID = q.ID
			pick = full.Answer%len(full.Options) + 1 // заведомо другой вариант
		case i == 19:
			skippedID = q.ID
			continue
		}
		if _, err := svc.AnswerExam(ex.ExamID, ex.Lease, q.ID, pick); err != nil {
			t.Fatal(err)
		}
	}

	clk.advance(15 * time.Minute)
	res, err := svc.FinishExam(ex.ExamID, ex.Lease)
	if err != nil {
		t.Fatal(err)
	}
	if res.Correct != 18 || !res.Passed || res.Total != 20 || res.TimedOut || res.ElapsedSec != 900 {
		t.Errorf("результат: %+v", res)
	}
	if len(res.Items) != 20 {
		t.Fatalf("в разборе %d вопросов, ожидалось 20", len(res.Items))
	}
	for _, it := range res.Items {
		if it.Question.Answer == 0 {
			t.Errorf("%s: в разборе нет правильного ответа", it.Question.ID)
		}
		if it.Question.ID == skippedID && (it.Chosen != 0 || it.Correct) {
			t.Errorf("пропущенный вопрос зачтён: %+v", it)
		}
	}

	// ошибка и пропуск попали в список на проработку
	ms, err := svc.GetMistakes()
	if err != nil {
		t.Fatal(err)
	}
	inList := map[string]bool{}
	for _, m := range ms {
		inList[m.Question.ID] = true
	}
	if !inList[wrongID] || !inList[skippedID] || len(ms) != 2 {
		t.Errorf("в списке ошибок %d вопросов: %v", len(ms), inList)
	}

	// повторная сдача того же билета невозможна
	if _, err := svc.FinishExam(ex.ExamID, ex.Lease); !errors.Is(err, ErrNoExam) {
		t.Errorf("завершённый экзамен сдан повторно: err=%v", err)
	}
}

func TestExamFailVerdict(t *testing.T) {
	svc := newService(t)
	clk := withClock(svc)
	ex, _ := svc.StartExam()
	clk.advance(30 * time.Minute)
	res, err := svc.FinishExam(ex.ExamID, ex.Lease)
	if err != nil {
		t.Fatal(err)
	}
	if res.Correct != 0 || res.Passed || !res.TimedOut || res.ElapsedSec != 1800 {
		t.Errorf("пустой билет должен быть провален по времени: %+v", res)
	}
	if n, _ := svc.store.MistakeCount(); n != 20 {
		t.Errorf("в списке ошибок %d вопросов, ожидалось 20", n)
	}
}

func TestExamValidatesAnswers(t *testing.T) {
	svc := newService(t)
	ex, _ := svc.StartExam()
	q := ex.Questions[0]
	if _, err := svc.AnswerExam(ex.ExamID, ex.Lease, "нет-такого", 1); err == nil {
		t.Error("принят ответ на вопрос не из билета")
	}
	if _, err := svc.AnswerExam(ex.ExamID, ex.Lease, q.ID, len(q.Options)+1); err == nil {
		t.Error("принят несуществующий вариант")
	}
	if _, err := svc.AnswerExam(ex.ExamID+1, ex.Lease, q.ID, 1); !errors.Is(err, ErrNoExam) {
		t.Errorf("ответ в чужой экзамен: err=%v", err)
	}
}

func TestOnlyOneActiveExam(t *testing.T) {
	svc := newService(t)
	ex, _ := svc.StartExam()
	if _, err := svc.StartExam(); !errors.Is(err, ErrExamActive) {
		t.Fatalf("второй экзамен поверх незавершённого: err=%v", err)
	}
	st, _ := svc.GetState()
	if st.ActiveExam == nil || st.ActiveExam.ExamID != ex.ExamID || st.ActiveExam.Total != 20 {
		t.Fatalf("незавершённый экзамен на главной: %+v", st.ActiveExam)
	}

	// брошенный экзамен не оставляет следов
	if err := svc.AbandonExam(ex.ExamID); err != nil {
		t.Fatal(err)
	}
	st, _ = svc.GetState()
	stats, _ := svc.GetStats()
	if st.ActiveExam != nil || st.MistakeCount != 0 || len(stats.Exams) != 0 || stats.Overall.Answered != 0 {
		t.Errorf("после броска: %+v, экзаменов в истории %d", st, len(stats.Exams))
	}
	if _, err := svc.StartExam(); err != nil {
		t.Errorf("после броска новый экзамен не стартует: %v", err)
	}
}

func TestExamPauseAndResume(t *testing.T) {
	svc := newService(t)
	clk := withClock(svc)
	ex, _ := svc.StartExam()
	q := ex.Questions[3]
	svc.AnswerExam(ex.ExamID, ex.Lease, q.ID, 2)

	clk.advance(10 * time.Minute)
	if err := svc.PauseExam(ex.ExamID, ex.Lease); err != nil { // страницу закрыли
		t.Fatal(err)
	}
	clk.advance(5 * time.Hour) // пока закрыта, время не идёт
	st, _ := svc.GetState()
	if st.ActiveExam.RemainingMs != 20*60*1000 || st.ActiveExam.Answered != 1 {
		t.Fatalf("на паузе: %+v", st.ActiveExam)
	}

	open, err := svc.ResumeExam(ex.ExamID)
	if err != nil || open.Session == nil {
		t.Fatalf("продолжение: %+v, err=%v", open, err)
	}
	s := open.Session
	if s.RemainingMs != 20*60*1000 || s.Chosen[q.ID] != 2 || s.Lease == ex.Lease {
		t.Errorf("после продолжения: осталось %d, ответы %v, lease сменился=%v",
			s.RemainingMs, s.Chosen, s.Lease != ex.Lease)
	}
	for _, q := range s.Questions {
		if q.Answer != 0 {
			t.Fatalf("%s: правильный ответ утёк при продолжении", q.ID)
		}
	}
	clk.advance(time.Minute) // таймер снова идёт
	if tick, _ := svc.ExamStatus(ex.ExamID, s.Lease); tick.RemainingMs != 19*60*1000 {
		t.Errorf("после минуты осталось %d мс", tick.RemainingMs)
	}
}

func TestExamTakeoverByAnotherDevice(t *testing.T) {
	svc := newService(t)
	clk := withClock(svc)
	ex, _ := svc.StartExam() // компьютер
	clk.advance(time.Minute)
	open, _ := svc.ResumeExam(ex.ExamID) // телефон
	phone := open.Session.Lease

	if _, err := svc.AnswerExam(ex.ExamID, ex.Lease, ex.Questions[0].ID, 1); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("старое устройство ответило: err=%v", err)
	}
	if _, err := svc.ExamStatus(ex.ExamID, ex.Lease); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("старое устройство не узнало о потере: err=%v", err)
	}
	if _, err := svc.FinishExam(ex.ExamID, ex.Lease); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("старое устройство сдало экзамен: err=%v", err)
	}
	// закрытие вкладки на старом устройстве не ставит телефон на паузу
	if err := svc.PauseExam(ex.ExamID, ex.Lease); err != nil {
		t.Fatal(err)
	}
	clk.advance(time.Minute)
	if tick, err := svc.ExamStatus(ex.ExamID, phone); err != nil || tick.RemainingMs != 28*60*1000 {
		t.Errorf("на телефоне осталось %d мс, err=%v", tick.RemainingMs, err)
	}
	if _, err := svc.AnswerExam(ex.ExamID, phone, ex.Questions[0].ID, 1); err != nil {
		t.Errorf("телефон не может отвечать: %v", err)
	}
}

func TestExamTimesOutWhileOpen(t *testing.T) {
	svc := newService(t)
	clk := withClock(svc)
	ex, _ := svc.StartExam()
	svc.AnswerExam(ex.ExamID, ex.Lease, ex.Questions[0].ID, 1)

	// вкладка свёрнута, время идёт и кончается
	clk.advance(31 * time.Minute)
	tick, err := svc.AnswerExam(ex.ExamID, ex.Lease, ex.Questions[1].ID, 1)
	if err != nil || tick.RemainingMs != 0 {
		t.Fatalf("ответ после истечения времени: %+v, err=%v", tick, err)
	}
	// закрыли страницу уже после истечения, вернулись — экзамен сдан по времени
	svc.PauseExam(ex.ExamID, ex.Lease)
	open, err := svc.ResumeExam(ex.ExamID)
	if err != nil || open.Result == nil || open.Session != nil {
		t.Fatalf("продолжение после истечения: %+v, err=%v", open, err)
	}
	r := open.Result
	if !r.TimedOut || r.ElapsedSec != 1800 || r.Items[1].Chosen != 0 || r.Items[0].Chosen != 1 {
		t.Errorf("результат по времени: timed_out=%v elapsed=%d", r.TimedOut, r.ElapsedSec)
	}
	if st, _ := svc.GetState(); st.ActiveExam != nil {
		t.Errorf("экзамен остался незавершённым: %+v", st.ActiveExam)
	}
}

func TestTrainingResumeAndAnswers(t *testing.T) {
	svc := newService(t)
	tr, err := svc.StartTraining(bank.Selection{Scope: "group", Group: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Questions) != 51 || tr.ResumeIndex != 0 || tr.Key != "resume:group:10" {
		t.Fatalf("тренировка: %d вопросов, ключ %q, позиция %d",
			len(tr.Questions), tr.Key, tr.ResumeIndex)
	}
	if tr.Questions[0].Answer == 0 {
		t.Error("в тренировке правильный ответ нужен для мгновенной подсветки")
	}

	// отвечаем неверно -> вопрос в списке; дважды верно -> уходит
	q := tr.Questions[3]
	wrong := q.Answer%len(q.Options) + 1
	r, err := svc.RecordAnswer(q.ID, wrong, "training")
	if err != nil {
		t.Fatal(err)
	}
	if r.Correct || r.Answer != q.Answer || !r.Mistake.InList {
		t.Errorf("неверный ответ: %+v", r)
	}
	if r, _ = svc.RecordAnswer(q.ID, q.Answer, "mistakes"); !r.Correct || !r.Mistake.InList {
		t.Errorf("после одного верного вопрос должен остаться в списке: %+v", r)
	}
	if r, _ = svc.RecordAnswer(q.ID, q.Answer, "mistakes"); !r.Mistake.JustSolved || r.Mistake.InList {
		t.Errorf("после двух верных вопрос должен уйти из списка: %+v", r)
	}

	// позиция запоминается и подхватывается при следующем входе
	if err := svc.SaveTrainingPosition(tr.Key, 17); err != nil {
		t.Fatal(err)
	}
	again, _ := svc.StartTraining(bank.Selection{Scope: "group", Group: 10})
	if again.ResumeIndex != 17 {
		t.Errorf("позиция не восстановилась: %d", again.ResumeIndex)
	}

	// перемешанная выборка начинается с начала
	shuffled, _ := svc.StartTraining(bank.Selection{Scope: "group", Group: 10, Shuffle: true})
	if shuffled.ResumeIndex != 0 {
		t.Errorf("перемешанная выборка не должна использовать позицию: %d", shuffled.ResumeIndex)
	}

	// битая позиция (банк изменился) не роняет режим
	svc.SaveTrainingPosition(tr.Key, 9999)
	fixed, _ := svc.StartTraining(bank.Selection{Scope: "group", Group: 10})
	if fixed.ResumeIndex != 0 {
		t.Errorf("позиция вне диапазона должна сбрасываться: %d", fixed.ResumeIndex)
	}
}

func TestRecordAnswerValidates(t *testing.T) {
	svc := newService(t)
	if _, err := svc.RecordAnswer("нет-такого", 1, "training"); err == nil {
		t.Error("несуществующий вопрос должен давать ошибку")
	}
	if _, err := svc.RecordAnswer("abc-ru-g1-q1", 1, "exam"); err == nil {
		t.Error("режим exam через RecordAnswer недопустим")
	}
}

func TestStats(t *testing.T) {
	svc := newService(t)
	ex, _ := svc.StartExam()
	for _, q := range ex.Questions {
		full, _ := svc.bank.ByID(q.ID)
		svc.AnswerExam(ex.ExamID, ex.Lease, q.ID, full.Answer)
	}
	if _, err := svc.FinishExam(ex.ExamID, ex.Lease); err != nil {
		t.Fatal(err)
	}

	st, err := svc.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Exams) != 1 || st.Exams[0].Correct != 20 || !st.Exams[0].Passed {
		t.Errorf("история экзаменов: %+v", st.Exams)
	}
	if st.Overall.Total != 1050 || st.Overall.Seen != 20 || st.Overall.Correct != 20 {
		t.Errorf("итого: %+v", st.Overall)
	}
	if len(st.BySection) != 4 || st.BySection[0].Key != "law" {
		t.Errorf("разделы: %+v", st.BySection)
	}
	sumTotal := 0
	for _, a := range st.ByGroup {
		sumTotal += a.Total
	}
	if len(st.ByGroup) != 10 || sumTotal != 1050 {
		t.Errorf("группы: %d строк, сумма вопросов %d", len(st.ByGroup), sumTotal)
	}
	// квоты билета: по разделам видно 2/15/2/1
	want := map[string]int{"law": 2, "pdd": 15, "faults": 2, "first_aid": 1}
	for _, a := range st.BySection {
		if a.Seen != want[a.Key] {
			t.Errorf("раздел %s: в билете было %d вопросов, ожидалось %d", a.Key, a.Seen, want[a.Key])
		}
	}
}
