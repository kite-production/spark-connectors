// Package usercache provides a cached lookup for Slack user display names.
package usercache

import (
	"sync"
	"time"
)

// entry holds a cached user display name with expiry.
type entry struct {
	displayName string
	fetchedAt   time.Time
}

// SlackUserInfoFunc abstracts the Slack API call to get user info.
type SlackUserInfoFunc func(userID string) (displayName string, err error)

// Cache provides a concurrency-safe cache for Slack user display names.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
	ttl     time.Duration
	fetch   SlackUserInfoFunc
}

// New creates a user cache with the given TTL and fetch function.
func New(ttl time.Duration, fetch SlackUserInfoFunc) *Cache {
	return &Cache{
		entries: make(map[string]entry),
		ttl:     ttl,
		fetch:   fetch,
	}
}

// GetDisplayName returns the display name for a user ID, fetching from
// Slack if not cached or expired.
func (c *Cache) GetDisplayName(userID string) string {
	c.mu.RLock()
	if e, ok := c.entries[userID]; ok && time.Since(e.fetchedAt) < c.ttl {
		c.mu.RUnlock()
		return e.displayName
	}
	c.mu.RUnlock()

	name, err := c.fetch(userID)
	if err != nil {
		return userID // Fall back to user ID on error.
	}

	c.mu.Lock()
	c.entries[userID] = entry{displayName: name, fetchedAt: time.Now()}
	c.mu.Unlock()

	return name
}
