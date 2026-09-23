package api

import (
	"fmt"
	"net/http"

	"github.com/dhawalhost/flagura/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type traceResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *traceResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *traceResponseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// TracingMiddleware extracts W3C trace context, manages the request root span, and attaches X-Trace-ID.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := telemetry.ExtractTraceContext(r.Context(), r.Header)

		ctx, span := telemetry.StartSpan(ctx, "http.request",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
				attribute.String("http.client_ip", GetClientIP(r)),
			),
		)
		defer span.End()

		traceID := telemetry.ExtractTraceID(ctx)
		if traceID != "" {
			w.Header().Set("X-Trace-ID", traceID)
		}

		rw := &traceResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rw, r.WithContext(ctx))

		span.SetAttributes(attribute.Int("http.status_code", rw.statusCode))
		if rw.statusCode >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("server error: %d", rw.statusCode))
		}
	})
}
