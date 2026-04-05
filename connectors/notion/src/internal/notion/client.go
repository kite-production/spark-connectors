// Package notion wraps the Notion REST API for polling and block operations.
package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultBaseURL    = "https://api.notion.com/v1"
	notionAPIVersion  = "2022-06-28"
	defaultRateLimit  = 3 // requests per second
)

// NotionAPI abstracts Notion API operations for testability.
type NotionAPI interface {
	SearchPages(ctx context.Context, query string, lastEditedAfter time.Time) ([]Page, error)
	GetBlockChildren(ctx context.Context, blockID string) ([]Block, error)
	AppendBlocks(ctx context.Context, pageID string, blocks []Block) (string, error)
}

// Page represents a Notion page from the search API.
type Page struct {
	ID             string
	LastEditedTime time.Time
	LastEditedByID string
	Title          string
	URL            string
}

// Block represents a Notion block (paragraph, heading, etc.).
type Block struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type"`

	Paragraph *RichTextBlock `json:"paragraph,omitempty"`
	Heading1  *RichTextBlock `json:"heading_1,omitempty"`
	Heading2  *RichTextBlock `json:"heading_2,omitempty"`
	Heading3  *RichTextBlock `json:"heading_3,omitempty"`
}

// RichTextBlock contains rich text items for a block.
type RichTextBlock struct {
	RichText []RichText `json:"rich_text"`
}

// RichText represents a Notion rich text object.
type RichText struct {
	Type string   `json:"type"`
	Text TextBody `json:"text"`
}

// TextBody holds the text content of a rich text element.
type TextBody struct {
	Content string `json:"content"`
}

// Client implements NotionAPI using the real Notion REST API.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
}

// NewClient creates a Notion API client with the given integration token.
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
		baseURL:    defaultBaseURL,
	}
}

// SearchPages searches for recently edited pages using the Notion search API.
func (c *Client) SearchPages(ctx context.Context, query string, lastEditedAfter time.Time) ([]Page, error) {
	body := map[string]any{
		"filter": map[string]string{
			"value":    "page",
			"property": "object",
		},
		"sort": map[string]string{
			"direction": "descending",
			"timestamp": "last_edited_time",
		},
	}
	if query != "" {
		body["query"] = query
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling search body: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/search", data)
	if err != nil {
		return nil, fmt.Errorf("searching pages: %w", err)
	}

	var result searchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing search response: %w", err)
	}

	var pages []Page
	for _, r := range result.Results {
		edited, _ := time.Parse(time.RFC3339, r.LastEditedTime)
		if !lastEditedAfter.IsZero() && edited.Before(lastEditedAfter) {
			continue
		}

		title := extractTitle(r.Properties)
		pages = append(pages, Page{
			ID:             r.ID,
			LastEditedTime: edited,
			LastEditedByID: r.LastEditedBy.ID,
			Title:          title,
			URL:            r.URL,
		})
	}

	return pages, nil
}

// GetBlockChildren retrieves all child blocks of a given block (or page).
func (c *Client) GetBlockChildren(ctx context.Context, blockID string) ([]Block, error) {
	path := fmt.Sprintf("/blocks/%s/children?page_size=100", blockID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting block children: %w", err)
	}

	var result blockChildrenResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing block children: %w", err)
	}

	return result.Results, nil
}

// AppendBlocks appends blocks to a page using the blocks.children.append API.
func (c *Client) AppendBlocks(ctx context.Context, pageID string, blocks []Block) (string, error) {
	body := map[string]any{
		"children": blocks,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling append body: %w", err)
	}

	path := fmt.Sprintf("/blocks/%s/children", pageID)
	resp, err := c.doRequest(ctx, http.MethodPatch, path, data)
	if err != nil {
		return "", fmt.Errorf("appending blocks: %w", err)
	}

	var result blockChildrenResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing append response: %w", err)
	}

	if len(result.Results) > 0 {
		return result.Results[0].ID, nil
	}
	return pageID, nil
}

// doRequest executes an authenticated HTTP request to the Notion API,
// handling rate limit (Retry-After) responses.
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", notionAPIVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Handle rate limiting with Retry-After.
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		waitSec, _ := strconv.Atoi(retryAfter)
		if waitSec <= 0 {
			waitSec = 1
		}
		return nil, &RateLimitError{RetryAfterSec: waitSec}
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("notion API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// RateLimitError indicates the Notion API returned 429 Too Many Requests.
type RateLimitError struct {
	RetryAfterSec int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("notion rate limited: retry after %ds", e.RetryAfterSec)
}

// --- Internal response types for JSON parsing ---

type searchResponse struct {
	Results []searchResult `json:"results"`
}

type searchResult struct {
	ID             string         `json:"id"`
	LastEditedTime string         `json:"last_edited_time"`
	LastEditedBy   userRef        `json:"last_edited_by"`
	Properties     map[string]any `json:"properties"`
	URL            string         `json:"url"`
}

type userRef struct {
	ID string `json:"id"`
}

type blockChildrenResponse struct {
	Results []Block `json:"results"`
}

// extractTitle extracts the page title from properties.
func extractTitle(props map[string]any) string {
	titleProp, ok := props["title"]
	if !ok {
		// Try "Name" property (common in databases).
		titleProp, ok = props["Name"]
		if !ok {
			return ""
		}
	}

	propMap, ok := titleProp.(map[string]any)
	if !ok {
		return ""
	}

	titleArr, ok := propMap["title"].([]any)
	if !ok {
		return ""
	}

	var title string
	for _, item := range titleArr {
		textItem, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if plainText, ok := textItem["plain_text"].(string); ok {
			title += plainText
		}
	}
	return title
}
