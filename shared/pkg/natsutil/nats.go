// Package natsutil provides NATS connection helpers and JetStream setup utilities.
package natsutil

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// ConnectOptions holds configuration for establishing a NATS connection.
type ConnectOptions struct {
	URL            string
	Name           string
	MaxReconnects  int
	ReconnectWait  time.Duration
}

// DefaultConnectOptions returns sensible defaults for local development.
func DefaultConnectOptions() ConnectOptions {
	return ConnectOptions{
		URL:           nats.DefaultURL,
		Name:          "kite-service",
		MaxReconnects: 10,
		ReconnectWait: 2 * time.Second,
	}
}

// Connect establishes a NATS connection with the given options.
func Connect(opts ConnectOptions) (*nats.Conn, error) {
	nc, err := nats.Connect(
		opts.URL,
		nats.Name(opts.Name),
		nats.MaxReconnects(opts.MaxReconnects),
		nats.ReconnectWait(opts.ReconnectWait),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS at %s: %w", opts.URL, err)
	}
	return nc, nil
}

// EnsureStream creates or updates a JetStream stream with the given config.
func EnsureStream(js nats.JetStreamContext, cfg *nats.StreamConfig) (*nats.StreamInfo, error) {
	info, err := js.StreamInfo(cfg.Name)
	if err == nil {
		info, err = js.UpdateStream(cfg)
		if err != nil {
			return nil, fmt.Errorf("updating stream %s: %w", cfg.Name, err)
		}
		return info, nil
	}

	info, err = js.AddStream(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating stream %s: %w", cfg.Name, err)
	}
	return info, nil
}
