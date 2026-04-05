package natsutil

import (
	"testing"
	"time"
)

func TestDefaultConnectOptions(t *testing.T) {
	opts := DefaultConnectOptions()
	if opts.URL == "" {
		t.Error("expected non-empty URL")
	}
	if opts.MaxReconnects <= 0 {
		t.Error("expected positive MaxReconnects")
	}
	if opts.ReconnectWait <= 0 {
		t.Error("expected positive ReconnectWait")
	}
}

func TestConnect_invalidURL(t *testing.T) {
	opts := ConnectOptions{
		URL:           "nats://localhost:1", // unlikely to be running
		Name:          "test",
		MaxReconnects: 0,
		ReconnectWait: 100 * time.Millisecond,
	}
	_, err := Connect(opts)
	if err == nil {
		t.Fatal("expected error connecting to invalid URL")
	}
}
