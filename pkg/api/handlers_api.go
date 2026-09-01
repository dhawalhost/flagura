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
		"version":     "1.1.0",
		"engine":      "Flagura-FastPath-Deterministic",
		"driver":      s.store.DriverName(),
		"timestamp":   time.Now().UTC(),
		"flags_count": len(flags),
	})
}

func (s *Server) resolveProjectID(r *http.Request) string {
	if p := r.Header.Get(domain.HeaderProjectID); p != "" {
		return p
	}
	if p := r.URL.Query().Get("project_id"); p != "" {
		return p
	}
	if p := r.URL.Query().Get("projectId"); p != "" {
		return p
	}
	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil && apiKey.ProjectID != "" {
		return apiKey.ProjectID
	}
	if c, err := r.Cookie(domain.CookieProjectName); err == nil && c.Value != "" {
		return c.Value
	}
	if u := UserFromContext(r.Context()); u != nil {
		if orgs, err := s.store.ListUserOrganizations(r.Context(), u.ID); err == nil && len(orgs) > 0 {
			for _, org := range orgs {
				if projs, err := s.store.ListProjects(r.Context(), org.ID); err == nil && len(projs) > 0 {
					return projs[0].ID
				}
			}
		}
	}
	return ""
}

func (s *Server) handleGetFlags(w http.ResponseWriter, r *http.Request) {
	projectID := s.resolveProjectID(r)
	if projectID == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectRequired, "project_id is required via X-Project-ID header, project_id query parameter, or active session", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}
	flags, err := s.store.ListFlagsByProject(r.Context(), projectID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
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
	if a := r.Header.Get(domain.HeaderActor); a != "" {
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
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}
	if flag.Key == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "flag key is required", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}
	if flag.ProjectID == "" {
		flag.ProjectID = s.resolveProjectID(r)
	}
	if flag.ProjectID == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectRequired, "project_id is required to create a feature flag", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, "")

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"flag":  flag,
		"audit": log,
	})
}

func (s *Server) handleUpdateFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.Split(id, "/")[0]

	var flag domain.FeatureFlag
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	if flag.Key == "" {
		flag.Key = id
	}
	if flag.ProjectID == "" {
		flag.ProjectID = s.resolveProjectID(r)
	}
	if flag.ProjectID == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectRequired, "project_id is required to update a feature flag", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, "")

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"flag":  flag,
		"audit": log,
	})
}

func (s *Server) handleDeleteFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.Split(id, "/")[0]

	actor := s.getActorFromRequest(r, "admin@flagura.dev")
	projectID := s.resolveProjectID(r)

	log, err := s.store.DeleteFlag(r.Context(), id, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}

	s.broadcastCurrentFlags(r.Context(), projectID, "")

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"deleted": id,
		"audit":   log,
	})
}

func (s *Server) handleToggleFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.TrimSuffix(id, "/toggle")

	var req struct {
		Environment domain.Environment `json:"environment"`
		Enabled     *bool              `json:"enabled"`
		Actor       string             `json:"actor"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}

	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil {
		if !apiKey.AllowsEnvironment(req.Environment) {
			s.writeError(w, r, domain.NewAppError(
				domain.ErrCodeEnvironmentRestricted,
				fmt.Sprintf("API key is scoped to environment '%s' and cannot modify '%s'", apiKey.Environment, req.Environment),
				http.StatusForbidden,
				domain.ErrEnvironmentRestricted,
			))
			return
		}
	}

	actor := s.getActorFromRequest(r, req.Actor)

	flag, log, err := s.store.ToggleFlag(r.Context(), id, req.Environment, req.Enabled, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, req.Environment)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"flag_key":    flag.Key,
		"environment": req.Environment,
		"enabled":     flag.Environments[req.Environment].Enabled,
		"flag":        flag,
		"audit":       log,
	})
}

func (s *Server) handleUpdateRollout(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.TrimSuffix(id, "/rollout")

	var req struct {
		Environment domain.Environment `json:"environment"`
		Percentage  float64            `json:"percentage"`
		Actor       string             `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}

	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil {
		if !apiKey.AllowsEnvironment(req.Environment) {
			s.writeError(w, r, domain.NewAppError(
				domain.ErrCodeEnvironmentRestricted,
				fmt.Sprintf("API key is scoped to environment '%s' and cannot modify '%s'", apiKey.Environment, req.Environment),
				http.StatusForbidden,
				domain.ErrEnvironmentRestricted,
			))
			return
		}
	}

	actor := s.getActorFromRequest(r, req.Actor)

	flag, log, err := s.store.UpdateRollout(r.Context(), id, req.Environment, req.Percentage, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, req.Environment)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
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
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	// Environment validation for API Key authentication
	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil {
		if req.Context.Environment == "" && apiKey.Environment != "" && apiKey.Environment != string(domain.EnvAll) && apiKey.Environment != "*" {
			req.Context.Environment = domain.Environment(apiKey.Environment)
		}
		if req.Context.Environment != "" && !apiKey.AllowsEnvironment(req.Context.Environment) {
			s.writeError(w, r, domain.NewAppError(
				domain.ErrCodeEnvironmentRestricted,
				fmt.Sprintf("API key is scoped to environment '%s' and cannot access '%s'", apiKey.Environment, req.Context.Environment),
				http.StatusForbidden,
				domain.ErrEnvironmentRestricted,
			))
			return
		}
	}

	if req.Context.Environment == "" {
		req.Context.Environment = domain.EnvProduction
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
		apiKey := s.getAPIKeyFromRequest(r)
		user, _ := s.getUserFromRequest(r)
		if apiKey == nil && user == nil {
			s.writeError(w, r, domain.NewAppError(
				domain.ErrCodeUnauthorized,
				"Bulk evaluation without specific flag keys requires an authenticated API key or session credential",
				http.StatusUnauthorized,
				domain.ErrUnauthorized,
			))
			return
		}
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
	projectID := s.resolveProjectID(r)
	if err := s.store.Reset(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.broadcastCurrentFlags(r.Context(), projectID, "")
	flags, _ := s.store.ListFlagsByProject(r.Context(), projectID)
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

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, env)

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

	projectID := s.resolveProjectID(r)
	flag, err := s.store.GetFlagByProject(r.Context(), projectID, key)
	if err != nil {
		flag, err = s.store.GetFlag(r.Context(), key)
		if err != nil {
			http.Error(w, "flag not found: "+err.Error(), http.StatusNotFound)
			return
		}
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

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, toEnv)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "promoted",
		"flag_key": flag.Key,
		"from":     fromEnv,
		"to":       toEnv,
		"audit":    auditLog,
	})
}

