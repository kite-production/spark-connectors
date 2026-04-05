package connector

import (
	"context"
	"fmt"
	"log"

	controlplanepb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/controlplane"
)

// Register calls the control-plane RegisterConnector RPC with this
// connector's ID, gRPC address, and capabilities.
func (b *BaseConnector) Register(ctx context.Context) error {
	if b.CPClient == nil {
		return fmt.Errorf("control-plane client not initialized")
	}

	resp, err := b.CPClient.RegisterConnector(ctx, &controlplanepb.RegisterConnectorRequest{
		ConnectorId:  b.Config.ConnectorID,
		GrpcAddress:  b.Config.GRPCAddress,
		Capabilities: b.Config.Capabilities,
	})
	if err != nil {
		return fmt.Errorf("RegisterConnector RPC: %w", err)
	}

	b.mu.Lock()
	b.registered = true
	b.mu.Unlock()

	log.Printf("connector %s registered at %s", resp.GetConnectorId(), resp.GetRegisteredAt().AsTime())
	return nil
}

// Deregister calls the control-plane DeregisterConnector RPC to
// gracefully remove this connector.
func (b *BaseConnector) Deregister(ctx context.Context) error {
	if b.CPClient == nil {
		return fmt.Errorf("control-plane client not initialized")
	}

	resp, err := b.CPClient.DeregisterConnector(ctx, &controlplanepb.DeregisterConnectorRequest{
		ConnectorId: b.Config.ConnectorID,
	})
	if err != nil {
		return fmt.Errorf("DeregisterConnector RPC: %w", err)
	}

	b.mu.Lock()
	b.registered = false
	b.mu.Unlock()

	log.Printf("connector %s deregistered at %s", resp.GetConnectorId(), resp.GetDeregisteredAt().AsTime())
	return nil
}

// RegisterWithRetry attempts registration with exponential backoff.
// This is useful when the control-plane may not be available at startup.
func (b *BaseConnector) RegisterWithRetry(ctx context.Context) error {
	return WithBackoff(ctx, b.Config.BackoffConfig, func() error {
		return b.Register(ctx)
	})
}
