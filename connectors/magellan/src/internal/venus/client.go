// Package venus is a minimal HTTP client for the Venus test
// messaging service, used by the on-demand magellan connector.
//
// Every tool call spawns a fresh container, calls one of these
// methods, writes JSON to stdout, and exits. No connection pooling,
// no persistent state — each invocation pays one TCP handshake to
// Venus, which runs on the same Docker network.
package venus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client wraps the Venus HTTP API.
type Client struct {
	baseURL string
	apiKey  string
	httpc   *http.Client
}

// New returns a Client pointed at the given Venus base URL
// (e.g. http://venus:8090) with Bearer-auth using apiKey.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

// do performs the HTTP request with standard headers. body may be nil.
func (c *Client) do(method, path string, body []byte) ([]byte, error) {
	var req *http.Request
	var err error
	url := c.baseURL + path
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("venus http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		snippet := string(raw)
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		return nil, fmt.Errorf("venus http %d: %s", resp.StatusCode, snippet)
	}
	return raw, nil
}

// ── Tool-implementing methods ────────────────────────────────────────

// SendMessage posts to /api/channels/{channel_id}/messages.
func (c *Client) SendMessage(channelID, text, threadID, replyToID string) ([]byte, error) {
	body := map[string]string{
		// Sender identity is set here rather than in the args
		// because the agent's "magellan" identity is what should
		// appear in the channel, not whatever the LLM wrote.
		"sender_id":   "magellan",
		"sender_name": "Spark Agent",
		"text":        text,
	}
	if threadID != "" {
		body["thread_id"] = threadID
	}
	if replyToID != "" {
		body["reply_to_id"] = replyToID
	}
	data, _ := json.Marshal(body)
	return c.do("POST", fmt.Sprintf("/api/channels/%s/messages", channelID), data)
}

// ListMessages returns messages in a channel, optionally since a
// specified ISO-8601 cursor.
func (c *Client) ListMessages(channelID, since string) ([]byte, error) {
	path := fmt.Sprintf("/api/channels/%s/messages", channelID)
	if since != "" {
		path += "?since=" + since
	}
	return c.do("GET", path, nil)
}

// CreateChannel creates a new channel.
func (c *Client) CreateChannel(name, description string) ([]byte, error) {
	body := map[string]string{"name": name}
	if description != "" {
		body["description"] = description
	}
	data, _ := json.Marshal(body)
	return c.do("POST", "/api/channels", data)
}

// ListChannels returns all Venus channels.
func (c *Client) ListChannels() ([]byte, error) {
	return c.do("GET", "/api/channels", nil)
}

// RegisterWebhook registers a webhook URL with Venus.
func (c *Client) RegisterWebhook(url string) ([]byte, error) {
	data, _ := json.Marshal(map[string]string{"url": url})
	return c.do("POST", "/api/webhooks", data)
}

// GetStatus returns Venus service status.
func (c *Client) GetStatus() ([]byte, error) {
	return c.do("GET", "/api/status", nil)
}
