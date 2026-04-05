package slogutil

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryServerInterceptor_logsSuccess(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("test-cp", slog.LevelDebug, &buf)

	interceptor := UnaryServerInterceptor(logger)

	info := &grpc.UnaryServerInfo{FullMethod: "/kite.v1.ControlPlane/ListAgents"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want ok", resp)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["method"] != "/kite.v1.ControlPlane/ListAgents" {
		t.Errorf("method = %v, want /kite.v1.ControlPlane/ListAgents", entry["method"])
	}
	if entry["code"] != "OK" {
		t.Errorf("code = %v, want OK", entry["code"])
	}
	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", entry["level"])
	}
	if entry["service"] != "test-cp" {
		t.Errorf("service = %v, want test-cp", entry["service"])
	}
}

func TestUnaryServerInterceptor_logsError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelDebug, &buf)

	interceptor := UnaryServerInterceptor(logger)

	info := &grpc.UnaryServerInfo{FullMethod: "/kite.v1.ControlPlane/GetAgent"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.NotFound, "agent not found")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	if err == nil {
		t.Fatal("expected error")
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["code"] != "NotFound" {
		t.Errorf("code = %v, want NotFound", entry["code"])
	}
	if entry["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR for failed RPC", entry["level"])
	}
}
