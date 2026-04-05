// Package slogutil provides structured JSON logging with OpenTelemetry trace
// context injection. Every log line includes service name, trace_id, and
// span_id from the active OTel span — enabling Loki↔Tempo correlation in
// Grafana.
//
// Usage:
//
//	logger := slogutil.New("api-gateway", slog.LevelInfo)
//	logger.InfoContext(ctx, "request handled", "method", "GET", "status", 200)
//
// Output:
//
//	{"time":"...","level":"INFO","msg":"request handled","service":"api-gateway",
//	 "trace_id":"abc123...","span_id":"def456...","method":"GET","status":200}
package slogutil

import (
	"context"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// New creates a structured JSON logger that injects service name and OTel
// trace context into every log record.
//
// Logs are written to stdout as JSON — Promtail parses them and ships to Loki.
// If w is nil, os.Stdout is used.
func New(service string, level slog.Level) *slog.Logger {
	return NewWithWriter(service, level, nil)
}

// NewWithWriter creates a logger writing to w (useful for testing).
func NewWithWriter(service string, level slog.Level, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(&traceHandler{
		inner:   handler,
		service: service,
	})
}

// ParseLevel converts a level string ("debug", "info", "warn", "error") to
// slog.Level. Returns slog.LevelInfo for unrecognized values.
func ParseLevel(s string) slog.Level {
	switch s {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "info", "INFO", "":
		return slog.LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// traceHandler wraps a slog.Handler to inject trace_id, span_id, and service
// into every log record. When no active span exists, trace_id and span_id are
// omitted (not set to empty strings) to keep logs clean.
type traceHandler struct {
	inner   slog.Handler
	service string
}

func (h *traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.String("service", h.service))

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}

	return h.inner.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{
		inner:   h.inner.WithAttrs(attrs),
		service: h.service,
	}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{
		inner:   h.inner.WithGroup(name),
		service: h.service,
	}
}
