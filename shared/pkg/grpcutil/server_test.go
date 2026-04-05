package grpcutil

import (
	"testing"

	"google.golang.org/grpc"
)

func TestNewServer(t *testing.T) {
	srv := NewServer()
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	srv.Stop()
}

func TestNewServer_withOptions(t *testing.T) {
	srv := NewServer(grpc.MaxRecvMsgSize(4 * 1024 * 1024))
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	srv.Stop()
}
