// Package webhook handles incoming webhook callbacks from Venus.
package webhook

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/kite-production/spark/services/connector-magellan/internal/normalizer"
	"github.com/kite-production/spark/services/connector-magellan/internal/venus"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

// Handler receives webhook POSTs from Venus and publishes to NATS.
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

// ServeHTTP handles incoming webhook POST from Venus.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg venus.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		log.Printf("[magellan] webhook: invalid JSON: %v", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Filter: skip bot-originated messages.
	if !normalizer.ShouldProcess(&msg) {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("[magellan] webhook: received message %s from %s in #%s: %s",
		msg.ID, msg.SenderName, msg.ChannelID, truncate(msg.Text, 80))

	// Normalize to InboundMessage proto.
	inbound := h.normalizer.Normalize(&msg)

	// Publish to NATS.
	if err := h.base.PublishInbound(inbound); err != nil {
		log.Printf("[magellan] webhook: publish failed: %v", err)
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
