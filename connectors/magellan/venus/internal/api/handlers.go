// Package api provides HTTP handlers for the Venus messaging service.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kite-production/spark/services/venus/internal/store"
)

// Handler wraps the store and provides HTTP handlers.
type Handler struct {
	store *store.Store
}

// New creates a new Handler.
func New(s *store.Store) *Handler {
	return &Handler{store: s}
}

// RegisterRoutes sets up all HTTP routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Channels
	mux.HandleFunc("POST /api/channels", h.CreateChannel)
	mux.HandleFunc("GET /api/channels", h.ListChannels)
	mux.HandleFunc("GET /api/channels/{id}", h.GetChannel)
	mux.HandleFunc("DELETE /api/channels/{id}", h.DeleteChannel)

	// Messages
	mux.HandleFunc("POST /api/channels/{id}/messages", h.SendMessage)
	mux.HandleFunc("GET /api/channels/{id}/messages", h.ListMessages)
	mux.HandleFunc("GET /api/channels/{id}/messages/{mid}", h.GetMessage)

	// Webhooks
	mux.HandleFunc("POST /api/webhooks", h.RegisterWebhook)
	mux.HandleFunc("DELETE /api/webhooks/{id}", h.RemoveWebhook)
	mux.HandleFunc("GET /api/webhooks", h.ListWebhooks)

	// Health & Status
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /api/status", h.Status)
}

// ── Channel Handlers ────────────────────────────────────────────────────────

func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	ch, err := h.store.CreateChannel(req.Name, req.Description)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ch)
}

func (h *Handler) ListChannels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListChannels())
}

func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ch, ok := h.store.GetChannel(id)
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !h.store.DeleteChannel(id) {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Message Handlers ────────────────────────────────────────────────────────

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("id")
	var req struct {
		SenderID   string `json:"sender_id"`
		SenderName string `json:"sender_name"`
		Text       string `json:"text"`
		ThreadID   string `json:"thread_id"`
		ReplyToID  string `json:"reply_to_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.SenderID == "" {
		req.SenderID = "anonymous"
	}
	if req.SenderName == "" {
		req.SenderName = req.SenderID
	}

	msg, err := h.store.AddMessage(channelID, req.SenderID, req.SenderName, req.Text, req.ThreadID, req.ReplyToID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("id")
	var since time.Time
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		var err error
		since, err = time.Parse(time.RFC3339Nano, sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since timestamp (use RFC3339)")
			return
		}
	}
	msgs, err := h.store.ListMessages(channelID, since)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (h *Handler) GetMessage(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("id")
	messageID := r.PathValue("mid")
	msg, ok := h.store.GetMessage(channelID, messageID)
	if !ok {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

// ── Webhook Handlers ────────────────────────────────────────────────────────

func (h *Handler) RegisterWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	wh := h.store.RegisterWebhook(req.URL)
	writeJSON(w, http.StatusCreated, wh)
}

func (h *Handler) RemoveWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !h.store.RemoveWebhook(id) {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListWebhooks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListWebhooks())
}

// ── Health & Status ─────────────────────────────────────────────────────────

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "venus"})
}

func (h *Handler) Status(w http.ResponseWriter, _ *http.Request) {
	stats := h.store.Stats()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service":  "venus",
		"version":  "1.0.0",
		"uptime":   time.Since(startTime).String(),
		"channels": stats["channels"],
		"messages": stats["messages"],
		"webhooks": stats["webhooks"],
	})
}

var startTime = time.Now()

// ── Helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
