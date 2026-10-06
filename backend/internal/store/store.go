// Package store — SQLite-хранилище: история ответов, список ошибок, экзамены.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // чистый Go, без CGO — образ собирается с CGO_ENABLED=0
)

const schema = `
CREATE TABLE IF NOT EXISTS answers (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  ts          INTEGER NOT NULL,
  question_id TEXT    NOT NULL,
  mode        TEXT    NOT NULL,          -- exam | training | mistakes
  chosen      INTEGER NOT NULL,          -- 0 = не отвечено (вышло время)
  correct     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS answers_question ON answers(question_id);

CREATE TABLE IF NOT EXISTS mistakes (
  question_id    TEXT PRIMARY KEY,
  wrong_count    INTEGER NOT NULL DEFAULT 0,
  streak         INTEGER NOT NULL DEFAULT 0,  -- верных подряд после последней ошибки
  first_wrong_ts INTEGER NOT NULL,
  last_wrong_ts  INTEGER NOT NULL,
  resolved_at    INTEGER                      -- NULL = ещё в списке на проработку
);

CREATE TABLE IF NOT EXISTS exams (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  started_at    INTEGER NOT NULL,
  finished_at   INTEGER,
  total         INTEGER NOT NULL,
  correct_count INTEGER,
  passed        INTEGER,
  elapsed_sec   INTEGER,
  timed_out     INTEGER
);

CREATE TABLE IF NOT EXISTS exam_items (
  exam_id     INTEGER NOT NULL,
  idx         INTEGER NOT NULL,
  question_id TEXT    NOT NULL,
  chosen      INTEGER NOT NULL,
  correct     INTEGER NOT NULL,
  PRIMARY KEY (exam_id, idx)
);

-- незавершённый экзамен: билет, ответы и таймер живут на сервере, чтобы
-- экзамен переживал перезагрузку страницы и переезжал между устройствами
CREATE TABLE IF NOT EXISTS active_exam (
  exam_id       INTEGER PRIMARY KEY,   -- = exams.id
  question_ids  TEXT    NOT NULL,      -- JSON-массив id в порядке билета
  chosen        TEXT    NOT NULL,      -- JSON {id вопроса: вариант}
  elapsed_ms    INTEGER NOT NULL,      -- набежало до последней паузы
  running_since INTEGER,               -- unix ms; NULL = на паузе (страница закрыта)
  lease         TEXT    NOT NULL       -- токен устройства, которое ведёт экзамен
);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

// ResolveStreak — сколько верных ответов подряд убирают вопрос из списка ошибок.
const ResolveStreak = 2

type Store struct{ db *sql.DB }

// DefaultPath — файл базы в пользовательском каталоге настроек.
func DefaultPath() (string, error) {
	if p := os.Getenv("ROADPOLICE_TRAINER_DB"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "roadpolice-trainer", "trainer.db"), nil
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("каталог базы: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("открытие базы: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("схема базы: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// MistakeState — состояние вопроса в списке ошибок после очередного ответа.
type MistakeState struct {
	InList     bool `json:"in_list"`     // вопрос сейчас в списке на проработку
	WrongCount int  `json:"wrong_count"` // сколько раз всего ошибались
	Streak     int  `json:"streak"`      // верных подряд после последней ошибки
	JustSolved bool `json:"just_solved"` // этим ответом вопрос ушёл из списка
}

// RecordAnswer пишет ответ в историю и обновляет список ошибок.
// Неверный ответ возвращает вопрос в список (даже если он был проработан),
// верный — наращивает серию и убирает вопрос после ResolveStreak подряд.
func (s *Store) RecordAnswer(questionID, mode string, chosen int, correct bool) (MistakeState, error) {
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return MistakeState{}, err
	}
	defer tx.Rollback()

	ci := 0
	if correct {
		ci = 1
	}
	if _, err := tx.Exec(`INSERT INTO answers(ts, question_id, mode, chosen, correct) VALUES(?,?,?,?,?)`,
		now, questionID, mode, chosen, ci); err != nil {
		return MistakeState{}, err
	}

	var st MistakeState
	if !correct {
		if _, err := tx.Exec(`
			INSERT INTO mistakes(question_id, wrong_count, streak, first_wrong_ts, last_wrong_ts, resolved_at)
			VALUES(?, 1, 0, ?, ?, NULL)
			ON CONFLICT(question_id) DO UPDATE SET
				wrong_count = wrong_count + 1,
				streak = 0,
				last_wrong_ts = excluded.last_wrong_ts,
				resolved_at = NULL`, questionID, now, now); err != nil {
			return MistakeState{}, err
		}
		if err := tx.QueryRow(`SELECT wrong_count, streak FROM mistakes WHERE question_id = ?`,
			questionID).Scan(&st.WrongCount, &st.Streak); err != nil {
			return MistakeState{}, err
		}
		st.InList = true
	} else {
		var wrong, streak int
		var resolved sql.NullInt64
		err := tx.QueryRow(`SELECT wrong_count, streak, resolved_at FROM mistakes WHERE question_id = ?`,
			questionID).Scan(&wrong, &streak, &resolved)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// вопрос никогда не заваливали — в списке ошибок его и нет
		case err != nil:
			return MistakeState{}, err
		case resolved.Valid:
			// уже проработан, серию дальше не считаем
			st = MistakeState{WrongCount: wrong, Streak: streak}
		default:
			streak++
			st = MistakeState{WrongCount: wrong, Streak: streak, InList: streak < ResolveStreak}
			if streak >= ResolveStreak {
				st.JustSolved = true
				if _, err := tx.Exec(`UPDATE mistakes SET streak = ?, resolved_at = ? WHERE question_id = ?`,
					streak, now, questionID); err != nil {
					return MistakeState{}, err
				}
			} else if _, err := tx.Exec(`UPDATE mistakes SET streak = ? WHERE question_id = ?`,
				streak, questionID); err != nil {
				return MistakeState{}, err
			}
		}
	}
	return st, tx.Commit()
}

// Mistake — строка списка на проработку.
type Mistake struct {
	QuestionID string `json:"question_id"`
	WrongCount int    `json:"wrong_count"`
	Streak     int    `json:"streak"`
	LastWrong  int64  `json:"last_wrong_ts"`
}

// Mistakes возвращает непроработанные ошибки: сначала самые частые,
// внутри равных — самые свежие.
func (s *Store) Mistakes() ([]Mistake, error) {
	rows, err := s.db.Query(`
		SELECT question_id, wrong_count, streak, last_wrong_ts
		FROM mistakes WHERE resolved_at IS NULL
		ORDER BY wrong_count DESC, last_wrong_ts DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mistake{}
	for rows.Next() {
		var m Mistake
		if err := rows.Scan(&m.QuestionID, &m.WrongCount, &m.Streak, &m.LastWrong); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) MistakeCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM mistakes WHERE resolved_at IS NULL`).Scan(&n)
	return n, err
}

// ActiveExam — незавершённый экзамен. RunningSince == 0 — таймер на паузе.
type ActiveExam struct {
	ExamID       int64
	QuestionIDs  []string
	Chosen       map[string]int
	ElapsedMs    int64
	RunningSince int64
	Lease        string
}

// StartExam создаёт запись экзамена и незавершённый экзамен с запущенным
// таймером, возвращает id.
func (s *Store) StartExam(questionIDs []string, lease string, now time.Time) (int64, error) {
	ids, err := json.Marshal(questionIDs)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO exams(started_at, total) VALUES(?,?)`, now.Unix(), len(questionIDs))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
		INSERT INTO active_exam(exam_id, question_ids, chosen, elapsed_ms, running_since, lease)
		VALUES(?, ?, '{}', 0, ?, ?)`, id, string(ids), now.UnixMilli(), lease); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// ActiveExam возвращает незавершённый экзамен или nil, если его нет.
func (s *Store) ActiveExam() (*ActiveExam, error) {
	var a ActiveExam
	var ids, chosen string
	var running sql.NullInt64
	err := s.db.QueryRow(`
		SELECT exam_id, question_ids, chosen, elapsed_ms, running_since, lease
		FROM active_exam ORDER BY exam_id DESC LIMIT 1`).
		Scan(&a.ExamID, &ids, &chosen, &a.ElapsedMs, &running, &a.Lease)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(ids), &a.QuestionIDs); err != nil {
		return nil, fmt.Errorf("билет экзамена %d: %w", a.ExamID, err)
	}
	if err := json.Unmarshal([]byte(chosen), &a.Chosen); err != nil {
		return nil, fmt.Errorf("ответы экзамена %d: %w", a.ExamID, err)
	}
	a.RunningSince = running.Int64
	return &a, nil
}

// SaveActiveExam сохраняет ответы, таймер и владельца незавершённого экзамена.
func (s *Store) SaveActiveExam(a *ActiveExam) error {
	chosen, err := json.Marshal(a.Chosen)
	if err != nil {
		return err
	}
	running := sql.NullInt64{Int64: a.RunningSince, Valid: a.RunningSince != 0}
	_, err = s.db.Exec(`
		UPDATE active_exam SET chosen = ?, elapsed_ms = ?, running_since = ?, lease = ?
		WHERE exam_id = ?`, string(chosen), a.ElapsedMs, running, a.Lease, a.ExamID)
	return err
}

// AbandonExam удаляет незавершённый экзамен бесследно: ни в историю,
// ни в статистику он не попадает.
func (s *Store) AbandonExam(examID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM active_exam WHERE exam_id = ?`, examID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("незавершённого экзамена %d нет", examID)
	}
	if _, err := tx.Exec(`DELETE FROM exams WHERE id = ? AND finished_at IS NULL`, examID); err != nil {
		return err
	}
	return tx.Commit()
}

