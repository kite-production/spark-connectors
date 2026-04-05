package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchPages_FiltersByLastEdited(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-2 * time.Hour)
	recent := now.Add(-30 * time.Second)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{
					"id":               "page-1",
					"last_edited_time": recent.Format(time.RFC3339),
					"last_edited_by":   map[string]string{"id": "user-1"},
					"properties":       map[string]any{},
					"url":              "https://notion.so/page-1",
				},
				{
					"id":               "page-2",
					"last_edited_time": old.Format(time.RFC3339),
					"last_edited_by":   map[string]string{"id": "user-2"},
					"properties":       map[string]any{},
					"url":              "https://notion.so/page-2",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient("test-token")
	client.baseURL = ts.URL

	cutoff := now.Add(-1 * time.Hour)
	pages, err := client.SearchPages(context.Background(), "", cutoff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1 (only recent)", len(pages))
	}
	if pages[0].ID != "page-1" {
		t.Errorf("page ID = %q, want %q", pages[0].ID, "page-1")
	}
	if pages[0].LastEditedByID != "user-1" {
		t.Errorf("LastEditedByID = %q, want %q", pages[0].LastEditedByID, "user-1")
	}
}

func TestGetBlockChildren(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{
					"id":   "block-1",
					"type": "paragraph",
					"paragraph": map[string]any{
						"rich_text": []map[string]any{
							{"type": "text", "text": map[string]string{"content": "Hello"}},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient("test-token")
	client.baseURL = ts.URL

	blocks, err := client.GetBlockChildren(context.Background(), "page-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Type != "paragraph" {
		t.Errorf("Type = %q, want %q", blocks[0].Type, "paragraph")
	}
}

func TestAppendBlocks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{"id": "new-block-1", "type": "paragraph"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient("test-token")
	client.baseURL = ts.URL

	blocks := []Block{
		{
			Type: "paragraph",
			Paragraph: &RichTextBlock{
				RichText: []RichText{{Type: "text", Text: TextBody{Content: "test"}}},
			},
		},
	}

	blockID, err := client.AppendBlocks(context.Background(), "page-1", blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blockID != "new-block-1" {
		t.Errorf("blockID = %q, want %q", blockID, "new-block-1")
	}
}

func TestRateLimitHandling(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"message":"rate limited"}`)
	}))
	defer ts.Close()

	client := NewClient("test-token")
	client.baseURL = ts.URL

	_, err := client.SearchPages(context.Background(), "", time.Time{})
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
	}
	if rle.RetryAfterSec != 5 {
		t.Errorf("RetryAfterSec = %d, want 5", rle.RetryAfterSec)
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{
			name: "title property",
			props: map[string]any{
				"title": map[string]any{
					"title": []any{
						map[string]any{"plain_text": "My Page"},
					},
				},
			},
			want: "My Page",
		},
		{
			name: "Name property (database)",
			props: map[string]any{
				"Name": map[string]any{
					"title": []any{
						map[string]any{"plain_text": "DB Entry"},
					},
				},
			},
			want: "DB Entry",
		},
		{
			name:  "empty properties",
			props: map[string]any{},
			want:  "",
		},
		{
			name:  "nil properties",
			props: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitle(tt.props)
			if got != tt.want {
				t.Errorf("extractTitle = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthorizationHeader(t *testing.T) {
	var gotAuth string
	var gotVersion string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("Notion-Version")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results":[]}`)
	}))
	defer ts.Close()

	client := NewClient("my-secret-token")
	client.baseURL = ts.URL

	_, _ = client.SearchPages(context.Background(), "", time.Time{})

	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-secret-token")
	}
	if gotVersion != notionAPIVersion {
		t.Errorf("Notion-Version = %q, want %q", gotVersion, notionAPIVersion)
	}
}
