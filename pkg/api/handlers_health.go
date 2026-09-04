package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/telemetry"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "alive",
		"service":   "flagura",
		"timestamp": time.Now().UTC(),
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status":    "unavailable",
			"error":     err.Error(),
			"timestamp": time.Now().UTC(),
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ready",
		"driver":    s.store.DriverName(),
		"timestamp": time.Now().UTC(),
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	telemetry.PrometheusHandler(s.store)(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	flags, _ := s.store.ListFlags(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"service":     "flagura-engine",
		"version":     domain.CleanVersion(),
		"engine":      "Flagura-FastPath-Deterministic",
		"driver":      s.store.DriverName(),
		"timestamp":   time.Now().UTC(),
		"flags_count": len(flags),
	})
}