// ExamItem — один вопрос сданного билета.
type ExamItem struct {
	QuestionID string `json:"question_id"`
	Chosen     int    `json:"chosen"`
	Correct    bool   `json:"correct"`
}

func (s *Store) FinishExam(examID int64, items []ExamItem, correct int, passed, timedOut bool, elapsedSec int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, it := range items {
		ci := 0
		if it.Correct {
			ci = 1
		}
		if _, err := tx.Exec(`INSERT INTO exam_items(exam_id, idx, question_id, chosen, correct) VALUES(?,?,?,?,?)`,
			examID, i, it.QuestionID, it.Chosen, ci); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM active_exam WHERE exam_id = ?`, examID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE exams SET finished_at=?, correct_count=?, passed=?, elapsed_sec=?, timed_out=? WHERE id=?`,
		time.Now().Unix(), correct, boolInt(passed), elapsedSec, boolInt(timedOut), examID); err != nil {
		return err
	}
	return tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ExamRecord — экзамен в истории.
type ExamRecord struct {
	ID         int64 `json:"id"`
	FinishedAt int64 `json:"finished_at"`
	Total      int   `json:"total"`
	Correct    int   `json:"correct"`
	Passed     bool  `json:"passed"`
	ElapsedSec int   `json:"elapsed_sec"`
	TimedOut   bool  `json:"timed_out"`
}

func (s *Store) Exams(limit int) ([]ExamRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, finished_at, total, correct_count, passed, elapsed_sec, timed_out
		FROM exams WHERE finished_at IS NOT NULL
		ORDER BY finished_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExamRecord{}
	for rows.Next() {
		var r ExamRecord
		var passed, timedOut int
		if err := rows.Scan(&r.ID, &r.FinishedAt, &r.Total, &r.Correct, &passed, &r.ElapsedSec, &timedOut); err != nil {
			return nil, err
		}
		r.Passed, r.TimedOut = passed == 1, timedOut == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// QuestionStat — агрегат ответов по одному вопросу.
type QuestionStat struct {
	Answered int `json:"answered"`
	Correct  int `json:"correct"`
}

func (s *Store) QuestionStats() (map[string]QuestionStat, error) {
	rows, err := s.db.Query(`
		SELECT question_id, COUNT(*), SUM(correct) FROM answers GROUP BY question_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]QuestionStat{}
	for rows.Next() {
		var id string
		var n, ok int
		if err := rows.Scan(&id, &n, &ok); err != nil {
			return nil, err
		}
		out[id] = QuestionStat{Answered: n, Correct: ok}
	}
	return out, rows.Err()
}

func (s *Store) Setting(key, def string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO settings(key, value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
