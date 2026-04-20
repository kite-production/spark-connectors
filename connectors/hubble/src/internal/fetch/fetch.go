// Package fetch implements web-fetch — a URL fetcher that strips
// HTML to readable text and caps output length.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	tagRegexp        = regexp.MustCompile(`<[^>]+>`)
	whitespaceRegexp = regexp.MustCompile(`\s+`)
)

// Result is what the web-fetch tool returns to the caller.
type Result struct {
	URL     string `json:"url"`
	Content string `json:"content"`
	Length  int    `json:"length"`
}

// Fetcher retrieves URLs with an HTTP client that has a sensible
// timeout and strips HTML so the LLM sees readable text rather than
// markup.
type Fetcher struct {
	client *http.Client
}

// New returns a Fetcher with a 15-second per-request timeout.
func New() *Fetcher {
	return &Fetcher{client: &http.Client{Timeout: 15 * time.Second}}
}

// Fetch issues a GET, strips HTML tags, and truncates the result
// to maxLength characters. maxLength <= 0 defaults to 8000.
func (f *Fetcher) Fetch(ctx context.Context, url string, maxLength int) (Result, error) {
	if strings.TrimSpace(url) == "" {
		return Result{}, fmt.Errorf("empty url")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return Result{}, fmt.Errorf("url must start with http:// or https://")
	}
	if maxLength <= 0 {
		maxLength = 8000
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "SparkHubble/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml,text/plain;q=0.9,*/*;q=0.5")

	resp, err := f.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB safety cap
	if err != nil {
		return Result{}, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		snippet := string(body)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "…"
		}
		return Result{}, fmt.Errorf("http %d from %s: %s", resp.StatusCode, url, snippet)
	}

	ctype := strings.ToLower(resp.Header.Get("Content-Type"))
	text := string(body)
	// For HTML/XML responses, strip markup. For JSON / plain text,
	// return the body as-is so consumers can parse directly.
	if strings.Contains(ctype, "html") || strings.Contains(ctype, "xml") || ctype == "" {
		text = stripHTML(text)
	}
	text = strings.TrimSpace(text)

	truncated := false
	if len(text) > maxLength {
		text = text[:maxLength] + "… (truncated)"
		truncated = true
	}

	reportedLen := len(text)
	if truncated {
		reportedLen -= len("… (truncated)")
	}

	return Result{
		URL:     url,
		Content: text,
		Length:  reportedLen,
	}, nil
}

func stripHTML(html string) string {
	t := tagRegexp.ReplaceAllString(html, " ")
	return whitespaceRegexp.ReplaceAllString(t, " ")
}
