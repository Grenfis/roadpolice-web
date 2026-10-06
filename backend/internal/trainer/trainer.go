// Package trainer — вся логика тренажёра: билеты, тренировка, ошибки,
// статистика. От HTTP не зависит, поэтому тестируется напрямую.
package trainer

import (
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"sync"
	"time"

	"roadpolice-web/internal/bank"
	"roadpolice-web/internal/store"
)

// Service — ядро тренажёра.
type Service struct {
	bank  *bank.Bank
	store *store.Store
	rnd   *rand.Rand

	now func() time.Time // подменяется в тестах

	mu sync.Mutex // экзамен: чтение-изменение-запись active_exam
}

func New(b *bank.Bank, s *store.Store) *Service {
	return &Service{
		bank:  b,
		store: s,
		rnd:   rand.New(rand.NewSource(time.Now().UnixNano())),
		now:   time.Now,
	}
}

// Close закрывает базу.
func (a *Service) Close() error {
	if a.store == nil {
		return nil
	}
	return a.store.Close()
}

// ---------- общее состояние ----------

type GroupInfo struct {
	Group int    `json:"group"`
	Count int    `json:"count"`
	Title string `json:"title"`
}

type State struct {
	Meta         bank.Meta       `json:"meta"`
	SectionOrder []string        `json:"section_order"`
	BySection    map[string]int  `json:"by_section"`
	Groups       []GroupInfo     `json:"groups"`
	MistakeCount int             `json:"mistake_count"`
	ResolveAfter int             `json:"resolve_after"`
	ActiveExam   *ActiveExamInfo `json:"active_exam"` // nil — незавершённого нет
}

// названия групп харцашара — по преобладающей теме вопросов группы
var groupTitles = map[int]string{
	1:  "Маневрирование, развороты, помехи",
	2:  "Закон о безопасности дорожного движения",
	3:  "Неисправности, запрещающие эксплуатацию",
	4:  "Дорожные знаки",
	5:  "Очерёдность проезда перекрёстков",
	6:  "Светофоры, регулировщик, запреты движения",
	7:  "Остановка, стоянка, разметка",
	8:  "Скорость, буксировка, перевозка грузов",
	9:  "Обгон, сигналы, спецтранспорт, световые приборы",
	10: "Первая помощь и безопасность жизни",
}

func (a *Service) GetState() (State, error) {
	mc, err := a.store.MistakeCount()
	if err != nil {
		return State{}, err
	}
	active, err := a.activeInfo(a.now())
	if err != nil {
		return State{}, err
	}
	counts := a.bank.CountByGroup()
	groups := make([]GroupInfo, 0, len(counts))
	for g, n := range counts {
		groups = append(groups, GroupInfo{Group: g, Count: n, Title: groupTitles[g]})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Group < groups[j].Group })

	return State{
		Meta:         a.bank.Meta,
		SectionOrder: a.bank.SectionIDs(),
		BySection:    a.bank.CountBySection(),
		Groups:       groups,
		MistakeCount: mc,
		ResolveAfter: store.ResolveStreak,
		ActiveExam:   active,
	}, nil
}

// ---------- экзамен ----------

// Экзамен хранится на сервере целиком: билет, ответы и таймер. Устройство,
// которое последним открыло экзамен, получает lease и ведёт его; остальные
// при следующем запросе получают ErrLeaseLost.
//
// Таймер считает сервер: elapsed_ms плюс время с running_since. Таймер стоит
// только пока страница закрыта (фронт присылает PauseExam на pagehide);
// свёрнутая вкладка и погасший экран паузой не считаются.

var (
	ErrExamActive = errors.New("есть незавершённый экзамен")
	ErrNoExam     = errors.New("экзамен не найден (уже завершён или брошен?)")
	ErrLeaseLost  = errors.New("экзамен продолжен на другом устройстве")
)

// поблажка на сетевую задержку: фронт завершает экзамен по своему таймеру
const timeoutSlack = 2 * time.Second

