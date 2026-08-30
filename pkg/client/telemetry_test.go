package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
)

func TestTelemetryBufferAggregationAndFlush(t *testing.T) {
	flushedPayloads := make(chan client.TelemetryPayload, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry/events" && r.Method == http.MethodPost {
			var payload client.TelemetryPayload
			_ = json.NewDecoder(r.Body).Decode(&payload)
			select {
			case flushedPayloads <- payload:
			default:
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	tb := client.NewTelemetryBuffer(ts.URL, "test_api_key", nil)

	// Record evaluations
	tb.Record("ai-search", "treatment")
	tb.Record("ai-search", "treatment")
	tb.Record("ai-search", "control")
	tb.Record("checkout-v2", "off")

	// Flush
	err := tb.Flush(context.Background())
	if err != nil {
		t.Fatalf("expected flush to succeed, got error: %v", err)
	}

	select {
	case p := <-flushedPayloads:
		if p.Events["ai-search"].Evaluations != 3 {
			t.Fatalf("expected 3 evaluations for 'ai-search', got %d", p.Events["ai-search"].Evaluations)
		}
		if p.Events["ai-search"].Variants["treatment"] != 2 {
			t.Fatalf("expected 2 treatment variants, got %d", p.Events["ai-search"].Variants["treatment"])
		}
		if p.Events["ai-search"].Variants["control"] != 1 {
			t.Fatalf("expected 1 control variant, got %d", p.Events["ai-search"].Variants["control"])
		}
		if p.Events["checkout-v2"].Evaluations != 1 {
			t.Fatalf("expected 1 evaluation for 'checkout-v2', got %d", p.Events["checkout-v2"].Evaluations)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for telemetry payload flush")
	}
}
