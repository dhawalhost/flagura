package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/store"
)

var (
	serverMu sync.RWMutex
	server   *api.Server
)

func getServer() (*api.Server, error) {
	serverMu.RLock()
	if server != nil {
		s := server
		serverMu.RUnlock()
		return s, nil
	}
	serverMu.RUnlock()

	serverMu.Lock()
	defer serverMu.Unlock()

	if server != nil {
		return server, nil
	}

	var st store.Store
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		pgStore, err := store.NewPostgresStore(dbURL)
		if err != nil {
			// In production serverless environments, falling back silently to an ephemeral
			// in-memory store causes split-brain state (sessions exist on some lambdas but not others).
			// Allow fallback only if explicitly opted in via ALLOW_MEMORY_FALLBACK=true.
			if strings.EqualFold(os.Getenv("ALLOW_MEMORY_FALLBACK"), "true") {
				log.Printf("[WARN] Failed to connect to PostgreSQL: %v. Explicit fallback to in-memory store allowed.", err)
				st = store.NewMemoryStore()
			} else {
				log.Printf("[ERROR] Failed to connect to PostgreSQL in serverless environment: %v", err)
				return nil, fmt.Errorf("database connection unavailable: %w", err)
			}
		} else {
			// #nosec G706 -- DriverName returns a trusted constant enumerated string
			log.Printf("[INFO] Successfully connected to %s", pgStore.DriverName())
			st = pgStore
		}
	} else {
		log.Println("[INFO] DATABASE_URL not set. Running with In-Memory Edge Store.")
		st = store.NewMemoryStore()
	}

	srv, err := api.NewServer(st)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Flagura server: %w", err)
	}

	server = srv
	return server, nil
}

func initServer() {
	_, _ = getServer()
}

// Handler is the Vercel serverless function entrypoint
func Handler(w http.ResponseWriter, r *http.Request) {
	srv, err := getServer()
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Service Unavailable",
			"message": "Database connection establishing or temporarily unavailable. Please retry.",
		})
		return
	}
	srv.ServeHTTP(w, r)
}
