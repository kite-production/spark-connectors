// Package service implements the ConnectorService gRPC interface for Magellan.
package service

import (
	"context"
	"log"
	"sync/atomic"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"github.com/kite-production/spark/services/connector-magellan/internal/venus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service implements the ConnectorService gRPC interface for Magellan.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer
	base        *baseconnector.BaseConnector
	venusClient *venus.Client
	connected   atomic.Bool
}

// New creates a new Magellan ConnectorService.
func New(base *baseconnector.BaseConnector, vc *venus.Client) *Service {
	svc := &Service{
		base:        base,
		venusClient: vc,
	}
	svc.connected.Store(true)
	return svc
}

// SendMessage delivers a message from Spark to Venus.
func (s *Service) SendMessage(ctx context.Context, req *connectorpb.SendMessageRequest) (*connectorpb.SendMessageResponse, error) {
	log.Printf("[magellan] sending message to Venus channel %s: %s", req.GetPeerId(), truncate(req.GetText(), 80))

	msg, err := s.venusClient.SendMessage(
		req.GetPeerId(),
		"magellan",         // senderID — identifies connector
		"Spark Agent",      // senderName — display name
		req.GetText(),
		req.GetThreadId(),
		req.GetReplyToId(),
	)
	if err != nil {
		log.Printf("[magellan] send failed: %v", err)
		return nil, err
	}

	return &connectorpb.SendMessageResponse{
		MessageId: msg.ID,
		SentAt:    timestamppb.Now(),
	}, nil
}

// GetStatus returns the connector's current status.
func (s *Service) GetStatus(ctx context.Context, _ *connectorpb.GetStatusRequest) (*connectorpb.GetStatusResponse, error) {
	return &connectorpb.GetStatusResponse{
		ConnectorId: s.base.Config.ConnectorID,
		Healthy:     s.base.IsHealthy(),
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
			Enabled:    true,
			Configured: true,
			Connected:  s.connected.Load(),
		}},
		CheckedAt: timestamppb.Now(),
	}, nil
}

// GetCapabilities returns what Magellan supports.
func (s *Service) GetCapabilities(ctx context.Context, _ *connectorpb.GetCapabilitiesRequest) (*connectorpb.GetCapabilitiesResponse, error) {
	return &connectorpb.GetCapabilitiesResponse{
		ConnectorId: s.base.Config.ConnectorID,
		Capabilities: &connectorpb.ConnectorCapabilities{
			SupportsThreads:     true,
			SupportsReactions:   false,
			SupportsEdit:        false,
			SupportsUnsend:      false,
			SupportsReply:       true,
			SupportsAttachments: false,
			SupportsImages:      false,
			MaxMessageLength:    10000,
		},
	}, nil
}

// ListAccounts returns the accounts this connector manages.
func (s *Service) ListAccounts(ctx context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
			Enabled:    true,
			Configured: true,
			Connected:  s.connected.Load(),
		}},
	}, nil
}

// HealthCheck delegates to BaseConnector.
func (s *Service) HealthCheck(ctx context.Context, req *connectorpb.HealthCheckRequest) (*connectorpb.HealthCheckResponse, error) {
	return s.base.HealthCheck(ctx, req)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
