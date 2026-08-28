package handler

import (
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/dhawalhost/flagura/internal/api"
	"github.com/dhawalhost/flagura/internal/store"
)

var (
	serverOnce sync.Once
	server     *api.Server
)

func initServer() {
	var st store.Store
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		pgStore, err := store.NewPostgresStore(dbURL)
		if err != nil {
			log.Printf("[WARN] Failed to connect to PostgreSQL: %v. Falling back to in-memory store.", err)
			st = store.NewMemoryStore()
		} else {
			// #nosec G706 -- DriverName returns a trusted constant enumerated string
			log.Printf("[INFO] Successfully connected to %s", pgStore.DriverName())
			st = pgStore
		}
	} else {
		log.Println("[INFO] DATABASE_URL not set. Running with In-Memory Edge Store.")
		st = store.NewMemoryStore()
	}

	var err error
	server, err = api.NewServer(st)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize Flagura server: %v", err)
	}
}

// Handler is the Vercel serverless function entrypoint
func Handler(w http.ResponseWriter, r *http.Request) {
	serverOnce.Do(initServer)

	// Restore original request path from Vercel rewrite headers
	if matchedPath := r.Header.Get("x-matched-path"); matchedPath != "" {
		r.URL.Path = matchedPath
	} else if vercelPath := r.Header.Get("x-vercel-matched-path"); vercelPath != "" {
		r.URL.Path = vercelPath
	} else if forwardedURI := r.Header.Get("x-forwarded-uri"); forwardedURI != "" {
		r.URL.Path = forwardedURI
	} else if r.URL.Path == "/api" || r.URL.Path == "/api/" || r.URL.Path == "/api/index" || r.URL.Path == "/api/index.go" {
		r.URL.Path = "/"
	}

	server.ServeHTTP(w, r)
}
