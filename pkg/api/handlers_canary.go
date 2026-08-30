package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// handleCanaryRoutes dispatches /api/v1/flags/:key/canary requests.
func (s *Server) handleCanaryRoutes(w http.ResponseWriter, r *http.Request) {
	// Extract flag key from /api/v1/flags/:key/canary or /api/v1/flags/:key/canary/rollback
	path := r.URL.Path
	trimmed := strings.TrimPrefix(path, "/api/v1/flags/")

	isRollback := strings.HasSuffix(trimmed, "/canary/rollback")
	flagKey := strings.TrimSuffix(trimmed, "/canary/rollback")
	flagKey = strings.TrimSuffix(flagKey, "/canary")
	flagKey = strings.Trim(flagKey, "/")

	if flagKey == "" {
		http.Error(w, "Flag key is required", http.StatusBadRequest)
		return
	}

	if isRollback {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleCanaryRollback(w, r, flagKey)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			s.handleCreateCanary(w, r, flagKey)
		})(w, r)
	case http.MethodGet:
		s.handleGetCanary(w, r, flagKey)
	case http.MethodDelete:
		s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			s.handleDeleteCanary(w, r, flagKey)
		})(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCreateCanary(w http.ResponseWriter, r *http.Request, flagKey string) {
	if s.canary == nil {
		http.Error(w, "Canary scheduler not initialized", http.StatusInternalServerError)
		return
	}

	var req domain.CanarySchedule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.FlagKey = flagKey

	sched, err := s.canary.SubmitSchedule(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(sched)
}

func (s *Server) handleGetCanary(w http.ResponseWriter, r *http.Request, flagKey string) {
	if s.canary == nil {
		http.Error(w, "Canary scheduler not initialized", http.StatusInternalServerError)
		return
	}

	sched, ok := s.canary.GetSchedule(flagKey)
	if !ok {
		http.Error(w, "No active canary schedule for flag "+flagKey, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sched)
}

func (s *Server) handleDeleteCanary(w http.ResponseWriter, r *http.Request, flagKey string) {
	if s.canary == nil {
		http.Error(w, "Canary scheduler not initialized", http.StatusInternalServerError)
		return
	}

	cancelled := s.canary.CancelSchedule(flagKey)
	if !cancelled {
		http.Error(w, "No active canary schedule found to cancel", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "cancelled",
		"flag_key": flagKey,
	})
}

func (s *Server) handleCanaryRollback(w http.ResponseWriter, r *http.Request, flagKey string) {
	if s.canary == nil {
		http.Error(w, "Canary scheduler not initialized", http.StatusInternalServerError)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "External APM Alert Triggered Rollback"
	}

	if err := s.canary.TriggerHealthRollback(r.Context(), flagKey, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "rolled_back",
		"flag_key": flagKey,
		"reason":   req.Reason,
	})
}
