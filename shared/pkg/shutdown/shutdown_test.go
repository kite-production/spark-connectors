package shutdown

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestGracefulGRPC_DrainsWithinGracePeriod(t *testing.T) {
	srv := grpc.NewServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()

	m := New("test-service", 5*time.Second)
	done := make(chan struct{})
	go func() {
		m.GracefulGRPC(srv)
		close(done)
	}()

	select {
	case <-done:
		// success — server drained within grace period
	case <-time.After(3 * time.Second):
		t.Fatal("GracefulGRPC did not complete within expected time")
	}
}

func TestGracefulGRPC_ForcesStopOnTimeout(t *testing.T) {
	srv := grpc.NewServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()

	// Use a very short grace period to trigger force stop.
	m := New("test-service", 1*time.Millisecond)
	done := make(chan struct{})
	go func() {
		m.GracefulGRPC(srv)
		close(done)
	}()

	select {
	case <-done:
		// success — force stop triggered
	case <-time.After(3 * time.Second):
		t.Fatal("GracefulGRPC did not force stop within expected time")
	}
}

func TestGracefulHTTP(t *testing.T) {
	shutdownCalled := false
	m := New("test-service", 5*time.Second)
	m.GracefulHTTP("health-server", func(ctx context.Context) error {
		shutdownCalled = true
		return nil
	})
	if !shutdownCalled {
		t.Fatal("expected shutdown function to be called")
	}
}

func TestRunSteps_ExecutesInOrder(t *testing.T) {
	var order []string
	m := New("test-service", 5*time.Second)
	m.Run([]Step{
		{Name: "step-1", Fn: func() error { order = append(order, "1"); return nil }},
		{Name: "step-2", Fn: func() error { order = append(order, "2"); return nil }},
		{Name: "step-3", Fn: func() error { order = append(order, "3"); return nil }},
	})
	if len(order) != 3 || order[0] != "1" || order[1] != "2" || order[2] != "3" {
		t.Fatalf("steps executed out of order: %v", order)
	}
}

func TestRunSteps_ContinuesOnError(t *testing.T) {
	var executed []string
	m := New("test-service", 5*time.Second)
	m.Run([]Step{
		{Name: "ok-1", Fn: func() error { executed = append(executed, "1"); return nil }},
		{Name: "fail", Fn: func() error { executed = append(executed, "2"); return errors.New("boom") }},
		{Name: "ok-3", Fn: func() error { executed = append(executed, "3"); return nil }},
	})
	if len(executed) != 3 {
		t.Fatalf("expected all 3 steps to execute, got %d", len(executed))
	}
}

func TestDefaultGracePeriod(t *testing.T) {
	if DefaultGracePeriod != 30*time.Second {
		t.Fatalf("expected 30s, got %s", DefaultGracePeriod)
	}
}
