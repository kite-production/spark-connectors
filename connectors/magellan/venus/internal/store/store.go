// Package store provides a thread-safe in-memory message store for Venus.
package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Channel represents a messaging channel.
type Channel struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// Message represents a single message in a channel.
type Message struct {
	ID         string    `json:"id"`
	ChannelID  string    `json:"channel_id"`
	SenderID   string    `json:"sender_id"`
	SenderName string    `json:"sender_name"`
	Text       string    `json:"text"`
	ThreadID   string    `json:"thread_id,omitempty"`
	ReplyToID  string    `json:"reply_to_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Webhook represents a registered webhook endpoint.
type Webhook struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Store is a thread-safe in-memory store for channels, messages, and webhooks.
type Store struct {
	mu       sync.RWMutex
	channels map[string]*Channel
	messages map[string][]*Message // channelID -> messages
	webhooks map[string]*Webhook

	// Subscribers for real-time message push (WebSocket, webhooks).
	subMu       sync.RWMutex
	subscribers []func(*Message)
}

// New creates a new Store with a default #general channel.
func New() *Store {
	s := &Store{
		channels: make(map[string]*Channel),
		messages: make(map[string][]*Message),
		webhooks: make(map[string]*Webhook),
	}
	// Auto-create #general channel.
	s.channels["general"] = &Channel{
		ID:          "general",
		Name:        "general",
		Description: "Default channel for testing",
		CreatedAt:   time.Now(),
	}
	s.messages["general"] = make([]*Message, 0)
	return s
}

// ── Channels ────────────────────────────────────────────────────────────────

// CreateChannel adds a new channel.
func (s *Store) CreateChannel(name, description string) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := name
	if _, exists := s.channels[id]; exists {
		return nil, fmt.Errorf("channel %q already exists", id)
	}

	ch := &Channel{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
	}
	s.channels[id] = ch
	s.messages[id] = make([]*Message, 0)
	return ch, nil
}

// GetChannel retrieves a channel by ID.
func (s *Store) GetChannel(id string) (*Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch, ok := s.channels[id]
	return ch, ok
}

// ListChannels returns all channels.
func (s *Store) ListChannels() []*Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		result = append(result, ch)
	}
	return result
}

// DeleteChannel removes a channel and its messages.
func (s *Store) DeleteChannel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[id]; !ok {
		return false
	}
	delete(s.channels, id)
	delete(s.messages, id)
	return true
}

// ── Messages ────────────────────────────────────────────────────────────────

// AddMessage creates a new message in a channel and notifies subscribers.
func (s *Store) AddMessage(channelID, senderID, senderName, text, threadID, replyToID string) (*Message, error) {
	s.mu.Lock()
	if _, ok := s.channels[channelID]; !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("channel %q not found", channelID)
	}

	msg := &Message{
		ID:         uuid.New().String(),
		ChannelID:  channelID,
		SenderID:   senderID,
		SenderName: senderName,
		Text:       text,
		ThreadID:   threadID,
		ReplyToID:  replyToID,
		CreatedAt:  time.Now(),
	}
	s.messages[channelID] = append(s.messages[channelID], msg)
	s.mu.Unlock()

	// Notify subscribers (non-blocking).
	s.subMu.RLock()
	for _, fn := range s.subscribers {
		go fn(msg)
	}
	s.subMu.RUnlock()

	return msg, nil
}

// ListMessages returns messages for a channel, optionally filtered by since timestamp.
func (s *Store) ListMessages(channelID string, since time.Time) ([]*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs, ok := s.messages[channelID]
	if !ok {
		return nil, fmt.Errorf("channel %q not found", channelID)
	}

	if since.IsZero() {
		return msgs, nil
	}

	result := make([]*Message, 0)
	for _, m := range msgs {
		if m.CreatedAt.After(since) {
			result = append(result, m)
		}
	}
	return result, nil
}

// GetMessage retrieves a single message by channel and message ID.
func (s *Store) GetMessage(channelID, messageID string) (*Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs, ok := s.messages[channelID]
	if !ok {
		return nil, false
	}
	for _, m := range msgs {
		if m.ID == messageID {
			return m, true
		}
	}
	return nil, false
}

// ── Webhooks ────────────────────────────────────────────────────────────────

// RegisterWebhook adds a webhook URL.
func (s *Store) RegisterWebhook(url string) *Webhook {
	s.mu.Lock()
	defer s.mu.Unlock()

	wh := &Webhook{
		ID:  uuid.New().String(),
		URL: url,
	}
	s.webhooks[wh.ID] = wh
	return wh
}

// RemoveWebhook deletes a webhook.
func (s *Store) RemoveWebhook(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[id]; !ok {
		return false
	}
	delete(s.webhooks, id)
	return true
}

// ListWebhooks returns all registered webhooks.
func (s *Store) ListWebhooks() []*Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Webhook, 0, len(s.webhooks))
	for _, wh := range s.webhooks {
		result = append(result, wh)
	}
	return result
}

// ── Subscribers ─────────────────────────────────────────────────────────────

// Subscribe registers a callback for new messages (used by WebSocket + webhook delivery).
func (s *Store) Subscribe(fn func(*Message)) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subscribers = append(s.subscribers, fn)
}

// Stats returns basic store statistics.
func (s *Store) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	totalMsgs := 0
	for _, msgs := range s.messages {
		totalMsgs += len(msgs)
	}
	return map[string]int{
		"channels": len(s.channels),
		"messages": totalMsgs,
		"webhooks": len(s.webhooks),
	}
}
