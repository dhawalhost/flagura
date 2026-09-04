package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/engine"
	"github.com/dhawalhost/flagura/pkg/telemetry"
)

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
