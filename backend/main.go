// roadpolice-web — веб-тренажёр теории ПДД Армении: Go отдаёт API, картинки
// вопросов и собранный фронт.
//
// Настройка через переменные окружения:
//
//	RP_ADDR          адрес прослушивания        (по умолчанию :8080)
//	RP_BANK_DIR      bank.json, images/ и explanations.json (./bank)
//	RP_DATA_DIR      база прогресса trainer.db  (./data)
//	RP_FRONTEND_DIR  собранный фронт            (./public)
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"roadpolice-web/internal/api"
	"roadpolice-web/internal/bank"
	"roadpolice-web/internal/explain"
	"roadpolice-web/internal/store"
	"roadpolice-web/internal/trainer"
)

const noBankHint = "положите bank.json и images/ из roadpolice-trainer/assets в каталог банка (см. README)"

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := env("RP_ADDR", ":8080")
	bankDir := env("RP_BANK_DIR", "./bank")
	dataDir := env("RP_DATA_DIR", "./data")
	frontDir := env("RP_FRONTEND_DIR", "./public")

	raw, err := os.ReadFile(filepath.Join(bankDir, "bank.json"))
	if err != nil {
		log.Fatalf("банк вопросов: %v\n%s", err, noBankHint)
	}
	b, err := bank.Load(raw)
	if err != nil {
		log.Fatalf("банк вопросов: %v", err)
	}
	st, err := store.Open(filepath.Join(dataDir, "trainer.db"))
	if err != nil {
		log.Fatalf("база: %v", err)
	}
	defer st.Close()
	svc := trainer.New(b, st)
	ex, err := explain.Load(filepath.Join(bankDir, "explanations.json"))
	if err != nil {
		log.Fatalf("пояснения: %v", err)
	}
	svc.SetExplanations(ex)

	mux := http.NewServeMux()
	api.Routes(mux, svc)
	mux.Handle("GET /img/", imageHandler(filepath.Join(bankDir, "images")))
	mux.Handle("GET /", frontendHandler(frontDir))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("слушаю %s (банк: %d вопросов, пояснений: %d, данные: %s)", addr, len(b.Questions), len(ex.Items), dataDir)
	log.Fatal(srv.ListenAndServe())
}

// imageHandler отдаёт картинки вопросов. Имена файлов не меняются, пока
// не обновится банк, поэтому кешируем на сутки.
func imageHandler(dir string) http.Handler {
	files := http.StripPrefix("/img/", http.FileServer(http.Dir(dir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r) // без листинга каталога
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		files.ServeHTTP(w, r)
	})
}

// frontendHandler отдаёт собранный фронт. index.html не кешируется, чтобы
// после деплоя сразу подтягивались новые хеши ассетов.
func frontendHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
