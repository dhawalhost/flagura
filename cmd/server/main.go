package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/config"
	"github.com/dhawalhost/flagura/pkg/store"
)

func isSQLite(dbURL string) bool {
	u := strings.ToLower(strings.TrimSpace(dbURL))
	return strings.HasPrefix(u, "sqlite://") ||
		strings.HasPrefix(u, "sqlite3://") ||
		strings.HasPrefix(u, "file:") ||
		strings.HasSuffix(u, ".db") ||
		strings.HasSuffix(u, ".sqlite") ||
		strings.HasSuffix(u, ".sqlite3")
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize structured logger
	var logHandler slog.Handler
	if cfg.LogFormat == "json" {
		logHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	} else {
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	}
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	var st store.Store
	if isSQLite(cfg.DatabaseURL) {
		sqliteStore, err := store.NewSQLiteStore(cfg.DatabaseURL)
		if err != nil {
			slog.Warn("Failed to initialize SQLite store. Falling back to In-Memory Edge Store", slog.Any("error", err))
			st = store.NewMemoryStore()
		} else {
			slog.Info("Connected to embedded database successfully", slog.String("driver", sqliteStore.DriverName()))
			st = sqliteStore
		}
	} else if cfg.DatabaseURL != "" {
		pgStore, err := store.NewPostgresStore(cfg.DatabaseURL)
		if err != nil {
			slog.Warn("Failed to connect to PostgreSQL. Falling back to In-Memory Edge Store", slog.Any("error", err))
			st = store.NewMemoryStore()
		} else {
			slog.Info("Connected to database successfully", slog.String("driver", pgStore.DriverName()))
			st = pgStore
		}
	} else {
		slog.Info("DATABASE_URL not set. Running with In-Memory Edge Store")
		st = store.NewMemoryStore()
	}

	server, err := api.NewServer(st)
	if err != nil {
		slog.Error("Failed to initialize Flagura server", slog.Any("error", err))
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           server,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}

	go func() {
		fmt.Printf("\n🚀 Flagura Server running on http://localhost:%s\n", cfg.ServerPort)
		fmt.Printf("   ├── Environment: %s\n", cfg.Environment)
		fmt.Printf("   ├── Storage Driver: %s\n", st.DriverName())
		fmt.Printf("   ├── Fast-Path Evaluator: FNV-1a 64-bit Deterministic\n")
		fmt.Printf("   ├── Structured Logging: %s (level: %s)\n", cfg.LogFormat, cfg.LogLevel)
		fmt.Printf("   └── UI Endpoints: / (Landing), /dashboard (Console), /auth (Login)\n\n")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server encountered fatal error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down Flagura server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("Flagura server exited successfully")
}
