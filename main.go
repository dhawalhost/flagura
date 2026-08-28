package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dhawalhost/flagura/internal/api"
	"github.com/dhawalhost/flagura/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	var st store.Store
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		pgStore, err := store.NewPostgresStore(dbURL)
		if err != nil {
			log.Printf("[WARN] Failed to connect to PostgreSQL: %v. Falling back to in-memory store.\n", err)
			st = store.NewMemoryStore()
		} else {
			log.Printf("[INFO] Connected to %s successfully.\n", pgStore.DriverName())
			st = pgStore
		}
	} else {
		log.Println("[INFO] DATABASE_URL not set. Running with In-Memory Edge Store.")
		st = store.NewMemoryStore()
	}

	server, err := api.NewServer(st)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize Flagura server: %v", err)
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		fmt.Printf("\n🚀 Flagura Engine running on http://localhost:%s\n", port)
		fmt.Printf("   ├── Storage Driver: %s\n", st.DriverName())
		fmt.Printf("   ├── Fast-Path Evaluator: FNV-1a 64-bit Deterministic\n")
		fmt.Printf("   └── UI Endpoints: / (Landing), /dashboard (Console), /auth (Login)\n\n")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[INFO] Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Server forced to shutdown: %v", err)
	}
	log.Println("[INFO] Server exited successfully.")
}
