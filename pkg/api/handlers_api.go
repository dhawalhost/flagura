package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/engine"
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
	flags, _ := s.store.ListFlags(r.Context())
	uptime := time.Since(s.startTime).Seconds()
	totalEvals := atomic.LoadUint64(&s.evalCount)

	prodEnabled := 0
	stagingEnabled := 0
	devEnabled := 0
	for _, f := range flags {
		if f.EnvConfig("production").Enabled {
			prodEnabled++
		}
		if f.EnvConfig("staging").Enabled {
			stagingEnabled++
		}
		if f.EnvConfig("development").Enabled {
			devEnabled++
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP flagura_up 1 if the service is operational\n")
	fmt.Fprintf(w, "# TYPE flagura_up gauge\n")
	fmt.Fprintf(w, "flagura_up 1\n\n")

	fmt.Fprintf(w, "# HELP flagura_build_info Version and metadata\n")
	fmt.Fprintf(w, "# TYPE flagura_build_info gauge\n")
	fmt.Fprintf(w, "flagura_build_info{version=\"1.1.0\",engine=\"deterministic-fastpath\"} 1\n\n")

	fmt.Fprintf(w, "# HELP flagura_uptime_seconds Engine uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE flagura_uptime_seconds gauge\n")
	fmt.Fprintf(w, "flagura_uptime_seconds %.2f\n\n", uptime)

	fmt.Fprintf(w, "# HELP flagura_evaluations_total Total flag evaluations served\n")
	fmt.Fprintf(w, "# TYPE flagura_evaluations_total counter\n")
	fmt.Fprintf(w, "flagura_evaluations_total %d\n\n", totalEvals)

	fmt.Fprintf(w, "# HELP flagura_flags_total Total feature flags in catalog\n")
	fmt.Fprintf(w, "# TYPE flagura_flags_total gauge\n")
	fmt.Fprintf(w, "flagura_flags_total %d\n\n", len(flags))

	fmt.Fprintf(w, "# HELP flagura_flags_enabled Total active flags by environment\n")
	fmt.Fprintf(w, "# TYPE flagura_flags_enabled gauge\n")
	fmt.Fprintf(w, "flagura_flags_enabled{environment=\"production\"} %d\n", prodEnabled)
	fmt.Fprintf(w, "flagura_flags_enabled{environment=\"staging\"} %d\n", stagingEnabled)
	fmt.Fprintf(w, "flagura_flags_enabled{environment=\"development\"} %d\n", devEnabled)
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

	allFlags, err := s.store.ListFlags(r.Context())
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
				} else {
					res := engine.EvaluateFlag(flag, req.Context)
					results[flag.Key] = res
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
			} else {
				res := engine.EvaluateFlag(flag, req.Context)
				results[flag.Key] = res
			}
		}
	}

	durationUs := float64(time.Since(start).Nanoseconds()) / 1000.0
	atomic.AddUint64(&s.evalCount, uint64(len(results)))

	w.Header().Set("Content-Type", "application/json")
	if includeTrace {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
