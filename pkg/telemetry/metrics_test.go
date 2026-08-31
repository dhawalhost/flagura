package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestRecordEvaluation(t *testing.T) {
	evaluations := []struct {
		name       string
		flag       string
		env        domain.Environment
		variant    string
		enabled    bool
		durationUs int64
	}{
		{
			name:       "Production treatment enabled evaluation",
			flag:       "ai-smart-search",
			env:        domain.EnvProduction,
			variant:    "treatment",
			enabled:    true,
			durationUs: 45,
		},
		{
			name:       "Production treatment second evaluation",
			flag:       "ai-smart-search",
			env:        domain.EnvProduction,
			variant:    "treatment",
			enabled:    true,
			durationUs: 55,
		},
		{
			name:       "Staging control disabled evaluation",
			flag:       "ai-smart-search",
			env:        domain.EnvStaging,
			variant:    "control",
			enabled:    false,
			durationUs: 30,
		},
	}

	for _, tt := range evaluations {
		t.Run(tt.name, func(t *testing.T) {
			RecordEvaluation(tt.flag, tt.env, tt.variant, tt.enabled, tt.durationUs)
		})
	}
}

func TestPrometheusHandler(t *testing.T) {
	memStore := store.NewMemoryStore()
	handler := PrometheusHandler(memStore)

	tests := []struct {
		name            string
		expectedSnippet string
	}{
		{name: "Liveness flagura_up metric", expectedSnippet: "flagura_up 1"},
		{name: "Total flags gauge", expectedSnippet: "flagura_flags_total"},
		{name: "Total evaluations counter", expectedSnippet: "flagura_evaluations_total"},
		{name: "Evaluation duration latency sum", expectedSnippet: "flagura_evaluation_duration_seconds_sum"},
		{name: "Flag label present in exposition", expectedSnippet: `flag="ai-smart-search"`},
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from /metrics, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(body, tt.expectedSnippet) {
				t.Errorf("Prometheus output missing snippet %q\nOutput: %s", tt.expectedSnippet, body)
			}
		})
	}
}
