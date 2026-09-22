package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTelemetryBufferAggregationAndFlush(t *testing.T) {
	flushedPayloads := make(chan TelemetryPayload, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry/events" && r.Method == http.MethodPost {
			var payload TelemetryPayload
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

	tb := NewTelemetryBuffer(ts.URL, "test_api_key", nil)

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

func TestTelemetryBufferCapacityBoundsAndDropEvents(t *testing.T) {
	flushedPayloads := make(chan TelemetryPayload, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry/events" && r.Method == http.MethodPost {
			var payload TelemetryPayload
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

	tb := NewTelemetryBuffer(ts.URL, "test_key", nil)
	// Set small bounds to test enforcement: max 5 flags, max 3 variants per flag
	tb.SetCapacityBounds(5, 3)

	// Record 5 distinct flags (should all succeed)
	for i := 1; i <= 5; i++ {
		tb.Record(fmt.Sprintf("flag-%d", i), "on")
	}
	if tb.DroppedEvents() != 0 {
		t.Fatalf("expected 0 dropped events so far, got %d", tb.DroppedEvents())
	}

	// Record 3 more new flags (exceeds maxFlags 5; should drop 3)
	tb.Record("flag-6", "on")
	tb.Record("flag-7", "on")
	tb.Record("flag-8", "on")
	if tb.DroppedEvents() != 3 {
		t.Fatalf("expected 3 dropped events due to flag bound, got %d", tb.DroppedEvents())
	}

	// For an existing flag ("flag-1"), record 2 additional variants (total 3 variants: "on", "var-a", "var-b")
	tb.Record("flag-1", "var-a")
	tb.Record("flag-1", "var-b")
	if tb.DroppedEvents() != 3 {
		t.Fatalf("expected still 3 dropped events, got %d", tb.DroppedEvents())
	}

	// Record a 4th variant for flag-1 (exceeds maxVariants 3; should drop)
	tb.Record("flag-1", "var-c")
	if tb.DroppedEvents() != 4 {
		t.Fatalf("expected 4 dropped events, got %d", tb.DroppedEvents())
	}

	// Record an existing variant for an existing flag (should succeed without dropping)
	tb.Record("flag-1", "var-a")
	if tb.DroppedEvents() != 4 {
		t.Fatalf("expected still 4 dropped events, got %d", tb.DroppedEvents())
	}

	// Flush and verify dropped_events in payload
	err := tb.Flush(context.Background())
	if err != nil {
		t.Fatalf("expected flush to succeed, got error: %v", err)
	}

	select {
	case p := <-flushedPayloads:
		if p.DroppedEvents != 4 {
			t.Fatalf("expected payload DroppedEvents to be 4, got %d", p.DroppedEvents)
		}
		if len(p.Events) != 5 {
			t.Fatalf("expected 5 tracked flags, got %d", len(p.Events))
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for telemetry payload flush")
	}

	// Verify DroppedEvents reset after flush
	if tb.DroppedEvents() != 0 {
		t.Fatalf("expected droppedEvents reset to 0 after flush, got %d", tb.DroppedEvents())
	}
}
