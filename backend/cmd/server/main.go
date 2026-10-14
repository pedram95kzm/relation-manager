package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"relations/internal/analysis"
	"relations/internal/auth"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/httpapi"
	"relations/internal/maintenance"
	"relations/internal/questionnaire"
)

func main() {
	cfg, e := config.Load()
	if e != nil {
		slog.Error("invalid configuration", "error", e.Error())
		os.Exit(1)
	}
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	boot, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, e := database.Open(boot, cfg.DatabaseURL)
	if e != nil {
		log.Error("database unavailable")
		os.Exit(1)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if e = database.Migrate(boot, db); e != nil {
		log.Error("migration failed", "error", e.Error())
		os.Exit(1)
	}
	if e = questionnaire.Seed(boot, db); e != nil {
		log.Error("questionnaire catalog initialization failed", "error", e.Error())
		os.Exit(1)
	}
	analyses := analysis.New(cfg, db, nil)
	if e = analyses.InitializeSettings(boot); e != nil {
		log.Error("settings initialization failed")
		os.Exit(1)
	}
	if _, e = analyses.RecoverStale(boot); e != nil {
		log.Error("analysis recovery failed")
		os.Exit(1)
	}
	cleaner := maintenance.New(db)
	if _, e = cleaner.Cleanup(boot); e != nil {
		log.Error("session maintenance failed")
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		log.Info("migrations applied")
		return
	}
	if cfg.SeedDefaultAccounts {
		if e = auth.New(cfg, db).SeedDefaults(boot); e != nil {
			log.Error("default account initialization failed", "error", e.Error())
			os.Exit(1)
		}
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.New(cfg, db, log), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				recovery, done := context.WithTimeout(ctx, 10*time.Second)
				if _, err := analyses.RecoverStale(recovery); err != nil {
					log.Error("analysis recovery failed")
				}
				if _, err := cleaner.Cleanup(recovery); err != nil {
					log.Error("session maintenance failed")
				}
				done()
			}
		}
	}()
	listener, e := net.Listen("tcp", cfg.HTTPAddr)
	if e != nil {
		log.Error("HTTP listener unavailable")
		os.Exit(1)
	}
	go func() {
		log.Info("server started", "address", cfg.HTTPAddr)
		if e := server.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Error("server failed")
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	_ = server.Shutdown(shutdown)
}
