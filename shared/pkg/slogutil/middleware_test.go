package slogutil

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPMiddleware_logsRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("test-gw", slog.LevelDebug, &buf)

	handler := HTTPMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["method"] != "GET" {
		t.Errorf("method = %v, want GET", entry["method"])
	}
	if entry["path"] != "/api/v1/agents" {
		t.Errorf("path = %v, want /api/v1/agents", entry["path"])
	}
	if entry["status"] != float64(200) {
		t.Errorf("status = %v, want 200", entry["status"])
	}
	if entry["service"] != "test-gw" {
		t.Errorf("service = %v, want test-gw", entry["service"])
	}
	if _, ok := entry["duration"]; !ok {
		t.Error("expected duration field")
	}
}

func TestHTTPMiddleware_500isError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelDebug, &buf)

	handler := HTTPMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR for 500", entry["level"])
	}
	if entry["status"] != float64(500) {
		t.Errorf("status = %v, want 500", entry["status"])
	}
}

func TestHTTPMiddleware_400isWarn(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter("svc", slog.LevelDebug, &buf)

	handler := HTTPMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["level"] != "WARN" {
		t.Errorf("level = %v, want WARN for 400", entry["level"])
	}
}

func TestSanitizePath_truncatesLongPaths(t *testing.T) {
	long := "/" + string(make([]byte, 300))
	result := sanitizePath(long)
	if len(result) > 260 {
		t.Errorf("path should be truncated, got len=%d", len(result))
	}
}
