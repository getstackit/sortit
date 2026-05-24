package tracing

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Init sets up the global OTEL TracerProvider and text map propagator.
// Spans are recorded (so trace/span IDs appear in logs) but not exported.
// Call the returned shutdown function on application exit.
func Init(_ context.Context, _ string) (shutdown func(context.Context) error, err error) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// SlogHandler wraps a slog.Handler and injects trace_id and span_id from
// the OTEL span context into every log record that carries a context.
type SlogHandler struct {
	inner slog.Handler
}

// NewSlogHandler returns a handler that enriches log records with trace context.
func NewSlogHandler(inner slog.Handler) *SlogHandler {
	return &SlogHandler{inner: inner}
}

func (h *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		record.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
	}
	if sc.HasSpanID() {
		record.AddAttrs(slog.String("span_id", sc.SpanID().String()))
	}
	return h.inner.Handle(ctx, record)
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{inner: h.inner.WithGroup(name)}
}
