// Package webhook handles incoming Bot Framework activities from Microsoft Teams.
package webhook

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/kite-production/spark/services/connector-msteams/internal/msteams"
	"github.com/kite-production/spark/services/connector-msteams/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

// Handler receives webhook POSTs from Microsoft Bot Framework and publishes
// normalized messages to NATS via the BaseConnector.
type Handler struct {
	base       *baseconnector.BaseConnector
	normalizer *normalizer.Normalizer
}

// New creates a webhook handler.
func New(base *baseconnector.BaseConnector, norm *normalizer.Normalizer) *Handler {
	return &Handler{
		base:       base,
		normalizer: norm,
	}
}

// ServeHTTP handles incoming Bot Framework activity POSTs from Teams.
//
// Microsoft Teams sends activities as JSON to this endpoint. In production,
// the Authorization header should be validated against the Bot Framework
// JWT signing keys. For now we do a simplified check that the header is present.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Simplified auth check: verify Authorization header is present.
	// Production should validate the JWT against Microsoft's OpenID metadata.
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		log.Println("[msteams] webhook: missing Authorization header")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var activity msteams.Activity
	if err := json.NewDecoder(r.Body).Decode(&activity); err != nil {
		log.Printf("[msteams] webhook: invalid JSON: %v", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Filter: only process message activities from real users.
	if !normalizer.ShouldProcess(&activity) {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("[msteams] webhook: received message %s from %s in %s: %s",
		activity.ID, activity.From.Name, activity.Conversation.ID, truncate(activity.Text, 80))

	// Normalize to InboundMessage proto.
	inbound := h.normalizer.Normalize(&activity)

	// Publish to NATS.
	if err := h.base.PublishInbound(inbound); err != nil {
		log.Printf("[msteams] webhook: publish failed: %v", err)
		http.Error(w, "publish failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
