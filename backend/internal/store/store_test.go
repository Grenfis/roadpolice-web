package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("открытие базы: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMistakeLifecycle(t *testing.T) {
	s := open(t)
	const q = "abc-ru-g1-q1"

	// верный ответ на вопрос, которого не было в ошибках — список не трогаем
	st, err := s.RecordAnswer(q, "training", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.InList || st.WrongCount != 0 {
		t.Errorf("верный ответ создал запись в ошибках: %+v", st)
	}

	// ошибка — вопрос попадает в список
	st, _ = s.RecordAnswer(q, "training", 1, false)
	if !st.InList || st.WrongCount != 1 || st.Streak != 0 {
		t.Errorf("после ошибки: %+v", st)
	}
	if n, _ := s.MistakeCount(); n != 1 {
		t.Errorf("в списке %d вопросов, ожидался 1", n)
	}

	// одного верного ответа мало
	st, _ = s.RecordAnswer(q, "mistakes", 3, true)
	if !st.InList || st.Streak != 1 || st.JustSolved {
		t.Errorf("после 1 верного: %+v", st)
	}
	if n, _ := s.MistakeCount(); n != 1 {
		t.Errorf("вопрос ушёл из списка раньше времени")
	}

	// второй верный подряд — вопрос проработан
	st, _ = s.RecordAnswer(q, "mistakes", 3, true)
	if st.InList || !st.JustSolved || st.Streak != ResolveStreak {
		t.Errorf("после 2 верных: %+v", st)
	}
	if n, _ := s.MistakeCount(); n != 0 {
		t.Errorf("в списке осталось %d вопросов", n)
	}

	// дальнейшие верные ответы ничего не ломают
	if st, _ = s.RecordAnswer(q, "training", 3, true); st.InList {
		t.Errorf("проработанный вопрос вернулся в список: %+v", st)
	}

	// новая ошибка возвращает вопрос в список и наращивает счётчик
	st, _ = s.RecordAnswer(q, "training", 2, false)
	if !st.InList || st.WrongCount != 2 || st.Streak != 0 {
		t.Errorf("после повторной ошибки: %+v", st)
	}
	ms, err := s.Mistakes()
	if err != nil || len(ms) != 1 || ms[0].QuestionID != q || ms[0].WrongCount != 2 {
		t.Errorf("список ошибок: %+v, err=%v", ms, err)
	}
}

func TestMistakesOrdering(t *testing.T) {
	s := open(t)
	s.RecordAnswer("q-rare", "training", 1, false)
	for i := 0; i < 3; i++ {
		s.RecordAnswer("q-often", "training", 1, false)
	}
	ms, _ := s.Mistakes()
	if len(ms) != 2 || ms[0].QuestionID != "q-often" {
		t.Errorf("сначала должны идти самые частые ошибки: %+v", ms)
	}
}

func TestExamFlow(t *testing.T) {
	s := open(t)
	id, err := s.StartExam(ticketIDs(20), "lease", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	items := []ExamItem{
		{QuestionID: "q1", Chosen: 1, Correct: true},
		{QuestionID: "q2", Chosen: 0, Correct: false},
	}
	if err := s.FinishExam(id, items, 19, true, false, 742); err != nil {
		t.Fatal(err)
	}
	exams, err := s.Exams(10)
	if err != nil || len(exams) != 1 {
		t.Fatalf("история экзаменов: %+v, err=%v", exams, err)
	}
	e := exams[0]
	if e.Correct != 19 || !e.Passed || e.ElapsedSec != 742 || e.Total != 20 || e.TimedOut {
		t.Errorf("запись экзамена: %+v", e)
	}
	// незавершённые экзамены в историю не попадают
	if _, err := s.StartExam(ticketIDs(20), "lease", time.Now()); err != nil {
		t.Fatal(err)
	}
	if exams, _ := s.Exams(10); len(exams) != 1 {
		t.Errorf("незавершённый экзамен попал в историю: %+v", exams)
	}
}

func TestQuestionStatsAndSettings(t *testing.T) {
	s := open(t)
	s.RecordAnswer("q1", "training", 1, true)
	s.RecordAnswer("q1", "training", 2, false)
	s.RecordAnswer("q2", "training", 1, true)
	st, err := s.QuestionStats()
	if err != nil {
		t.Fatal(err)
	}
	if st["q1"].Answered != 2 || st["q1"].Correct != 1 || st["q2"].Answered != 1 {
		t.Errorf("агрегаты: %+v", st)
	}

	if v, _ := s.Setting("theme", "system"); v != "system" {
		t.Errorf("значение по умолчанию потеряно: %q", v)
	}
	if err := s.SetSetting("theme", "dark"); err != nil {
		t.Fatal(err)
	}
	s.SetSetting("theme", "light") // перезапись
	if v, _ := s.Setting("theme", "system"); v != "light" {
		t.Errorf("настройка не перезаписалась: %q", v)
	}
}

func ticketIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("q%d", i+1)
	}
	return ids
}

