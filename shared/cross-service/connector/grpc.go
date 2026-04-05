package connector

import (
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/kite-production/spark/pkg/grpcutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// NewGRPCServer creates a new gRPC server with standard interceptors and
// registers the provided ConnectorService implementation.
func NewGRPCServer(svc connectorpb.ConnectorServiceServer, opts ...grpc.ServerOption) *grpc.Server {
	srv := grpcutil.NewServer(opts...)
	connectorpb.RegisterConnectorServiceServer(srv, svc)
	return srv
}

// insecureCredentials returns insecure transport credentials.
// Internal helper used for control-plane client connections.
func insecureCredentials() credentials.TransportCredentials {
	return insecure.NewCredentials()
}
