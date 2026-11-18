package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"app/internal/config"
	"app/internal/database"
	"app/internal/server"

	"gorm.io/gorm"
)

var (
	mu     sync.Mutex
	router http.Handler
)

// schemaLockKey adalah kunci advisory lock PostgreSQL agar migrate/seed tidak
// berjalan bersamaan dari banyak instance serverless (cold start paralel).
const schemaLockKey = 90210

// schemaReady true bila tabel dan akun bawaan (admin & member) sudah ada,
// sehingga migrate + seed yang lambat bisa dilewati pada cold start berikutnya.
func schemaReady(db *gorm.DB, cfg *config.Config) bool {
	m := db.Migrator()
	if !m.HasTable(&database.User{}) || !m.HasTable(&database.Post{}) {
		return false
	}
	adminUsername := cfg.AdminUsername
	if adminUsername == "" {
		adminUsername = "admin"
	}
	var count int64
	if err := db.Model(&database.User{}).
		Where("username IN ?", []string{adminUsername, "member"}).
		Count(&count).Error; err != nil {
		return false
	}
	return count >= 2
}

// prepareSchema menjalankan migrate + seed hanya bila perlu, dan diserialkan
// antar-instance memakai advisory lock tingkat transaksi (aman untuk pooler).
func prepareSchema(db *gorm.DB, cfg *config.Config) error {
	if schemaReady(db, cfg) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", schemaLockKey).Error; err != nil {
			return err
		}
		if err := database.Migrate(tx); err != nil {
			return err
		}
		return database.Seed(tx, cfg)
	})
}

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

	// Serverless: banyak instance berjalan paralel, jaga agar jumlah koneksi per instance kecil
	// supaya batas koneksi database tidak habis.
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(3)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
	}

	if err := prepareSchema(db, cfg); err != nil {
		return nil, err
	}

	router = server.SetupRouter(cfg, db)
	return router, nil
}

// restoreOriginalPath mengembalikan path & query asli request.
// vercel.json me-rewrite semua request ke /api?__p=<path-asli>, karena fungsi
// hanya menerima path hasil rewrite ("/api"), bukan path yang diminta klien.
// Nilai placeholder yang tidak terganti (":p", "$1", dst) diabaikan.
func restoreOriginalPath(r *http.Request) {
	q := r.URL.Query()
	p, ok := q["__p"]
	if ok {
		q.Del("__p")
		r.URL.RawQuery = q.Encode()
		if !strings.HasPrefix(p[0], ":") && !strings.HasPrefix(p[0], "$") {
			r.URL.Path = "/" + strings.TrimPrefix(p[0], "/")
			r.URL.RawPath = ""
			r.RequestURI = r.URL.RequestURI()
			return
		}
	}
	if r.URL.Path == "/api" || r.URL.Path == "/api/index" {
		r.URL.Path = "/"
	}
}

// Handler adalah entry point yang dipanggil Vercel.
func Handler(w http.ResponseWriter, r *http.Request) {
	seen := r.URL.String()
	if r.URL.Query().Get("debug") == "1" {
		// Mode diagnosis: tampilkan persis apa yang diterima fungsi dari Vercel.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"method":      r.Method,
			"url":         seen,
			"path":        r.URL.Path,
			"raw_query":   r.URL.RawQuery,
			"request_uri": r.RequestURI,
			"host":        r.Host,
			"headers":     r.Header,
		})
		return
	}
	restoreOriginalPath(r)
	w.Header().Set("X-Debug-Path", seen+" => "+r.URL.Path) // bantu diagnosis routing; aman dihapus
	rt, err := initRouter()
	if err != nil {
		log.Printf("init gagal: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error","message":"Server gagal inisialisasi"}`))
		return
	}
	rt.ServeHTTP(w, r)
}
