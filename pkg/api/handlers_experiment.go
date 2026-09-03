package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/stats"
)

// handleIngestEvents accepts batched experiment events from client SDKs.
// POST /api/v1/events
func (s *Server) handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Events []domain.ExperimentEvent `json:"events"`
		Event  *domain.ExperimentEvent  `json:"event,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	events := req.Events
	if req.Event != nil {
		events = append(events, *req.Event)
	}

	if len(events) == 0 {
		http.Error(w, "No events provided in payload", http.StatusBadRequest)
		return
	}

	projectID := s.resolveProjectID(r)
	for i := range events {
		if events[i].ProjectID == "" {
			events[i].ProjectID = projectID
		}
	}

	if err := s.store.RecordExperimentEvents(r.Context(), events); err != nil {
		http.Error(w, "Failed to record events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "accepted",
		"ingested": len(events),
	})
}

// handleGetExperimentReport calculates and returns A/B statistical results for a flag.
// GET /api/v1/experiments/:key?metric=signup&env=production&control=control
func (s *Server) handleGetExperimentReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract flag key from URL /api/v1/experiments/:key or /api/v1/flags/:key/experiment
	path := r.URL.Path
	var flagKey string
	if strings.HasPrefix(path, "/api/v1/experiments/") {
		flagKey = strings.TrimPrefix(path, "/api/v1/experiments/")
	} else if strings.HasPrefix(path, "/api/v1/flags/") && strings.HasSuffix(path, "/experiment") {
		trimmed := strings.TrimPrefix(path, "/api/v1/flags/")
		flagKey = strings.TrimSuffix(trimmed, "/experiment")
	}
	flagKey = strings.Trim(flagKey, "/")

	if flagKey == "" {
		http.Error(w, "Flag key is required", http.StatusBadRequest)
		return
	}

	projectID := s.resolveProjectID(r)
	flag, err := s.store.GetFlagByProject(r.Context(), projectID, flagKey)
	if err != nil || flag == nil {
		flag, err = s.store.GetFlag(r.Context(), flagKey)
		if err != nil || flag == nil {
			http.Error(w, "Flag not found: "+flagKey, http.StatusNotFound)
			return
		}
	}

	// Read query parameters
	metricName := r.URL.Query().Get("metric")
	if metricName == "" {
		metricName = "conversion"
	}
	env := domain.Environment(r.URL.Query().Get("env"))
	if env == "" {
		env = domain.EnvProduction
	}
	controlVariant := r.URL.Query().Get("control")
	if controlVariant == "" {
		if envCfg, ok := flag.Environments[env]; ok && envCfg.OffVariant != "" {
			controlVariant = envCfg.OffVariant
		} else {
			controlVariant = "control"
		}
	}

	// 1. Fetch experiment events from store
	events, err := s.store.GetExperimentEvents(r.Context(), flagKey, 10000)
	if err != nil {
		http.Error(w, "Failed to retrieve experiment events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Fetch evaluation exposure counts from telemetry aggregator
	exposures := make(map[string]int64)
	if s.telemetry != nil {
		statsData := s.telemetry.Stats(flagKey)
		if vMap, ok := statsData["variants"].(map[string]uint64); ok {
			for v, count := range vMap {
				if count > math.MaxInt64 {
					exposures[v] = math.MaxInt64
				} else {
					exposures[v] = int64(count)
				}
			}
		}
	}

	// Fallback: If exposures are zero (e.g. cold start), estimate baseline from events
	for _, ev := range events {
		if ev.FlagKey == flagKey {
			if _, exists := exposures[ev.Variant]; !exists {
				exposures[ev.Variant] = 0
			}
		}
	}

	// 3. Compute statistical report
	report := stats.AnalyzeExperiment(flagKey, metricName, domain.EventTypeConversion, env, controlVariant, exposures, events)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}