type Exam struct {
	ExamID         int64           `json:"exam_id"`
	Questions      []bank.Question `json:"questions"` // без правильных ответов
	TimeMinutes    int             `json:"time_minutes"`
	PassMinCorrect int             `json:"pass_min_correct"`
}

// ExamSession — экзамен, открытый на этом устройстве.
type ExamSession struct {
	Exam
	Lease       string         `json:"lease"`
	Chosen      map[string]int `json:"chosen"`
	RemainingMs int64          `json:"remaining_ms"`
}

// ExamOpen — итог открытия экзамена: либо сессия, либо (если время вышло,
// пока страница была закрыта) готовый результат.
type ExamOpen struct {
	Session *ExamSession `json:"session,omitempty"`
	Result  *ExamResult  `json:"result,omitempty"`
}

// ExamTick — сколько осталось времени по часам сервера.
type ExamTick struct {
	RemainingMs int64 `json:"remaining_ms"`
}

// ActiveExamInfo — незавершённый экзамен для главного экрана.
type ActiveExamInfo struct {
	ExamID      int64 `json:"exam_id"`
	Answered    int   `json:"answered"`
	Total       int   `json:"total"`
	RemainingMs int64 `json:"remaining_ms"`
}

func (a *Service) examLimit() time.Duration {
	return time.Duration(a.bank.Meta.Exam.TimeMinutes) * time.Minute
}

func elapsed(ae *store.ActiveExam, now time.Time) time.Duration {
	ms := ae.ElapsedMs
	if ae.RunningSince != 0 {
		ms += max(0, now.UnixMilli()-ae.RunningSince)
	}
	return time.Duration(ms) * time.Millisecond
}

func (a *Service) remaining(ae *store.ActiveExam, now time.Time) int64 {
	return max(0, (a.examLimit() - elapsed(ae, now)).Milliseconds())
}

func newLease() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		panic(err) // crypto/rand не отказывает на поддерживаемых ОС
	}
	return hex.EncodeToString(b)
}

// activeExam возвращает незавершённый экзамен с заданным id.
func (a *Service) activeExam(examID int64) (*store.ActiveExam, error) {
	ae, err := a.store.ActiveExam()
	if err != nil {
		return nil, err
	}
	if ae == nil || ae.ExamID != examID {
		return nil, ErrNoExam
	}
	return ae, nil
}

// ownedExam — то же, но только для устройства-владельца.
func (a *Service) ownedExam(examID int64, lease string) (*store.ActiveExam, error) {
	ae, err := a.activeExam(examID)
	if err != nil {
		return nil, err
	}
	if ae.Lease != lease {
		return nil, ErrLeaseLost
	}
	return ae, nil
}

func (a *Service) activeInfo(now time.Time) (*ActiveExamInfo, error) {
	ae, err := a.store.ActiveExam()
	if err != nil || ae == nil {
		return nil, err
	}
	return &ActiveExamInfo{
		ExamID:      ae.ExamID,
		Answered:    len(ae.Chosen),
		Total:       len(ae.QuestionIDs),
		RemainingMs: a.remaining(ae, now),
	}, nil
}

func (a *Service) session(ae *store.ActiveExam, now time.Time) (*ExamSession, error) {
	qs := make([]bank.Question, 0, len(ae.QuestionIDs))
	for _, id := range ae.QuestionIDs {
		q, ok := a.bank.ByID(id)
		if !ok {
			return nil, fmt.Errorf("вопроса %s из билета нет в банке (банк обновился?)", id)
		}
		c := *q
		c.Answer = 0 // правильный ответ до сдачи на устройство не уходит
		qs = append(qs, c)
	}
	return &ExamSession{
		Exam: Exam{
			ExamID:         ae.ExamID,
			Questions:      qs,
			TimeMinutes:    a.bank.Meta.Exam.TimeMinutes,
			PassMinCorrect: a.bank.Meta.Exam.PassMinCorrect,
		},
		Lease:       ae.Lease,
		Chosen:      ae.Chosen,
		RemainingMs: a.remaining(ae, now),
	}, nil
}

