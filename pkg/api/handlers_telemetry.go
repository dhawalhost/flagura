package api

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

const maxFlagsPerProject = 1000

// FlagAggregatedMetric holds accumulated runtime metrics for a single flag.
type FlagAggregatedMetric struct {
	TotalEvaluations uint64            `json:"total_evaluations"`
	Variants         map[string]uint64 `json:"variants"`
	LastEvaluatedAt  int64             `json:"last_evaluated_at"`
}

// ProjectTelemetry holds telemetry metrics scoped to a single tenant project.
type ProjectTelemetry struct {
	flagMetrics  map[string]*FlagAggregatedMetric
	totalEvals   uint64
	hourlyPoints [24]uint64
}

// TelemetryAggregator stores and aggregates runtime evaluations from connected client SDKs scoped by project.
type TelemetryAggregator struct {
	mu       sync.RWMutex
	projects map[string]*ProjectTelemetry
}

// NewTelemetryAggregator creates a new thread-safe TelemetryAggregator.
func NewTelemetryAggregator() *TelemetryAggregator {
	return &TelemetryAggregator{
		projects: make(map[string]*ProjectTelemetry),
	}
}

func (ta *TelemetryAggregator) getOrCreateProjectLocked(projectID string) *ProjectTelemetry {
	if projectID == "" {
		projectID = domain.DefaultProjectID
	}
	proj, ok := ta.projects[projectID]
	if !ok {
		proj = &ProjectTelemetry{
			flagMetrics: make(map[string]*FlagAggregatedMetric),
		}
		ta.projects[projectID] = proj
	}
	return proj
}

// Ingest merges incoming client telemetry batches into the server's aggregated state for the given project.
func (ta *TelemetryAggregator) Ingest(projectID string, events map[string]struct {
	Evaluations uint64            `json:"evaluations"`
	Variants    map[string]uint64 `json:"variants"`
}) int {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if projectID == "" {
		projectID = domain.DefaultProjectID
	}
	proj := ta.getOrCreateProjectLocked(projectID)

	now := time.Now().UTC()
	hourIdx := now.Hour()
	updatedCount := 0

	for flagKey, metric := range events {
		if metric.Evaluations == 0 {
			continue
		}

		m, exists := proj.flagMetrics[flagKey]
		if !exists {
			// Bounded memory enforcement: evict oldest evaluated flag if capacity exceeded
			if len(proj.flagMetrics) >= maxFlagsPerProject {
				var oldestKey string
				var oldestTime int64 = math.MaxInt64
				for k, v := range proj.flagMetrics {
					if v.LastEvaluatedAt < oldestTime {
						oldestTime = v.LastEvaluatedAt
						oldestKey = k
					}
				}
				if oldestKey != "" {
					delete(proj.flagMetrics, oldestKey)
				}
			}

			m = &FlagAggregatedMetric{
				Variants: make(map[string]uint64),
			}
			proj.flagMetrics[flagKey] = m
		}

		m.TotalEvaluations += metric.Evaluations
		m.LastEvaluatedAt = now.UnixMilli()
		proj.totalEvals += metric.Evaluations
		proj.hourlyPoints[hourIdx] += metric.Evaluations

		for vKey, vCount := range metric.Variants {
			m.Variants[vKey] += vCount
		}
		updatedCount++
	}

	return updatedCount
}

// Stats returns a snapshot of evaluation statistics for the project and dashboard.
func (ta *TelemetryAggregator) Stats(projectID string, flagKey string) map[string]interface{} {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	if projectID == "" {
		projectID = domain.DefaultProjectID
	}
	proj, ok := ta.projects[projectID]
	if !ok {
		return map[string]interface{}{
			"total_evaluations": 0,
			"flags":             map[string]FlagAggregatedMetric{},
			"hourly_points":     [24]uint64{},
		}
	}

	if flagKey != "" && flagKey != "all" {
		if m, ok := proj.flagMetrics[flagKey]; ok {
			varCopy := make(map[string]uint64, len(m.Variants))
			for k, v := range m.Variants {
				varCopy[k] = v
			}
			return map[string]interface{}{
				"flag_key":          flagKey,
				"total_evaluations": m.TotalEvaluations,
				"variants":          varCopy,
				"last_evaluated_at": m.LastEvaluatedAt,
				"hourly_points":     proj.hourlyPoints,
			}
		}
		return map[string]interface{}{
			"flag_key":          flagKey,
			"total_evaluations": 0,
			"variants":          map[string]uint64{},
			"last_evaluated_at": 0,
			"hourly_points":     proj.hourlyPoints,
		}
	}

	flagsCopy := make(map[string]FlagAggregatedMetric, len(proj.flagMetrics))
	for k, v := range proj.flagMetrics {
		varCopy := make(map[string]uint64, len(v.Variants))
		for vk, vv := range v.Variants {
			varCopy[vk] = vv
		}
		flagsCopy[k] = FlagAggregatedMetric{
			TotalEvaluations: v.TotalEvaluations,
			Variants:         varCopy,
			LastEvaluatedAt:  v.LastEvaluatedAt,
		}
	}

	return map[string]interface{}{
		"total_evaluations": proj.totalEvals,
		"flags":             flagsCopy,
		"hourly_points":     proj.hourlyPoints,
	}
}

// handleIngestTelemetry ingests batched evaluation telemetry from client SDKs.
func (s *Server) handleIngestTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Try standard evaluation aggregator format (map of flags)
	var req struct {
		Timestamp int64 `json:"timestamp"`
		Events    map[string]struct {
			Evaluations uint64            `json:"evaluations"`
			Variants    map[string]uint64 `json:"variants"`
		} `json:"events"`
	}

	if err := json.Unmarshal(bodyBytes, &req); err == nil && req.Events != nil {
		count := 0
		if s.telemetry != nil {
			count = s.telemetry.Ingest(projectID, req.Events)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "ok",
			"flags_updated": count,
		})
		return
	}

	// 2. Try SDK track/conversion event array format
	var trackReq struct {
		Events []struct {
			FlagKey     string      `json:"flag_key"`
			Variant     string      `json:"variant"`
			MetricName  string      `json:"metric_name"`
			Value       interface{} `json:"value"`
			UserID      string      `json:"user_id"`
			Environment string      `json:"environment"`
			Timestamp   interface{} `json:"timestamp"`
		} `json:"events"`
	}

	if err := json.Unmarshal(bodyBytes, &trackReq); err == nil && len(trackReq.Events) > 0 {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":          "ok",
			"events_ingested": len(trackReq.Events),
		})
		return
	}

	http.Error(w, "Invalid telemetry payload format", http.StatusBadRequest)
}

// handleGetTelemetryStats returns evaluation telemetry statistics for the authorized project.
func (s *Server) handleGetTelemetryStats(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	flagKey := r.URL.Query().Get("flag")

	var stats map[string]interface{}
	if s.telemetry != nil {
		stats = s.telemetry.Stats(projectID, flagKey)
	} else {
		stats = map[string]interface{}{
			"total_evaluations": 0,
			"flags":             map[string]interface{}{},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}
