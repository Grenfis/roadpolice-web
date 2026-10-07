// Package api — HTTP-слой над trainer.Service: JSON-маршруты /api/*.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"roadpolice-web/internal/bank"
	"roadpolice-web/internal/trainer"
)

type handler struct{ svc *trainer.Service }

// Routes регистрирует маршруты API в mux.
func Routes(mux *http.ServeMux, svc *trainer.Service) {
	h := handler{svc}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		reply(w)(map[string]string{"status": "ok"}, nil)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, _ *http.Request) {
		reply(w)(svc.GetState())
	})
	mux.HandleFunc("POST /api/exams", func(w http.ResponseWriter, _ *http.Request) {
		reply(w)(svc.StartExam())
	})
	mux.HandleFunc("POST /api/exams/{id}/resume", h.resumeExam)
	mux.HandleFunc("PUT /api/exams/{id}/answers", h.answerExam)
	mux.HandleFunc("GET /api/exams/{id}/status", h.examStatus)
	mux.HandleFunc("POST /api/exams/{id}/pause", h.pauseExam)
	mux.HandleFunc("POST /api/exams/{id}/finish", h.finishExam)
	mux.HandleFunc("DELETE /api/exams/{id}", h.abandonExam)
	mux.HandleFunc("POST /api/training", h.startTraining)
	mux.HandleFunc("PUT /api/training/position", h.saveTrainingPosition)
	mux.HandleFunc("POST /api/answers", h.recordAnswer)
	mux.HandleFunc("GET /api/mistakes", func(w http.ResponseWriter, _ *http.Request) {
		reply(w)(svc.GetMistakes())
	})
	mux.HandleFunc("GET /api/stats", func(w http.ResponseWriter, _ *http.Request) {
		reply(w)(svc.GetStats())
	})
	mux.HandleFunc("GET /api/questions/{id}/explanation", func(w http.ResponseWriter, r *http.Request) {
		reply(w)(svc.GetExplanation(r.PathValue("id")))
	})
	mux.HandleFunc("PUT /api/questions/{id}/explanation/{part}/flag", h.setFlag)
	mux.HandleFunc("DELETE /api/questions/{id}/explanation/{part}/flag", func(w http.ResponseWriter, r *http.Request) {
		reply(w)(struct{}{}, svc.ClearFlag(r.PathValue("id"), r.PathValue("part")))
	})
	mux.HandleFunc("GET /api/flags", func(w http.ResponseWriter, _ *http.Request) {
		reply(w)(svc.Flags())
	})
}

// ---------- пояснения ----------

func (h handler) setFlag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Comment string `json:"comment"`
	}
	if !decode(w, r, &body) {
		return
	}
	reply(w)(struct{}{}, h.svc.SetFlag(r.PathValue("id"), r.PathValue("part"), body.Comment))
}

// ---------- экзамен ----------

type leaseBody struct {
	Lease string `json:"lease"`
}

func (h handler) resumeExam(w http.ResponseWriter, r *http.Request) {
	id, ok := examID(w, r)
	if !ok {
		return
	}
	reply(w)(h.svc.ResumeExam(id))
}

func (h handler) answerExam(w http.ResponseWriter, r *http.Request) {
	id, ok := examID(w, r)
	if !ok {
		return
	}
	var body struct {
		Lease      string `json:"lease"`
		QuestionID string `json:"question_id"`
		Chosen     int    `json:"chosen"`
	}
	if !decode(w, r, &body) {
		return
	}
	reply(w)(h.svc.AnswerExam(id, body.Lease, body.QuestionID, body.Chosen))
}

func (h handler) examStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := examID(w, r)
	if !ok {
		return
	}
	reply(w)(h.svc.ExamStatus(id, r.URL.Query().Get("lease")))
}

// pauseExam вызывается через navigator.sendBeacon при закрытии страницы.
func (h handler) pauseExam(w http.ResponseWriter, r *http.Request) {
	id, ok := examID(w, r)
	if !ok {
		return
	}
	var body leaseBody
	if !decode(w, r, &body) {
		return
	}
	reply(w)(struct{}{}, h.svc.PauseExam(id, body.Lease))
}

func (h handler) finishExam(w http.ResponseWriter, r *http.Request) {
	id, ok := examID(w, r)
	if !ok {
		return
	}
	var body leaseBody
	if !decode(w, r, &body) {
		return
	}
	reply(w)(h.svc.FinishExam(id, body.Lease))
}

func (h handler) abandonExam(w http.ResponseWriter, r *http.Request) {
	id, ok := examID(w, r)
	if !ok {
		return
	}
	reply(w)(struct{}{}, h.svc.AbandonExam(id))
}

// ---------- тренировка ----------

func (h handler) startTraining(w http.ResponseWriter, r *http.Request) {
	var sel bank.Selection
	if !decode(w, r, &sel) {
		return
	}
	reply(w)(h.svc.StartTraining(sel))
}

func (h handler) saveTrainingPosition(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Index int    `json:"index"`
	}
	if !decode(w, r, &body) {
		return
	}
	reply(w)(struct{}{}, h.svc.SaveTrainingPosition(body.Key, body.Index))
}

func (h handler) recordAnswer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QuestionID string `json:"question_id"`
		Chosen     int    `json:"chosen"`
		Mode       string `json:"mode"`
	}
	if !decode(w, r, &body) {
		return
	}
	reply(w)(h.svc.RecordAnswer(body.QuestionID, body.Chosen, body.Mode))
}

// ---------- общее ----------

func examID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "", fmt.Errorf("номер экзамена: %w", err))
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "", fmt.Errorf("тело запроса: %w", err))
		return false
	}
	return true
}

// reply отвечает результатом или ошибкой; вызывается как reply(w)(svc.X()).
// Ошибки экзамена получают код, по которому фронт решает, что показать.
func reply(w http.ResponseWriter) func(v any, err error) {
	return func(v any, err error) { send(w, v, err) }
}

func send(w http.ResponseWriter, v any, err error) {
	switch {
	case errors.Is(err, trainer.ErrExamActive):
		fail(w, http.StatusConflict, "exam_active", err)
	case errors.Is(err, trainer.ErrLeaseLost):
		fail(w, http.StatusConflict, "lease_lost", err)
	case errors.Is(err, trainer.ErrNoExam):
		fail(w, http.StatusNotFound, "no_exam", err)
	case errors.Is(err, trainer.ErrExamQuestion):
		fail(w, http.StatusConflict, "exam_question", err)
	case errors.Is(err, trainer.ErrNoPart):
		fail(w, http.StatusNotFound, "no_part", err)
	case err != nil:
		log.Printf("ошибка: %v", err)
		fail(w, http.StatusInternalServerError, "", err)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(v)
	}
}

func fail(w http.ResponseWriter, status int, code string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "code": code})
}
