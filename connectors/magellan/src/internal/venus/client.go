// Package venus provides an HTTP client for the Venus messaging service.
package venus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Message matches the Venus message JSON format.
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

// Webhook matches the Venus webhook JSON format.
type Webhook struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Client communicates with the Venus HTTP API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient creates a Venus API client with API key authentication.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// doRequest creates an HTTP request with the API key header.
func (c *Client) doRequest(method, url string, body []byte) (*http.Response, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return c.http.Do(req)
}

// SendMessage posts a message to a Venus channel.
func (c *Client) SendMessage(channelID, senderID, senderName, text, threadID, replyToID string) (*Message, error) {
	body := map[string]string{
		"sender_id":   senderID,
		"sender_name": senderName,
		"text":        text,
	}
	if threadID != "" {
		body["thread_id"] = threadID
	}
	if replyToID != "" {
		body["reply_to_id"] = replyToID
	}

	data, _ := json.Marshal(body)
	resp, err := c.doRequest("POST", fmt.Sprintf("%s/api/channels/%s/messages", c.baseURL, channelID), data)
	if err != nil {
		return nil, fmt.Errorf("venus send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("venus send: status %d", resp.StatusCode)
	}

	var msg Message
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return nil, fmt.Errorf("venus send: decode: %w", err)
	}
	return &msg, nil
}

// RegisterWebhook registers a webhook URL with Venus.
func (c *Client) RegisterWebhook(webhookURL string) (*Webhook, error) {
	data, _ := json.Marshal(map[string]string{"url": webhookURL})
	resp, err := c.doRequest("POST", fmt.Sprintf("%s/api/webhooks", c.baseURL), data)
	if err != nil {
		return nil, fmt.Errorf("venus webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("venus webhook: status %d", resp.StatusCode)
	}

	var wh Webhook
	if err := json.NewDecoder(resp.Body).Decode(&wh); err != nil {
		return nil, fmt.Errorf("venus webhook: decode: %w", err)
	}
	return &wh, nil
}

// Health checks if Venus is reachable (health endpoint is auth-exempt).
func (c *Client) Health() error {
	resp, err := c.http.Get(fmt.Sprintf("%s/health", c.baseURL))
	if err != nil {
		return fmt.Errorf("venus health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("venus health: status %d", resp.StatusCode)
	}
	return nil
}
