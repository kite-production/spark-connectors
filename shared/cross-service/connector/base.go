// Package connector provides shared infrastructure for connector services.
// Future connectors (Slack, Gmail, Google Chat, etc.) import this package
// to get NATS publishing, gRPC server setup, control-plane registration,
// and reconnection logic out of the box.
package connector

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	controlplanepb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/controlplane"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
)

// Config holds the configuration for a BaseConnector.
type Config struct {
	// ConnectorID is the unique identifier for this connector instance.
	ConnectorID string

	// GRPCAddress is the address where this connector's gRPC server listens
	// (e.g., "localhost:50061"). Reported to control-plane during registration.
	GRPCAddress string

	// GRPCPort is the port for the gRPC server (e.g., "50061").
	GRPCPort string

	// NATSUrl is the NATS server URL.
	NATSUrl string

	// ControlPlaneAddress is the gRPC address of the control-plane service.
	ControlPlaneAddress string

	// Capabilities describes the features this connector supports.
	Capabilities *connectorpb.ConnectorCapabilities

	// BackoffConfig controls reconnection behavior.
	BackoffConfig BackoffConfig
}

// NATSPublisher abstracts NATS JetStream publishing for testability.
type NATSPublisher interface {
	Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// ControlPlaneClient abstracts the control-plane gRPC client for testability.
type ControlPlaneClient interface {
	RegisterConnector(ctx context.Context, in *controlplanepb.RegisterConnectorRequest, opts ...grpc.CallOption) (*controlplanepb.RegisterConnectorResponse, error)
	DeregisterConnector(ctx context.Context, in *controlplanepb.DeregisterConnectorRequest, opts ...grpc.CallOption) (*controlplanepb.DeregisterConnectorResponse, error)
}

// NATSConnector abstracts NATS connection establishment for testability.
type NATSConnector func(cfg Config) (*nats.Conn, NATSPublisher, error)

// CPDialer abstracts control-plane connection establishment for testability.
type CPDialer func(address string) (*grpc.ClientConn, ControlPlaneClient, error)

// BaseConnector provides shared infrastructure that all connector
// implementations embed. It manages the NATS publisher, gRPC server,
// control-plane registration, and health state.
type BaseConnector struct {
	Config     Config
	Publisher  NATSPublisher
	CPClient   ControlPlaneClient
	GRPCServer *grpc.Server
	NATSConn   *nats.Conn
	CPConn     *grpc.ClientConn

	// ConnectNATS is the function used to establish a NATS connection.
	// Override in tests.
	ConnectNATS NATSConnector

	// DialCP is the function used to connect to the control-plane.
	// Override in tests.
	DialCP CPDialer

	mu         sync.RWMutex
	healthy    bool
	registered bool
	stopCh     chan struct{}
}

// New creates a new BaseConnector with the given config. It does not
// start any connections — call Start() to connect to NATS and register.
func New(cfg Config) *BaseConnector {
	if cfg.BackoffConfig.InitialInterval == 0 {
		cfg.BackoffConfig = DefaultBackoffConfig()
	}
	return &BaseConnector{
		Config:      cfg,
		ConnectNATS: defaultConnectNATS,
		DialCP:      defaultDialCP,
		healthy:     false,
		stopCh:      make(chan struct{}),
	}
}

// IsHealthy returns whether the connector is in a healthy state.
func (b *BaseConnector) IsHealthy() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.healthy
}

// SetHealthy updates the connector's health state.
func (b *BaseConnector) SetHealthy(healthy bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.healthy = healthy
}

// IsRegistered returns whether the connector has registered with the control-plane.
func (b *BaseConnector) IsRegistered() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.registered
}

// Start connects to NATS, sets up the gRPC server, and registers with
// the control-plane. It uses reconnection logic with exponential backoff
// if any step fails.
func (b *BaseConnector) Start(ctx context.Context, svc connectorpb.ConnectorServiceServer) error {
	// Connect to NATS with reconnection.
	err := WithBackoff(ctx, b.Config.BackoffConfig, func() error {
		nc, pub, err := b.ConnectNATS(b.Config)
		if err != nil {
			return err
		}
		b.NATSConn = nc
		b.Publisher = pub
		return nil
	})
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}

	// Set up gRPC server.
	b.GRPCServer = NewGRPCServer(svc)

	// Connect to control-plane and register.
	err = WithBackoff(ctx, b.Config.BackoffConfig, func() error {
		if b.Config.ControlPlaneAddress == "" {
			return fmt.Errorf("control-plane address not configured")
		}
		conn, client, err := b.DialCP(b.Config.ControlPlaneAddress)
		if err != nil {
			return err
		}
		b.CPConn = conn
		b.CPClient = client
		return b.Register(ctx)
	})
	if err != nil {
		return fmt.Errorf("registering with control-plane: %w", err)
	}

	b.SetHealthy(true)
	log.Printf("connector %s started successfully", b.Config.ConnectorID)
	return nil
}

// Stop deregisters from the control-plane and cleans up connections.
func (b *BaseConnector) Stop(ctx context.Context) {
	b.SetHealthy(false)

	close(b.stopCh)

	if b.IsRegistered() {
		if err := b.Deregister(ctx); err != nil {
			log.Printf("deregistration failed: %v", err)
		}
	}

	if b.GRPCServer != nil {
		b.GRPCServer.GracefulStop()
	}

	if b.NATSConn != nil {
		b.NATSConn.Close()
	}

	if b.CPConn != nil {
		b.CPConn.Close()
	}

	log.Printf("connector %s stopped", b.Config.ConnectorID)
}

// HealthCheck implements the ConnectorService HealthCheck RPC. Connector
// implementations can delegate to this for the standard health response.
func (b *BaseConnector) HealthCheck(_ context.Context, _ *connectorpb.HealthCheckRequest) (*connectorpb.HealthCheckResponse, error) {
	status := "SERVING"
	if !b.IsHealthy() {
		status = "NOT_SERVING"
	}
	return &connectorpb.HealthCheckResponse{
		Status:  status,
		Service: b.Config.ConnectorID,
	}, nil
}

func defaultConnectNATS(cfg Config) (*nats.Conn, NATSPublisher, error) {
	url := cfg.NATSUrl
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(url,
		nats.Name(fmt.Sprintf("connector-%s", cfg.ConnectorID)),
		nats.MaxReconnects(10),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("NATS connect: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("JetStream context: %w", err)
	}
	return nc, js, nil
}

func defaultDialCP(address string) (*grpc.ClientConn, ControlPlaneClient, error) {
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecureCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing control-plane: %w", err)
	}
	client := controlplanepb.NewControlPlaneServiceClient(conn)
	return conn, client, nil
}
