package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kite-production/spark/services/connector-googlechat/internal/normalizer"
)

func TestHandler_ValidEvent(t *testing.T) {
	var received *normalizer.ChatEvent
	handler := New("12345", func(ev *normalizer.ChatEvent) {
		received = ev
	})

	body := `{"type":"MESSAGE","message":{"name":"spaces/S/messages/M","text":"hello"},"space":{"name":"spaces/S","type":"ROOM"},"user":{"name":"users/U","displayName":"Alice"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Goog-Project-Number", "12345")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if received == nil {
		t.Fatal("expected event to be received")
	}
	if received.Type != "MESSAGE" {
		t.Errorf("event type = %q, want %q", received.Type, "MESSAGE")
	}
	if received.Message.Text != "hello" {
		t.Errorf("message text = %q, want %q", received.Message.Text, "hello")
	}
}

func TestHandler_ProjectNumberMismatch(t *testing.T) {
	handler := New("12345", func(_ *normalizer.ChatEvent) {
		t.Error("handler should not be called on project number mismatch")
	})

	body := `{"type":"MESSAGE","message":{"text":"hello"},"space":{"name":"spaces/S"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Goog-Project-Number", "99999")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandler_NoProjectNumberVerification(t *testing.T) {
	var called bool
	handler := New("", func(_ *normalizer.ChatEvent) {
		called = true
	})

	body := `{"type":"MESSAGE","message":{"text":"hello"},"space":{"name":"spaces/S"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	handler := New("", func(_ *normalizer.ChatEvent) {
		t.Error("handler should not be called for GET")
	})

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	handler := New("", func(_ *normalizer.ChatEvent) {
		t.Error("handler should not be called for invalid JSON")
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
