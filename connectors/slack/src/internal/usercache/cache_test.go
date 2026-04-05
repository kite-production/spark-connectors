package usercache

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCache_GetDisplayName(t *testing.T) {
	var callCount atomic.Int32

	cache := New(5*time.Minute, func(userID string) (string, error) {
		callCount.Add(1)
		if userID == "U123" {
			return "Alice", nil
		}
		return "", errors.New("not found")
	})

	// First call should fetch.
	name := cache.GetDisplayName("U123")
	if name != "Alice" {
		t.Errorf("got %q, want %q", name, "Alice")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 fetch call, got %d", callCount.Load())
	}

	// Second call should use cache.
	name = cache.GetDisplayName("U123")
	if name != "Alice" {
		t.Errorf("got %q, want %q", name, "Alice")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 fetch call (cached), got %d", callCount.Load())
	}
}

func TestCache_FetchError_FallsBackToUserID(t *testing.T) {
	cache := New(5*time.Minute, func(userID string) (string, error) {
		return "", errors.New("api error")
	})

	name := cache.GetDisplayName("U999")
	if name != "U999" {
		t.Errorf("got %q, want %q (fallback to user ID)", name, "U999")
	}
}

func TestCache_Expiry(t *testing.T) {
	var callCount atomic.Int32

	cache := New(1*time.Millisecond, func(userID string) (string, error) {
		callCount.Add(1)
		return "Name", nil
	})

	cache.GetDisplayName("U1")
	if callCount.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", callCount.Load())
	}

	time.Sleep(5 * time.Millisecond)

	cache.GetDisplayName("U1")
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls after expiry, got %d", callCount.Load())
	}
}
