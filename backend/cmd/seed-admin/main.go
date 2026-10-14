package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/database"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err.Error())
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database unavailable")
		os.Exit(1)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err = database.Migrate(ctx, db); err != nil {
		log.Error("migration failed")
		os.Exit(1)
	}
	created, err := auth.New(cfg, db).SeedAdmin(ctx)
	if err != nil {
		log.Error("administrator initialization failed", "error", err.Error())
		os.Exit(1)
	}
	if created {
		log.Info("administrator created; complete profile after login")
	} else {
		log.Info("administrator already exists; account unchanged")
	}
}