// StartExam собирает билет по квотам разделов и запускает таймер.
// Если есть незавершённый экзамен, возвращает ErrExamActive: сначала его
// нужно продолжить или бросить.
func (a *Service) StartExam() (*ExamSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ae, err := a.store.ActiveExam(); err != nil {
		return nil, err
	} else if ae != nil {
		return nil, ErrExamActive
	}
	qs, err := a.bank.Ticket(a.rnd)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(qs))
	for i, q := range qs {
		ids[i] = q.ID
	}
	now := a.now()
	lease := newLease()
	id, err := a.store.StartExam(ids, lease, now)
	if err != nil {
		return nil, err
	}
	return a.session(&store.ActiveExam{ExamID: id, QuestionIDs: ids, Chosen: map[string]int{},
		RunningSince: now.UnixMilli(), Lease: lease}, now)
}

// ResumeExam открывает незавершённый экзамен на этом устройстве: выдаёт
// новый lease (прежнее устройство теряет экзамен) и снимает таймер с паузы.
// Если время вышло, пока страница была закрыта, экзамен завершается.
func (a *Service) ResumeExam(examID int64) (ExamOpen, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ae, err := a.activeExam(examID)
	if err != nil {
		return ExamOpen{}, err
	}
	now := a.now()
	if a.remaining(ae, now) == 0 {
		res, err := a.finish(ae, now)
		if err != nil {
			return ExamOpen{}, err
		}
		return ExamOpen{Result: &res}, nil
	}
	ae.Lease = newLease()
	if ae.RunningSince == 0 {
		ae.RunningSince = now.UnixMilli()
	}
	if err := a.store.SaveActiveExam(ae); err != nil {
		return ExamOpen{}, err
	}
	s, err := a.session(ae, now)
	if err != nil {
		return ExamOpen{}, err
	}
	return ExamOpen{Session: s}, nil
}

// AnswerExam сохраняет выбранный вариант. После истечения времени ответы
// не принимаются: RemainingMs == 0 — сигнал фронту завершать экзамен.
func (a *Service) AnswerExam(examID int64, lease, questionID string, chosen int) (ExamTick, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ae, err := a.ownedExam(examID, lease)
	if err != nil {
		return ExamTick{}, err
	}
	now := a.now()
	left := a.remaining(ae, now)
	if left == 0 {
		return ExamTick{}, nil
	}
	if !slices.Contains(ae.QuestionIDs, questionID) {
		return ExamTick{}, fmt.Errorf("вопроса %s нет в билете", questionID)
	}
	q, _ := a.bank.ByID(questionID)
	if chosen < 1 || chosen > len(q.Options) {
		return ExamTick{}, fmt.Errorf("варианта %d у вопроса %s нет", chosen, questionID)
	}
	ae.Chosen[questionID] = chosen
	if err := a.store.SaveActiveExam(ae); err != nil {
		return ExamTick{}, err
	}
	return ExamTick{RemainingMs: left}, nil
}

// ExamStatus — опрос с устройства: жив ли ещё lease и сколько осталось.
func (a *Service) ExamStatus(examID int64, lease string) (ExamTick, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ae, err := a.ownedExam(examID, lease)
	if err != nil {
		return ExamTick{}, err
	}
	return ExamTick{RemainingMs: a.remaining(ae, a.now())}, nil
}

// PauseExam останавливает таймер (страница закрыта или перезагружается).
// Чужой или устаревший lease молча игнорируется: экзамен уже ведёт другое
// устройство, и его таймер останавливать нельзя.
func (a *Service) PauseExam(examID int64, lease string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	ae, err := a.ownedExam(examID, lease)
	if errors.Is(err, ErrLeaseLost) || errors.Is(err, ErrNoExam) {
		return nil
	}
	if err != nil {
		return err
	}
	if ae.RunningSince == 0 {
		return nil
	}
	now := a.now()
	ae.ElapsedMs = min(elapsed(ae, now), a.examLimit()).Milliseconds()
	ae.RunningSince = 0
	return a.store.SaveActiveExam(ae)
}

// AbandonExam бросает незавершённый экзамен без следа в истории и ошибках.
// lease не требуется: бросить можно и с главного экрана другого устройства.
func (a *Service) AbandonExam(examID int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.activeExam(examID); err != nil {
		return err
	}
	return a.store.AbandonExam(examID)
}

