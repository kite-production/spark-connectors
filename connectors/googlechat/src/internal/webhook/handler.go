// Package webhook implements the HTTP webhook handler for Google Chat events.
package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/kite-production/spark/services/connector-googlechat/internal/normalizer"
)

// EventHandler is called when a valid Chat event is received.
type EventHandler func(ev *normalizer.ChatEvent)

// Handler handles Google Chat webhook HTTP requests.
type Handler struct {
	projectNumber string
	onEvent       EventHandler
}

// New creates a webhook Handler.
// projectNumber is the Google Cloud project number used for request verification.
func New(projectNumber string, handler EventHandler) *Handler {
	return &Handler{
		projectNumber: projectNumber,
		onEvent:       handler,
	}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify project number from the request header if configured.
	if h.projectNumber != "" {
		projNum := r.Header.Get("X-Goog-Project-Number")
		if projNum != h.projectNumber {
			log.Printf("webhook: project number mismatch: got %q, want %q", projNum, h.projectNumber)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, fmt.Sprintf("reading body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var ev normalizer.ChatEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	h.onEvent(&ev)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}
