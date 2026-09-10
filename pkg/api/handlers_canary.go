package api

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
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

		// Authenticate: caller must have valid user/API key session OR matching FLAGURA_WEBHOOK_SECRET
		webhookSecret := os.Getenv("FLAGURA_WEBHOOK_SECRET")
		isWebhookAuthed := false
		if webhookSecret != "" {
			secBytes := []byte(webhookSecret)
			headerSec := []byte(r.Header.Get("X-Webhook-Secret"))
			querySec := []byte(r.URL.Query().Get("token"))
			bearerSec := []byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))

			if subtle.ConstantTimeCompare(headerSec, secBytes) == 1 ||
				subtle.ConstantTimeCompare(querySec, secBytes) == 1 ||
				subtle.ConstantTimeCompare(bearerSec, secBytes) == 1 {
				isWebhookAuthed = true
			}
		}

		user, _ := s.getUserFromRequest(r)
		if !isWebhookAuthed && user == nil {
			slog.WarnContext(r.Context(), "security_event",
				slog.String("event_type", "canary_unauthorized"),
				slog.String("ip", GetClientIP(r)),
				slog.String("path", r.URL.Path),
				slog.String("method", r.Method),
				slog.String("user_agent", r.UserAgent()),
				slog.String("reason", "invalid_webhook_secret_or_session"),
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "Unauthorized",
				"message": "Authentication required. Provide a valid session, Bearer token, or X-Webhook-Secret.",
			})
			return
		}

		var projectID string
		if user != nil {
			var err error
			projectID, err = s.resolveAndAuthorizeProjectID(r)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
		} else {
			projectID = s.resolveProjectID(r)
		}

		s.handleCanaryRollback(w, r, projectID, flagKey)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			projectID, err := s.resolveAndAuthorizeProjectID(r)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			s.handleCreateCanary(w, r, projectID, flagKey)
		})(w, r)
	case http.MethodGet:
		s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			projectID, err := s.resolveAndAuthorizeProjectID(r)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			s.handleGetCanary(w, r, projectID, flagKey)
		})(w, r)
	case http.MethodDelete:
		s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			projectID, err := s.resolveAndAuthorizeProjectID(r)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			s.handleDeleteCanary(w, r, projectID, flagKey)
		})(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCreateCanary(w http.ResponseWriter, r *http.Request, projectID, flagKey string) {
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
	req.ProjectID = projectID

	sched, err := s.canary.SubmitSchedule(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(sched)
}

func (s *Server) handleGetCanary(w http.ResponseWriter, r *http.Request, projectID, flagKey string) {
	if s.canary == nil {
		http.Error(w, "Canary scheduler not initialized", http.StatusInternalServerError)
		return
	}

	sched, ok := s.canary.GetSchedule(projectID, flagKey)
	if !ok {
		http.Error(w, "No active canary schedule for flag "+flagKey, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sched)
}

func (s *Server) handleDeleteCanary(w http.ResponseWriter, r *http.Request, projectID, flagKey string) {
	if s.canary == nil {
		http.Error(w, "Canary scheduler not initialized", http.StatusInternalServerError)
		return
	}

	cancelled := s.canary.CancelSchedule(projectID, flagKey)
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

func (s *Server) handleCanaryRollback(w http.ResponseWriter, r *http.Request, projectID, flagKey string) {
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

	if err := s.canary.TriggerHealthRollback(r.Context(), projectID, flagKey, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":     "rolled_back",
		"project_id": projectID,
		"flag_key":   flagKey,
		"reason":     req.Reason,
	})
}
