package msteams

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client communicates with the Microsoft Graph API and Azure AD for Teams.
type Client struct {
	appID       string
	appPassword string
	tenantID    string
	http        *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewClient creates a Microsoft Teams Graph API client.
func NewClient(appID, appPassword, tenantID string) *Client {
	return &Client{
		appID:       appID,
		appPassword: appPassword,
		tenantID:    tenantID,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GetAccessToken returns a valid access token, refreshing if needed.
// Uses OAuth2 client credentials flow against Azure AD.
func (c *Client) GetAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with 2-minute buffer).
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-2*time.Minute)) {
		return c.accessToken, nil
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", c.tenantID)

	data := url.Values{
		"client_id":     {c.appID},
		"client_secret": {c.appPassword},
		"scope":         {"https://graph.microsoft.com/.default"},
		"grant_type":    {"client_credentials"},
	}

	resp, err := c.http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return c.accessToken, nil
}

// SendMessage posts a message to a Teams channel via the Graph API.
// If replyToID is non-empty, sends as a reply in that thread.
func (c *Client) SendMessage(teamID, channelID, content, replyToID string) (string, error) {
	token, err := c.GetAccessToken()
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}

	body := map[string]interface{}{
		"body": map[string]string{
			"contentType": "html",
			"content":     content,
		},
	}

	var apiURL string
	if replyToID != "" {
		// Reply to an existing message in a thread.
		apiURL = fmt.Sprintf(
			"https://graph.microsoft.com/v1.0/teams/%s/channels/%s/messages/%s/replies",
			teamID, channelID, replyToID,
		)
	} else {
		// New message in channel.
		apiURL = fmt.Sprintf(
			"https://graph.microsoft.com/v1.0/teams/%s/channels/%s/messages",
			teamID, channelID,
		)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("graph send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("graph send: status %d", resp.StatusCode)
	}

	var graphResp GraphMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&graphResp); err != nil {
		return "", fmt.Errorf("graph decode: %w", err)
	}

	return graphResp.ID, nil
}

// SendChatMessage posts a message to a personal/group chat (non-channel).
func (c *Client) SendChatMessage(chatID, content string) (string, error) {
	token, err := c.GetAccessToken()
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}

	body := map[string]interface{}{
		"body": map[string]string{
			"contentType": "html",
			"content":     content,
		},
	}

	apiURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/chats/%s/messages", chatID)

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("graph chat send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("graph chat send: status %d", resp.StatusCode)
	}

	var graphResp GraphMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&graphResp); err != nil {
		return "", fmt.Errorf("graph decode: %w", err)
	}

	return graphResp.ID, nil
}

// MarkdownToHTML converts basic markdown to HTML for Teams messages.
// Teams natively renders HTML, so we do a lightweight conversion.
func MarkdownToHTML(md string) string {
	text := md

	// Bold: **text** → <b>text</b>
	for strings.Contains(text, "**") {
		start := strings.Index(text, "**")
		rest := text[start+2:]
		end := strings.Index(rest, "**")
		if end == -1 {
			break
		}
		text = text[:start] + "<b>" + rest[:end] + "</b>" + rest[end+2:]
	}

	// Italic: *text* → <i>text</i> (avoid matching already-processed bold)
	for strings.Contains(text, "*") {
		start := strings.Index(text, "*")
		rest := text[start+1:]
		end := strings.Index(rest, "*")
		if end == -1 {
			break
		}
		text = text[:start] + "<i>" + rest[:end] + "</i>" + rest[end+1:]
	}

	// Code: `text` → <code>text</code>
	for strings.Contains(text, "`") {
		start := strings.Index(text, "`")
		rest := text[start+1:]
		end := strings.Index(rest, "`")
		if end == -1 {
			break
		}
		text = text[:start] + "<code>" + rest[:end] + "</code>" + rest[end+1:]
	}

	// Newlines → <br>
	text = strings.ReplaceAll(text, "\n", "<br>")

	return text
}
