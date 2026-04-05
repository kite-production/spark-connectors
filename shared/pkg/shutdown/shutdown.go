// Package shutdown provides ordered graceful shutdown for Go services.
//
// It runs named shutdown steps in sequence, logging each step for
// observability. A typical usage drains gRPC connections with a timeout,
// then closes database and NATS connections.
package shutdown

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
)

// DefaultGracePeriod is the default timeout for draining in-flight requests.
const DefaultGracePeriod = 30 * time.Second

// Step represents a named shutdown action.
type Step struct {
	Name string
	Fn   func() error
}

// Manager coordinates ordered service shutdown.
type Manager struct {
	service     string
	gracePeriod time.Duration
}

// New creates a Manager for the given service name with the specified grace period.
func New(service string, gracePeriod time.Duration) *Manager {
	return &Manager{service: service, gracePeriod: gracePeriod}
}

// GracefulGRPC stops the gRPC server gracefully within the manager's grace
// period. If the grace period is exceeded, it forces the server to stop.
func (m *Manager) GracefulGRPC(server *grpc.Server) {
	log.Printf("[%s] shutdown: draining gRPC connections (grace=%s)", m.service, m.gracePeriod)
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[%s] shutdown: gRPC server drained gracefully", m.service)
	case <-time.After(m.gracePeriod):
		log.Printf("[%s] shutdown: grace period exceeded, forcing gRPC stop", m.service)
		server.Stop()
	}
}

// GracefulHTTP shuts down an HTTP-like server that supports Shutdown(ctx).
func (m *Manager) GracefulHTTP(name string, shutdownFn func(ctx context.Context) error) {
	log.Printf("[%s] shutdown: stopping %s", m.service, name)
	ctx, cancel := context.WithTimeout(context.Background(), m.gracePeriod)
	defer cancel()
	if err := shutdownFn(ctx); err != nil {
		log.Printf("[%s] shutdown: %s error: %v", m.service, name, err)
	}
}

// Run executes shutdown steps in order, logging each one.
func (m *Manager) Run(steps []Step) {
	for _, step := range steps {
		log.Printf("[%s] shutdown: %s", m.service, step.Name)
		if err := step.Fn(); err != nil {
			log.Printf("[%s] shutdown: %s error: %v", m.service, step.Name, err)
		}
	}
}