func TestActiveExamLifecycle(t *testing.T) {
	s := open(t)
	if a, err := s.ActiveExam(); err != nil || a != nil {
		t.Fatalf("на пустой базе есть незавершённый экзамен: %+v, err=%v", a, err)
	}
	now := time.UnixMilli(1_700_000_000_000)
	id, err := s.StartExam(ticketIDs(3), "dev1", now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.ActiveExam()
	if err != nil || a == nil {
		t.Fatalf("незавершённый экзамен не нашёлся: err=%v", err)
	}
	if a.ExamID != id || len(a.QuestionIDs) != 3 || a.QuestionIDs[2] != "q3" ||
		a.Lease != "dev1" || a.RunningSince != now.UnixMilli() || a.ElapsedMs != 0 || len(a.Chosen) != 0 {
		t.Fatalf("после старта: %+v", a)
	}

	a.Chosen["q2"] = 3
	a.ElapsedMs, a.RunningSince, a.Lease = 61_000, 0, "dev2"
	if err := s.SaveActiveExam(a); err != nil {
		t.Fatal(err)
	}
	b, _ := s.ActiveExam()
	if b.Chosen["q2"] != 3 || b.ElapsedMs != 61_000 || b.RunningSince != 0 || b.Lease != "dev2" {
		t.Fatalf("после сохранения: %+v", b)
	}

	// завершение убирает экзамен из незавершённых
	if err := s.FinishExam(id, nil, 0, false, false, 61); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.ActiveExam(); a != nil {
		t.Fatalf("завершённый экзамен остался незавершённым: %+v", a)
	}
}

func TestAbandonExam(t *testing.T) {
	s := open(t)
	id, err := s.StartExam(ticketIDs(20), "lease", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AbandonExam(id); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.ActiveExam(); a != nil {
		t.Fatalf("брошенный экзамен остался: %+v", a)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM exams`).Scan(&n)
	if n != 0 {
		t.Errorf("брошенный экзамен остался в exams")
	}
	if err := s.AbandonExam(id); err == nil {
		t.Error("второй раз бросить тот же экзамен получилось")
	}
}

func TestExplanationFlags(t *testing.T) {
	s := open(t)
	if err := s.SetFlag("q1", "answer", "r1", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFlag("q1", "answer", "r1", "не тот пункт"); err != nil { // смена комментария
		t.Fatal(err)
	}
	s.SetFlag("q1", "2", "r9", "")
	s.SetFlag("q2", "answer", "r5", "")

	fl, err := s.Flags("q1")
	if err != nil || len(fl) != 2 || fl["answer@r1"].Comment != "не тот пункт" {
		t.Fatalf("пометки q1: %+v, err=%v", fl, err)
	}
	if err := s.ClearFlag("q1", "answer", "r1"); err != nil {
		t.Fatal(err)
	}
	if fl, _ := s.Flags("q1"); len(fl) != 1 {
		t.Errorf("после снятия осталось %d пометок", len(fl))
	}
	if all, _ := s.AllFlags(); len(all) != 2 {
		t.Errorf("всего пометок %d, ожидалось 2", len(all))
	}
}