type ReviewItem struct {
	Question bank.Question `json:"question"`
	Chosen   int           `json:"chosen"` // 0 — не отвечено
	Correct  bool          `json:"correct"`
}

type ExamResult struct {
	ExamID     int64        `json:"exam_id"`
	Total      int          `json:"total"`
	Correct    int          `json:"correct"`
	Passed     bool         `json:"passed"`
	TimedOut   bool         `json:"timed_out"`
	ElapsedSec int          `json:"elapsed_sec"`
	Items      []ReviewItem `json:"items"`
}

// FinishExam сдаёт экзамен с ответами, сохранёнными на сервере.
func (a *Service) FinishExam(examID int64, lease string) (ExamResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ae, err := a.ownedExam(examID, lease)
	if err != nil {
		return ExamResult{}, err
	}
	return a.finish(ae, a.now())
}

// finish оценивает билет, пишет результат и пополняет список ошибок;
// пропущенные вопросы считаются неверными.
func (a *Service) finish(ae *store.ActiveExam, now time.Time) (ExamResult, error) {
	limit := a.examLimit()
	spent := elapsed(ae, now)
	timedOut := spent >= limit-timeoutSlack
	spent = min(spent, limit)

	res := ExamResult{ExamID: ae.ExamID, Total: len(ae.QuestionIDs), TimedOut: timedOut,
		ElapsedSec: int(spent / time.Second)}
	items := make([]store.ExamItem, 0, len(ae.QuestionIDs))
	for _, id := range ae.QuestionIDs {
		q, ok := a.bank.ByID(id)
		if !ok {
			return ExamResult{}, fmt.Errorf("вопроса %s из билета нет в банке (банк обновился?)", id)
		}
		pick := ae.Chosen[id]
		okAns := pick == q.Answer
		if okAns {
			res.Correct++
		}
		res.Items = append(res.Items, ReviewItem{Question: *q, Chosen: pick, Correct: okAns})
		items = append(items, store.ExamItem{QuestionID: id, Chosen: pick, Correct: okAns})
		if _, err := a.store.RecordAnswer(id, "exam", pick, okAns); err != nil {
			return ExamResult{}, err
		}
	}
	res.Passed = res.Correct >= a.bank.Meta.Exam.PassMinCorrect
	if err := a.store.FinishExam(ae.ExamID, items, res.Correct, res.Passed, timedOut, res.ElapsedSec); err != nil {
		return ExamResult{}, err
	}
	return res, nil
}

// ---------- тренировка ----------

type Training struct {
	Key         string          `json:"key"`
	Questions   []bank.Question `json:"questions"` // с правильными ответами: подсветка мгновенная
	ResumeIndex int             `json:"resume_index"`
}

// StartTraining отдаёт выборку вопросов и позицию, на которой остановились
// в прошлый раз (для неперемешанных выборок).
func (a *Service) StartTraining(sel bank.Selection) (Training, error) {
	qs, err := a.bank.Select(sel, a.rnd)
	if err != nil {
		return Training{}, err
	}
	key := trainingKey(sel)
	resume := 0
	if !sel.Shuffle && key != "" {
		v, err := a.store.Setting(key, "0")
		if err != nil {
			return Training{}, err
		}
		fmt.Sscanf(v, "%d", &resume)
		if resume < 0 || resume >= len(qs) {
			resume = 0
		}
	}
	out := make([]bank.Question, len(qs))
	for i, q := range qs {
		out[i] = *q
	}
	return Training{Key: key, Questions: out, ResumeIndex: resume}, nil
}

func trainingKey(sel bank.Selection) string {
	switch sel.Scope {
	case "", "all":
		return "resume:all"
	case "group":
		return fmt.Sprintf("resume:group:%d", sel.Group)
	case "section":
		return "resume:section:" + sel.Section
	default:
		return ""
	}
}

func (a *Service) SaveTrainingPosition(key string, index int) error {
	if key == "" {
		return nil
	}
	return a.store.SetSetting(key, fmt.Sprintf("%d", index))
}

