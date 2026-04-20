// Package searxng is a minimal HTTP client for the SearXNG
// metasearch engine (https://docs.searxng.org/).
//
// SearXNG aggregates results from upstream search engines (Google,
// Bing, DuckDuckGo, Wikipedia, etc.) into a single JSON response.
// Spark runs its own SearXNG sidecar on the Docker network so search
// queries never leak to third-party search APIs.
package searxng

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a SearXNG instance over HTTP.
type Client struct {
	baseURL string
	httpc   *http.Client
}

// New returns a Client pointed at the given SearXNG base URL (e.g.
// "http://searxng:8080").
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpc:   &http.Client{Timeout: 20 * time.Second},
	}
}

// Result is a single search hit.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type rawResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type rawResponse struct {
	Results []rawResult `json:"results"`
}

// Search executes a query and returns at most maxResults hits.
// maxResults <= 0 defaults to 5.
func (c *Client) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("empty query")
	}
	if maxResults <= 0 {
		maxResults = 5
	}

	// SearXNG's JSON format is accessible via either GET /search
	// or POST /search. POST works reliably across all versions we
	// tested; GET is sometimes blocked for JSON in the default
	// limiter config.
	form := url.Values{}
	form.Set("q", query)
	form.Set("format", "json")
	form.Set("pageno", "1")
	form.Set("safesearch", "1")

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/search", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SparkHubble/1.0")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		snippet := string(body)
		if len(snippet) > 500 {
			snippet = snippet[:500] + "…"
		}
		return nil, fmt.Errorf("searxng http %d: %s", resp.StatusCode, snippet)
	}

	var raw rawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal searxng response: %w", err)
	}

	results := make([]Result, 0, maxResults)
	for _, r := range raw.Results {
		if len(results) >= maxResults {
			break
		}
		if r.URL == "" {
			continue
		}
		results = append(results, Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}
	return results, nil
}
