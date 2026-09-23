package telemetry_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/dhawalhost/flagura/pkg/telemetry"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTracer_StartSpanAndExtract(t *testing.T) {
	// Set up TracerProvider
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)

	ctx := context.Background()
	ctx, span := telemetry.StartSpan(ctx, "test.operation")
	defer span.End()

	traceID := telemetry.ExtractTraceID(ctx)
	if traceID == "" {
		t.Fatalf("expected non-empty traceID, got empty")
	}

	header := make(http.Header)
	telemetry.InjectTraceContext(ctx, header)

	traceparent := header.Get("traceparent")
	if traceparent == "" {
		t.Fatalf("expected injected traceparent header, got empty")
	}

	// Extract into new context
	extractedCtx := telemetry.ExtractTraceContext(context.Background(), header)
	extractedTraceID := telemetry.ExtractTraceID(extractedCtx)
	if extractedTraceID != traceID {
		t.Fatalf("expected extracted traceID %q to match original %q", extractedTraceID, traceID)
	}
}

func TestTracer_EmptyTraceID(t *testing.T) {
	ctx := context.Background()
	traceID := telemetry.ExtractTraceID(ctx)
	if traceID != "" {
		t.Fatalf("expected empty traceID from background context, got %q", traceID)
	}
}

func TestTracer_ExtractInvalidHeader(t *testing.T) {
	header := make(http.Header)
	header.Set("traceparent", "invalid-traceparent-value")

	ctx := telemetry.ExtractTraceContext(context.Background(), header)
	traceID := telemetry.ExtractTraceID(ctx)
	if traceID != "" {
		t.Fatalf("expected empty traceID from invalid header, got %q", traceID)
	}
}
