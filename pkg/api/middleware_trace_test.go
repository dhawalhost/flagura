package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/telemetry"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTracingMiddleware_InjectsXTraceID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)

	handler := api.TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := telemetry.ExtractTraceID(r.Context())
		if traceID == "" {
			t.Errorf("expected active traceID in request context, got empty")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	traceIDHeader := rec.Header().Get("X-Trace-ID")
	if traceIDHeader == "" {
		t.Fatalf("expected X-Trace-ID response header to be present, got empty")
	}
}

func TestTracingMiddleware_PropagatesIncomingTraceparent(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)

	expectedTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	traceparent := "00-" + expectedTraceID + "-00f067aa0ba902b7-01"

	handler := api.TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := telemetry.ExtractTraceID(r.Context())
		if traceID != expectedTraceID {
			t.Errorf("expected traceID %q, got %q", expectedTraceID, traceID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/evaluate", nil)
	req.Header.Set("traceparent", traceparent)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	resTraceID := rec.Header().Get("X-Trace-ID")
	if resTraceID != expectedTraceID {
		t.Fatalf("expected X-Trace-ID response header %q, got %q", expectedTraceID, resTraceID)
	}
}
