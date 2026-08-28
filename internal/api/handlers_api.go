package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/internal/domain"
	"github.com/dhawalhost/flagura/internal/engine"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	flags, _ := s.store.ListFlags(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"service":     "flagura-engine",
		"version":     "1.22.4",
		"engine":      "Flagura-FastPath-Deterministic",
		"driver":      s.store.DriverName(),
		"timestamp":   time.Now().UTC(),
		"flags_count": len(flags),
	})
}

func (s *Server) handleGetFlags(w http.ResponseWriter, r *http.Request) {
	flags, err := s.store.ListFlags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"flags":     flags,
		"count":     len(flags),
		"timestamp": time.Now().UTC(),
	})
}

func (s *Server) getActorFromRequest(r *http.Request, fallback string) string {
	if u := UserFromContext(r.Context()); u != nil && u.Email != "" {
		return u.Email
	}
	if a := r.Header.Get("X-Actor"); a != "" {
		return a
	}
	if a := r.URL.Query().Get("actor"); a != "" {
		return a
	}
	if fallback != "" {
		return fallback
	}
	return "developer@flagura.dev"
}

func (s *Server) handleCreateFlag(w http.ResponseWriter, r *http.Request) {
	var flag domain.FeatureFlag
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if flag.Key == "" {
		http.Error(w, "flag key is required", http.StatusBadRequest)
		return
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"flag":  flag,
		"audit": log,
	})
}

func (s *Server) handleUpdateFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.Split(id, "/")[0]

	var flag domain.FeatureFlag
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if flag.Key == "" {
		flag.Key = id
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"flag":  flag,
		"audit": log,
	})
}

func (s *Server) handleDeleteFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.Split(id, "/")[0]

	actor := s.getActorFromRequest(r, "admin@flagura.dev")

	log, err := s.store.DeleteFlag(r.Context(), id, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"deleted": id,
		"audit":   log,
	})
}

func (s *Server) handleToggleFlag(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/flags/{id}/toggle
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/flags/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	id := parts[0]

	var req struct {
		Environment domain.Environment `json:"environment"`
		Enabled     *bool              `json:"enabled"`
		Actor       string             `json:"actor"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}

	actor := s.getActorFromRequest(r, req.Actor)

	flag, log, err := s.store.ToggleFlag(r.Context(), id, req.Environment, req.Enabled, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"flag_key":    flag.Key,
		"environment": req.Environment,
		"enabled":     flag.Environments[req.Environment].Enabled,
		"flag":        flag,
		"audit":       log,
	})
}

func (s *Server) handleUpdateRollout(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/flags/{id}/rollout
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/flags/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	id := parts[0]

	var req struct {
		Environment domain.Environment `json:"environment"`
		Percentage  float64            `json:"percentage"`
		Actor       string             `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}

	actor := s.getActorFromRequest(r, req.Actor)

	flag, log, err := s.store.UpdateRollout(r.Context(), id, req.Environment, req.Percentage, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"flag_key":    flag.Key,
		"environment": req.Environment,
		"percentage":  req.Percentage,
		"flag":        flag,
		"audit":       log,
	})
}

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req struct {
		Flags   []string                 `json:"flags"`
		Context domain.EvaluationContext `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	allFlags, err := s.store.ListFlags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	results := make(map[string]domain.EvaluationResult)
	targetMap := make(map[string]domain.FeatureFlag)
	for _, f := range allFlags {
		targetMap[f.Key] = f
		targetMap[f.ID] = f
	}

	if len(req.Flags) > 0 {
		for _, k := range req.Flags {
			if flag, ok := targetMap[k]; ok {
				res := engine.EvaluateFlag(flag, req.Context)
				results[flag.Key] = res
			} else {
				results[k] = domain.EvaluationResult{
					FlagKey:             k,
					Enabled:             false,
					Variant:             "off",
					Value:               false,
					Reason:              domain.ReasonFlagNotFound,
					EvaluationLatencyNs: 65,
					EvaluationLatencyUs: 0.065,
				}
			}
		}
	} else {
		for _, flag := range allFlags {
			res := engine.EvaluateFlag(flag, req.Context)
			results[flag.Key] = res
		}
	}

	durationUs := float64(time.Since(start).Nanoseconds()) / 1000.0

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(domain.BatchEvaluationResponse{
		Results:         results,
		TotalFlags:      len(results),
		Environment:     req.Context.Environment,
		TotalDurationUs: durationUs,
		EvaluatedAt:     time.Now().UTC(),
	})
}

func (s *Server) handleBenchmark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Iterations  int                `json:"iterations"`
		Environment domain.Environment `json:"environment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Iterations <= 0 {
		req.Iterations = 10000
	}
	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}

	allFlags, err := s.store.ListFlags(r.Context())
	if err != nil || len(allFlags) == 0 {
		http.Error(w, "no flags available for benchmark", http.StatusInternalServerError)
		return
	}

	targetFlag := allFlags[0]
	metrics := engine.RunBenchmark(targetFlag, req.Environment, req.Iterations)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}

	logs, err := s.store.ListAuditLogs(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":  logs,
		"total": len(logs),
	})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Reset(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	flags, _ := s.store.ListFlags(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message":     "Database reset to default seed flags",
		"flags_count": len(flags),
	})
}
