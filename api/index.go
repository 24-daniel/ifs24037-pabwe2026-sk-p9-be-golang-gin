package handler

import (
	"log"
	"net/http"
	"sync"

	"app/internal/config"
	"app/internal/database"
	"app/internal/server"
)

var (
	mu     sync.Mutex
	router http.Handler
)

// initRouter menyiapkan koneksi DB, migrate, seed, dan router sekali per instance.
// Jika gagal, request berikutnya akan mencoba lagi.
func initRouter() (http.Handler, error) {
	mu.Lock()
	defer mu.Unlock()

	if router != nil {
		return router, nil
	}

	cfg := config.LoadConfig()

	db, err := database.ConnectDB(cfg)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, err
	}
	if err := database.Seed(db, cfg); err != nil {
		return nil, err
	}

	router = server.SetupRouter(cfg, db)
	return router, nil
}

// Handler adalah entry point yang dipanggil Vercel.
func Handler(w http.ResponseWriter, r *http.Request) {
	rt, err := initRouter()
	if err != nil {
		log.Printf("init gagal: %v", err)
		http.Error(w, `{"status":"error","message":"Server gagal inisialisasi"}`, http.StatusInternalServerError)
		return
	}
	rt.ServeHTTP(w, r)
}