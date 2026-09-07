package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// FlagAggregatedMetric holds accumulated runtime metrics for a single flag.
type FlagAggregatedMetric struct {
	TotalEvaluations uint64            `json:"total_evaluations"`
	Variants         map[string]uint64 `json:"variants"`
	LastEvaluatedAt  int64             `json:"last_evaluated_at"`
}

// TelemetryAggregator stores and aggregates runtime evaluations from connected client SDKs.
type TelemetryAggregator struct {
	mu           sync.RWMutex
	flagMetrics  map[string]*FlagAggregatedMetric
	totalEvals   uint64
	hourlyPoints [24]uint64
}

// NewTelemetryAggregator creates a new thread-safe TelemetryAggregator.
func NewTelemetryAggregator() *TelemetryAggregator {
	return &TelemetryAggregator{
		flagMetrics: make(map[string]*FlagAggregatedMetric),
	}
}

// Ingest merges incoming client telemetry batches into the server's aggregated state.
func (ta *TelemetryAggregator) Ingest(events map[string]struct {
	Evaluations uint64            `json:"evaluations"`
	Variants    map[string]uint64 `json:"variants"`
}) int {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	now := time.Now().UTC()
	hourIdx := now.Hour()
	updatedCount := 0

	for flagKey, metric := range events {
		if metric.Evaluations == 0 {
			continue
		}

		m, exists := ta.flagMetrics[flagKey]
		if !exists {
			m = &FlagAggregatedMetric{
				Variants: make(map[string]uint64),
			}
			ta.flagMetrics[flagKey] = m
		}

		m.TotalEvaluations += metric.Evaluations
		m.LastEvaluatedAt = now.UnixMilli()
		ta.totalEvals += metric.Evaluations
		ta.hourlyPoints[hourIdx] += metric.Evaluations

		for vKey, vCount := range metric.Variants {
			m.Variants[vKey] += vCount
		}
		updatedCount++
	}

	return updatedCount
}

// Stats returns a snapshot of evaluation statistics for the dashboard.
func (ta *TelemetryAggregator) Stats(flagKey string) map[string]interface{} {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	if flagKey != "" && flagKey != "all" {
		if m, ok := ta.flagMetrics[flagKey]; ok {
			varCopy := make(map[string]uint64, len(m.Variants))
			for k, v := range m.Variants {
				varCopy[k] = v
			}
			return map[string]interface{}{
				"flag_key":          flagKey,
				"total_evaluations": m.TotalEvaluations,
				"variants":          varCopy,
				"last_evaluated_at": m.LastEvaluatedAt,
				"hourly_points":     ta.hourlyPoints,
			}
		}
		return map[string]interface{}{
			"flag_key":          flagKey,
			"total_evaluations": 0,
			"variants":          map[string]uint64{},
			"last_evaluated_at": 0,
			"hourly_points":     ta.hourlyPoints,
		}
	}

	flagsCopy := make(map[string]FlagAggregatedMetric, len(ta.flagMetrics))
	for k, v := range ta.flagMetrics {
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
		"total_evaluations": ta.totalEvals,
		"flags":             flagsCopy,
		"hourly_points":     ta.hourlyPoints,
	}
}

// handleIngestTelemetry ingests batched evaluation telemetry from client SDKs.
func (s *Server) handleIngestTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
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
			count = s.telemetry.Ingest(req.Events)
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

// handleGetTelemetryStats returns evaluation telemetry statistics for the UI.
func (s *Server) handleGetTelemetryStats(w http.ResponseWriter, r *http.Request) {
	flagKey := r.URL.Query().Get("flag")

	var stats map[string]interface{}
	if s.telemetry != nil {
		stats = s.telemetry.Stats(flagKey)
	} else {
		stats = map[string]interface{}{
			"total_evaluations": 0,
			"flags":             map[string]interface{}{},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}