type AnswerResult struct {
	Correct bool               `json:"correct"`
	Answer  int                `json:"answer"`
	Mistake store.MistakeState `json:"mistake"`
}

// RecordAnswer проверяет ответ по банку (фронтенду не доверяем) и обновляет
// историю со списком ошибок. mode: training | mistakes.
func (a *Service) RecordAnswer(questionID string, chosen int, mode string) (AnswerResult, error) {
	q, ok := a.bank.ByID(questionID)
	if !ok {
		return AnswerResult{}, fmt.Errorf("нет вопроса %s", questionID)
	}
	if mode != "training" && mode != "mistakes" {
		return AnswerResult{}, fmt.Errorf("неизвестный режим %q", mode)
	}
	correct := chosen == q.Answer
	st, err := a.store.RecordAnswer(questionID, mode, chosen, correct)
	if err != nil {
		return AnswerResult{}, err
	}
	return AnswerResult{Correct: correct, Answer: q.Answer, Mistake: st}, nil
}

// ---------- работа над ошибками ----------

type MistakeRow struct {
	Question   bank.Question `json:"question"`
	WrongCount int           `json:"wrong_count"`
	Streak     int           `json:"streak"`
	LastWrong  int64         `json:"last_wrong_ts"`
}

func (a *Service) GetMistakes() ([]MistakeRow, error) {
	ms, err := a.store.Mistakes()
	if err != nil {
		return nil, err
	}
	out := make([]MistakeRow, 0, len(ms))
	for _, m := range ms {
		q, ok := a.bank.ByID(m.QuestionID)
		if !ok {
			continue // вопрос исчез из банка после обновления — молча пропускаем
		}
		out = append(out, MistakeRow{Question: *q, WrongCount: m.WrongCount,
			Streak: m.Streak, LastWrong: m.LastWrong})
	}
	return out, nil
}

// ---------- статистика ----------

type Accuracy struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Total    int    `json:"total"`    // вопросов в банке
	Seen     int    `json:"seen"`     // сколько из них хоть раз отвечены
	Answered int    `json:"answered"` // всего ответов
	Correct  int    `json:"correct"`  // из них верных
}

type Stats struct {
	Exams     []store.ExamRecord `json:"exams"`
	BySection []Accuracy         `json:"by_section"`
	ByGroup   []Accuracy         `json:"by_group"`
	Overall   Accuracy           `json:"overall"`
}

func (a *Service) GetStats() (Stats, error) {
	exams, err := a.store.Exams(50)
	if err != nil {
		return Stats{}, err
	}
	qstats, err := a.store.QuestionStats()
	if err != nil {
		return Stats{}, err
	}

	sec := map[string]*Accuracy{}
	grp := map[int]*Accuracy{}
	overall := Accuracy{Key: "all", Title: "Весь банк"}
	for _, q := range a.bank.Questions {
		s, ok := sec[q.Section]
		if !ok {
			s = &Accuracy{Key: q.Section, Title: a.bank.Meta.Sections[q.Section].Title}
			sec[q.Section] = s
		}
		g, ok := grp[q.Group]
		if !ok {
			g = &Accuracy{Key: fmt.Sprintf("%d", q.Group), Title: groupTitles[q.Group]}
			grp[q.Group] = g
		}
		s.Total++
		g.Total++
		overall.Total++
		if st, seen := qstats[q.ID]; seen {
			for _, acc := range []*Accuracy{s, g, &overall} {
				acc.Seen++
				acc.Answered += st.Answered
				acc.Correct += st.Correct
			}
		}
	}

	out := Stats{Exams: exams, Overall: overall}
	for _, sid := range a.bank.SectionIDs() {
		if s, ok := sec[sid]; ok {
			out.BySection = append(out.BySection, *s)
		}
	}
	gids := make([]int, 0, len(grp))
	for g := range grp {
		gids = append(gids, g)
	}
	sort.Ints(gids)
	for _, g := range gids {
		out.ByGroup = append(out.ByGroup, *grp[g])
	}
	return out, nil
}
