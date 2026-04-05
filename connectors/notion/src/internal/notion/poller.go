package notion

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// PageEvent represents a detected page update from the poller.
type PageEvent struct {
	Page   Page
	Text   string
	Blocks []Block
}

// Poller polls the Notion search API for recently edited pages.
type Poller struct {
	api       NotionAPI
	query     string
	interval  time.Duration
	onMessage func(*PageEvent)

	mu            sync.Mutex
	lastPollTime  time.Time
	processedKeys map[string]struct{}
}

// NewPoller creates a Poller that checks for page updates.
func NewPoller(api NotionAPI, query string, interval time.Duration, handler func(*PageEvent)) *Poller {
	return &Poller{
		api:           api,
		query:         query,
		interval:      interval,
		onMessage:     handler,
		processedKeys: make(map[string]struct{}),
	}
}

// Run starts the polling loop. It blocks until the context is cancelled.
func (p *Poller) Run(ctx context.Context) error {
	// Do an initial poll immediately.
	p.poll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// LastPollTime returns the last successful poll timestamp.
func (p *Poller) LastPollTime() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastPollTime
}

func (p *Poller) poll(ctx context.Context) {
	p.mu.Lock()
	lastPoll := p.lastPollTime
	p.mu.Unlock()

	if lastPoll.IsZero() {
		// First poll: look back 2 minutes.
		lastPoll = time.Now().Add(-2 * time.Minute)
	}

	pages, err := p.api.SearchPages(ctx, p.query, lastPoll)
	if err != nil {
		var rle *RateLimitError
		if errors.As(err, &rle) {
			log.Printf("notion rate limited, will retry in %ds", rle.RetryAfterSec)
		} else if ctx.Err() == nil {
			log.Printf("notion poll error: %v", err)
		}
		return
	}

	now := time.Now()

	for _, page := range pages {
		// Build idempotency key to avoid reprocessing.
		key := idempotencyKey(page.ID, page.LastEditedTime)

		p.mu.Lock()
		_, seen := p.processedKeys[key]
		if !seen {
			p.processedKeys[key] = struct{}{}
		}
		p.mu.Unlock()

		if seen {
			continue
		}

		// Fetch page block content.
		blocks, err := p.api.GetBlockChildren(ctx, page.ID)
		if err != nil {
			var rle *RateLimitError
			if errors.As(err, &rle) {
				log.Printf("notion rate limited fetching blocks for %s", page.ID)
			} else if ctx.Err() == nil {
				log.Printf("failed to get blocks for page %s: %v", page.ID, err)
			}
			continue
		}

		text := ExtractText(blocks)

		p.onMessage(&PageEvent{
			Page:   page,
			Text:   text,
			Blocks: blocks,
		})
	}

	p.mu.Lock()
	p.lastPollTime = now
	// Prune old keys to prevent unbounded growth (keep last 1000).
	if len(p.processedKeys) > 1000 {
		p.processedKeys = make(map[string]struct{})
	}
	p.mu.Unlock()
}

func idempotencyKey(pageID string, lastEdited time.Time) string {
	return pageID + ":" + lastEdited.Format(time.RFC3339)
}
