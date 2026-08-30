package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

type MetricsCollector struct {
	mu           sync.RWMutex
	evalCounters map[string]*uint64 // key: flag:env:variant:enabled -> count
	durationSum  uint64             // in nanoseconds
	durationCnt  uint64
}

var globalCollector = &MetricsCollector{
	evalCounters: make(map[string]*uint64),
}

// RecordEvaluation records an evaluation event for Prometheus metrics
func RecordEvaluation(flagKey string, env domain.Environment, variant string, enabled bool, latencyNs int64) {
	key := fmt.Sprintf(`flag="%s",environment="%s",variant="%s",enabled="%t"`, flagKey, env, variant, enabled)

	globalCollector.mu.RLock()
	counter, exists := globalCollector.evalCounters[key]
	globalCollector.mu.RUnlock()

	if !exists {
		globalCollector.mu.Lock()
		counter, exists = globalCollector.evalCounters[key]
		if !exists {
			var newCount uint64
			counter = &newCount
			globalCollector.evalCounters[key] = counter
		}
		globalCollector.mu.Unlock()
	}

	atomic.AddUint64(counter, 1)
	if latencyNs > 0 {
		atomic.AddUint64(&globalCollector.durationSum, uint64(latencyNs))
		atomic.AddUint64(&globalCollector.durationCnt, 1)
	}
}

// PrometheusHandler exposes metrics in standard Prometheus text format (0.0.4)
func PrometheusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// 1. Process up & build info
		_, _ = fmt.Fprintln(w, "# HELP flagura_up Status of Flagura server instance (1 = up)")
		_, _ = fmt.Fprintln(w, "# TYPE flagura_up gauge")
		_, _ = fmt.Fprintln(w, "flagura_up 1")
		_, _ = fmt.Fprintln(w)

		_, _ = fmt.Fprintln(w, "# HELP flagura_build_info Version and metadata")
		_, _ = fmt.Fprintln(w, "# TYPE flagura_build_info gauge")
		_, _ = fmt.Fprintln(w, `flagura_build_info{version="1.4.0",engine="deterministic-fastpath"} 1`)
		_, _ = fmt.Fprintln(w)

		// 2. Active flags & kill switches
		flags, err := st.ListFlags(context.Background())
		if err == nil {
			_, _ = fmt.Fprintln(w, "# HELP flagura_flags_total Total number of registered feature flags")
			_, _ = fmt.Fprintln(w, "# TYPE flagura_flags_total gauge")
			_, _ = fmt.Fprintf(w, "flagura_flags_total %d\n\n", len(flags))

			killSwitchCount := 0
			for _, f := range flags {
				for _, envCfg := range f.Environments {
					if !envCfg.Enabled {
						killSwitchCount++
					}
				}
			}

			_, _ = fmt.Fprintln(w, "# HELP flagura_kill_switches_active Total number of engaged kill-switches across all environments")
			_, _ = fmt.Fprintln(w, "# TYPE flagura_kill_switches_active gauge")
			_, _ = fmt.Fprintf(w, "flagura_kill_switches_active %d\n\n", killSwitchCount)
		}

		// 3. Evaluations counter
		globalCollector.mu.RLock()
		snapshotCounters := make(map[string]uint64, len(globalCollector.evalCounters))
		for k, v := range globalCollector.evalCounters {
			snapshotCounters[k] = atomic.LoadUint64(v)
		}
		durSum := atomic.LoadUint64(&globalCollector.durationSum)
		durCnt := atomic.LoadUint64(&globalCollector.durationCnt)
		globalCollector.mu.RUnlock()

		_, _ = fmt.Fprintln(w, "# HELP flagura_evaluations_total Total number of flag evaluations performed")
		_, _ = fmt.Fprintln(w, "# TYPE flagura_evaluations_total counter")
		if len(snapshotCounters) == 0 {
			_, _ = fmt.Fprintln(w, `flagura_evaluations_total{flag="none",environment="production",variant="none",enabled="false"} 0`)
		} else {
			for labels, count := range snapshotCounters {
				_, _ = fmt.Fprintf(w, "flagura_evaluations_total{%s} %d\n", labels, count)
			}
		}
		_, _ = fmt.Fprintln(w)

		// 4. Latency Summary
		_, _ = fmt.Fprintln(w, "# HELP flagura_evaluation_duration_seconds Latency summary of flag evaluations in seconds")
		_, _ = fmt.Fprintln(w, "# TYPE flagura_evaluation_duration_seconds summary")
		durSumSec := float64(durSum) / 1e9
		_, _ = fmt.Fprintf(w, "flagura_evaluation_duration_seconds_sum %f\n", durSumSec)
		_, _ = fmt.Fprintf(w, "flagura_evaluation_duration_seconds_count %d\n", durCnt)
	}
}
