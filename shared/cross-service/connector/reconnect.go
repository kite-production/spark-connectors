package connector

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"
)

// BackoffConfig controls the exponential backoff behavior for
// reconnection and retry logic.
type BackoffConfig struct {
	// InitialInterval is the first wait duration after a failure.
	InitialInterval time.Duration

	// MaxInterval caps the backoff duration.
	MaxInterval time.Duration

	// Multiplier scales the interval after each failure.
	Multiplier float64

	// MaxRetries is the maximum number of attempts (0 = unlimited).
	MaxRetries int
}

// DefaultBackoffConfig returns sensible defaults for reconnection.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      10,
	}
}

// NextInterval calculates the backoff interval for the given attempt
// number (0-indexed). The result is capped at MaxInterval.
func (c BackoffConfig) NextInterval(attempt int) time.Duration {
	interval := float64(c.InitialInterval) * math.Pow(c.Multiplier, float64(attempt))
	if interval > float64(c.MaxInterval) {
		interval = float64(c.MaxInterval)
	}
	return time.Duration(interval)
}

// WithBackoff executes fn repeatedly with exponential backoff until it
// succeeds, the context is cancelled, or MaxRetries is exceeded.
func WithBackoff(ctx context.Context, cfg BackoffConfig, fn func() error) error {
	var lastErr error
	for attempt := 0; cfg.MaxRetries == 0 || attempt < cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled during backoff: %w", err)
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		log.Printf("attempt %d failed: %v", attempt+1, lastErr)

		if cfg.MaxRetries > 0 && attempt >= cfg.MaxRetries-1 {
			break
		}

		wait := cfg.NextInterval(attempt)
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during backoff wait: %w", ctx.Err())
		case <-time.After(wait):
		}
	}
	return fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxRetries, lastErr)
}
