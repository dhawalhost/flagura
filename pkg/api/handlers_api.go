package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/engine"
	"github.com/dhawalhost/flagura/pkg/store"
	"github.com/dhawalhost/flagura/pkg/telemetry"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	_, err := s.store.ListFlags(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unavailable","error":"storage not ready"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
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
		"version":     "1.1.0",
		"engine":      "Flagura-FastPath-Deterministic",
		"driver":      s.store.DriverName(),
		"timestamp":   time.Now().UTC(),
		"flags_count": len(flags),
	})
}

func (s *Server) resolveProjectID(r *http.Request) string {
	if p := r.Header.Get("X-Project-ID"); p != "" {
		return p
	}
	if p := r.URL.Query().Get("project_id"); p != "" {
		return p
	}
	if p := r.URL.Query().Get("projectId"); p != "" {
		return p
	}
	if c, err := r.Cookie("flagura_project_id"); err == nil && c.Value != "" {
		return c.Value
	}
	return store.DefaultProjectID
}

func (s *Server) handleGetFlags(w http.ResponseWriter, r *http.Request) {
	projectID := s.resolveProjectID(r)
	flags, err := s.store.ListFlagsByProject(r.Context(), projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"project_id": projectID,
		"flags":      flags,
		"count":      len(flags),
		"timestamp":  time.Now().UTC(),
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
	if flag.ProjectID == "" {
		flag.ProjectID = s.resolveProjectID(r)
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcastCurrentFlags(r.Context())

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
	if flag.ProjectID == "" {
		flag.ProjectID = s.resolveProjectID(r)
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcastCurrentFlags(r.Context())

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

	s.broadcastCurrentFlags(r.Context())

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

	s.broadcastCurrentFlags(r.Context())

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

	s.broadcastCurrentFlags(r.Context())

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
		Trace   bool                     `json:"trace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	includeTrace := req.Trace || r.URL.Query().Get("trace") == "true"

	projectID := s.resolveProjectID(r)
	allFlags, err := s.store.ListFlagsByProject(r.Context(), projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	results := make(map[string]domain.EvaluationResult)
	traces := make(map[string]engine.EvaluationTrace)
	targetMap := make(map[string]domain.FeatureFlag)
	for _, f := range allFlags {
		targetMap[f.Key] = f
		targetMap[f.ID] = f
	}

	if len(req.Flags) > 0 {
		for _, k := range req.Flags {
			if flag, ok := targetMap[k]; ok {
				if includeTrace {
					res, trace := engine.EvaluateFlagWithTrace(flag, req.Context)
					results[flag.Key] = res
					traces[flag.Key] = trace
					telemetry.RecordEvaluation(flag.Key, req.Context.Environment, res.Variant, res.Enabled, res.EvaluationLatencyNs)
				} else {
					res := engine.EvaluateFlag(flag, req.Context)
					results[flag.Key] = res
					telemetry.RecordEvaluation(flag.Key, req.Context.Environment, res.Variant, res.Enabled, res.EvaluationLatencyNs)
				}
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
			if includeTrace {
				res, trace := engine.EvaluateFlagWithTrace(flag, req.Context)
				results[flag.Key] = res
				traces[flag.Key] = trace
				telemetry.RecordEvaluation(flag.Key, req.Context.Environment, res.Variant, res.Enabled, res.EvaluationLatencyNs)
			} else {
				res := engine.EvaluateFlag(flag, req.Context)
				results[flag.Key] = res
				telemetry.RecordEvaluation(flag.Key, req.Context.Environment, res.Variant, res.Enabled, res.EvaluationLatencyNs)
			}
		}
	}

	durationUs := float64(time.Since(start).Nanoseconds()) / 1000.0
	atomic.AddUint64(&s.evalCount, uint64(len(results)))

	w.Header().Set("Content-Type", "application/json")
	if includeTrace {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"project_id":        projectID,
			"results":           results,
			"traces":            traces,
			"evaluated_count":   len(results),
			"total_duration_us": durationUs,
		})
		return
	}

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

	projectID := s.resolveProjectID(r)
	allFlags, err := s.store.ListFlagsByProject(r.Context(), projectID)
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

	projectID := s.resolveProjectID(r)
	logs, err := s.store.ListAuditLogsByProject(r.Context(), projectID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"project_id": projectID,
		"logs":       logs,
		"total":      len(logs),
	})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Reset(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.broadcastCurrentFlags(r.Context())
	flags, _ := s.store.ListFlags(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message":     "Database reset to default seed flags",
		"flags_count": len(flags),
	})
}

// handleWebhookKillSwitch engages the kill-switch for a flag triggered by automated alerts (e.g. Datadog/Sentry).
func (s *Server) handleWebhookKillSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Security: Webhook requests must be authenticated.
	// Check Authorization header (Bearer token), X-Webhook-Secret header, or ?token= query param.
	webhookSecret := os.Getenv("FLAGURA_WEBHOOK_SECRET")
	providedToken := ""

	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		providedToken = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if providedToken == "" {
		providedToken = r.Header.Get("X-Webhook-Secret")
	}
	if providedToken == "" {
		providedToken = r.Header.Get("X-API-Key")
	}
	if providedToken == "" {
		providedToken = r.URL.Query().Get("token")
	}

	authenticated := false

	// Check if matching webhook secret
	if webhookSecret != "" && providedToken != "" && subtle.ConstantTimeCompare([]byte(providedToken), []byte(webhookSecret)) == 1 {
		authenticated = true
	} else if providedToken != "" && webhookSecret == "" {
		// If no secret configured in env, verify against a valid user session or API token
		if sess, err := s.store.GetSession(r.Context(), providedToken); err == nil && sess != nil && !sess.IsExpired() {
			authenticated = true
		}
	}

	// Also check if caller has an active authenticated session cookie
	if !authenticated {
		if user, err := s.getUserFromRequest(r); err == nil && user != nil {
			authenticated = true
		}
	}

	if !authenticated {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Unauthorized",
			"message": "Webhook authentication failed. Provide a valid Bearer token, X-Webhook-Secret header, or active session.",
		})
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/api/v1/webhooks/kill-switch/")
	key = strings.TrimSpace(strings.Split(key, "/")[0])
	if key == "" {
		http.Error(w, "flag key is required in url path", http.StatusBadRequest)
		return
	}

	envStr := r.URL.Query().Get("env")
	if envStr == "" {
		envStr = "production"
	}
	env := domain.Environment(envStr)

	disabled := false
	actor := "webhook-alert-automation"

	flag, log, err := s.store.ToggleFlag(r.Context(), key, env, &disabled, actor)
	if err != nil {
		http.Error(w, "Flag not found or failed to disable: "+err.Error(), http.StatusNotFound)
		return
	}

	s.broadcastCurrentFlags(r.Context())

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "kill_switch_engaged",
		"flag_key":    flag.Key,
		"environment": env,
		"enabled":     false,
		"audit":       log,
	})
}

// handlePromoteEnvironment copies flag rules and configuration from one environment to another.
func (s *Server) handlePromoteEnvironment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	key = strings.TrimSuffix(key, "/promote")
	key = strings.TrimSpace(key)
	if key == "" {
		http.Error(w, "flag key is required", http.StatusBadRequest)
		return
	}

	fromEnv := domain.Environment(r.URL.Query().Get("from"))
	toEnv := domain.Environment(r.URL.Query().Get("to"))
	if fromEnv == "" || toEnv == "" {
		http.Error(w, "query parameters 'from' and 'to' are required", http.StatusBadRequest)
		return
	}

	flag, err := s.store.GetFlag(r.Context(), key)
	if err != nil {
		http.Error(w, "flag not found: "+err.Error(), http.StatusNotFound)
		return
	}

	srcConfig, ok := flag.Environments[fromEnv]
	if !ok {
		http.Error(w, fmt.Sprintf("source environment %q does not exist on flag", fromEnv), http.StatusBadRequest)
		return
	}

	// Clone source environment configuration
	rulesCopy := make([]domain.TargetingRule, len(srcConfig.Rules))
	for i, r := range srcConfig.Rules {
		vals := make([]string, len(r.Values))
		copy(vals, r.Values)
		rulesCopy[i] = domain.TargetingRule{
			ID:           r.ID,
			Name:         r.Name,
			Attribute:    r.Attribute,
			CustomKey:    r.CustomKey,
			Operator:     r.Operator,
			Values:       vals,
			Action:       r.Action,
			ServeVariant: r.ServeVariant,
		}
	}

	variantsCopy := make([]domain.FlagVariant, len(srcConfig.Variants))
	for i, v := range srcConfig.Variants {
		variantsCopy[i] = domain.FlagVariant{
			Key:         v.Key,
			Name:        v.Name,
			Value:       v.Value,
			Weight:      v.Weight,
			Description: v.Description,
		}
	}

	flag.Environments[toEnv] = domain.EnvironmentConfig{
		Enabled:        srcConfig.Enabled,
		Strategy:       srcConfig.Strategy,
		Percentage:     srcConfig.Percentage,
		Rules:          rulesCopy,
		Variants:       variantsCopy,
		DefaultVariant: srcConfig.DefaultVariant,
		OffVariant:     srcConfig.OffVariant,
	}
	flag.UpdatedAt = time.Now().UTC()

	actor := s.getActorFromRequest(r, "system")

	auditLog, err := s.store.SaveFlag(r.Context(), *flag, actor)
	if err != nil {
		http.Error(w, "failed to promote environment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcastCurrentFlags(r.Context())

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "promoted",
		"flag_key": flag.Key,
		"from":     fromEnv,
		"to":       toEnv,
		"audit":    auditLog,
	})
}
