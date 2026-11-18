package main

import (
	"fmt"
	"log"

	"app/internal/config"
	"app/internal/database"
	"app/internal/server"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.ConnectDB(cfg)
	if err != nil {
		log.Fatalf("Gagal terhubung ke database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("Gagal migrasi database: %v", err)
	}

	if err := database.Seed(db, cfg); err != nil {
		log.Fatalf("Gagal seed database: %v", err)
	}

	r := server.SetupRouter(cfg, db)

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("Server berjalan di http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Server berhenti karena error: %v", err)
	}
}