func (s *Server) handleInvitations(w http.ResponseWriter, r *http.Request) {
	user, err := s.getUserFromRequest(r)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeUnauthorized, "Unauthorized", http.StatusUnauthorized, err))
		return
	}

	switch r.Method {
	case http.MethodGet:
		orgID := r.URL.Query().Get("organization_id")
		if orgID == "" {
			orgs, _ := s.store.ListUserOrganizations(r.Context(), user.ID)
			if len(orgs) > 0 {
				orgID = orgs[0].ID
			}
		}
		invitations, err := s.store.ListOrgInvitations(r.Context(), orgID)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, err.Error(), http.StatusInternalServerError, err))
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"invitations": invitations,
		})

	case http.MethodPost:
		var req struct {
			OrganizationID string `json:"organization_id"`
			Email          string `json:"email"`
			Role           string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
			return
		}

		if req.OrganizationID == "" {
			orgs, _ := s.store.ListUserOrganizations(r.Context(), user.ID)
			if len(orgs) > 0 {
				req.OrganizationID = orgs[0].ID
			}
		}
		if req.Role == "" {
			req.Role = "developer"
		}

		org, err := s.store.GetOrganization(r.Context(), req.OrganizationID)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, "Organization not found", http.StatusNotFound, err))
			return
		}

		inv := domain.OrgInvitation{
			OrganizationID: org.ID,
			OrgName:        org.Name,
			Email:          strings.TrimSpace(strings.ToLower(req.Email)),
			Role:           req.Role,
			InvitedBy:      user.Email,
		}

		createdInv, err := s.store.CreateOrgInvitation(r.Context(), inv)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, err.Error(), http.StatusInternalServerError, err))
			return
		}

		s.writeJSON(w, http.StatusCreated, map[string]interface{}{
			"invitation": createdInv,
			"invite_url": fmt.Sprintf("/auth?invite=%s", createdInv.Token),
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetInvitationByToken(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.Error(w, "invalid invitation token path", http.StatusBadRequest)
		return
	}
	token := parts[len(parts)-1]
	inv, err := s.store.GetOrgInvitation(r.Context(), token)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, err.Error(), http.StatusNotFound, err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"invitation": inv,
	})
}

func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := s.getUserFromRequest(r)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeUnauthorized, "Unauthorized", http.StatusUnauthorized, err))
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	member, err := s.store.AcceptOrgInvitation(r.Context(), req.Token, user.ID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, err.Error(), http.StatusNotFound, err))
		return
	}

	// Switch active project to accepted org's project
	if projs, err := s.store.ListProjects(r.Context(), member.OrganizationID); err == nil && len(projs) > 0 {
		s.setProjectCookie(w, r, projs[0].ID, time.Now().Add(7*24*time.Hour))
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"member":  member,
		"message": "Invitation accepted successfully",
	})
}
