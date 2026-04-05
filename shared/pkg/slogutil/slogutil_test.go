package slogutil

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestNew_includesServiceName(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("test-service", slog.LevelDebug, &buf)
	logger.Info("hello")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["service"] != "test-service" {
		t.Errorf("expected service=test-service, got %v", entry["service"])
	}
	if entry["msg"] != "hello" {
		t.Errorf("expected msg=hello, got %v", entry["msg"])
	}
	if entry["level"] != "INFO" {
		t.Errorf("expected level=INFO, got %v", entry["level"])
	}
}

func TestNew_noTraceContext_omitsTraceFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelDebug, &buf)
	logger.InfoContext(context.Background(), "no span")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := entry["trace_id"]; ok {
		t.Error("trace_id should not be present without active span")
	}
	if _, ok := entry["span_id"]; ok {
		t.Error("span_id should not be present without active span")
	}
}

func TestNew_withTraceContext_includesTraceFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelDebug, &buf)

	// Create a fake span context with known IDs.
	traceID, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	spanID, _ := trace.SpanIDFromHex("0102030405060708")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	logger.InfoContext(ctx, "with span")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["trace_id"] != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("trace_id = %v, want 0102030405060708090a0b0c0d0e0f10", entry["trace_id"])
	}
	if entry["span_id"] != "0102030405060708" {
		t.Errorf("span_id = %v, want 0102030405060708", entry["span_id"])
	}
}

func TestNew_respectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelWarn, &buf)

	logger.Info("should be filtered")
	if buf.Len() != 0 {
		t.Error("INFO log should be filtered at WARN level")
	}

	logger.Warn("should pass")
	if buf.Len() == 0 {
		t.Error("WARN log should pass at WARN level")
	}
}

func TestNew_additionalAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelDebug, &buf)
	logger.Info("req", "method", "GET", "status", 200)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["method"] != "GET" {
		t.Errorf("method = %v, want GET", entry["method"])
	}
	// JSON numbers decode as float64
	if entry["status"] != float64(200) {
		t.Errorf("status = %v, want 200", entry["status"])
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestWithAttrs_preservesTraceContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelDebug, &buf).With("component", "auth")

	traceID, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	spanID, _ := trace.SpanIDFromHex("0102030405060708")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	logger.InfoContext(ctx, "with attrs")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["component"] != "auth" {
		t.Errorf("component = %v, want auth", entry["component"])
	}
	if entry["service"] != "svc" {
		t.Errorf("service = %v, want svc", entry["service"])
	}
	if entry["trace_id"] != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("trace_id missing after WithAttrs")
	}
}
