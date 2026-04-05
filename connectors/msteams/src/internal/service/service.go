// Package service implements the ConnectorService gRPC server for Microsoft Teams.
package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/kite-production/spark/services/connector-msteams/internal/msteams"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service implements the ConnectorService gRPC interface for Microsoft Teams.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer
	base      *baseconnector.BaseConnector
	client    *msteams.Client
	connected atomic.Bool
}

// New creates a new MS Teams connector service.
func New(base *baseconnector.BaseConnector, client *msteams.Client) *Service {
	svc := &Service{
		base:   base,
		client: client,
	}
	svc.connected.Store(true)
	return svc
}

// SendMessage implements ConnectorService.SendMessage.
//
// For Teams channel messages, PeerId is expected to be in the format
// "teamID/channelID" (separated by "/"). For personal/group chats,
// PeerId is the chat conversation ID.
func (s *Service) SendMessage(ctx context.Context, req *connectorpb.SendMessageRequest) (*connectorpb.SendMessageResponse, error) {
	start := time.Now()

	// Convert markdown to HTML since Teams uses HTML formatting.
	htmlContent := msteams.MarkdownToHTML(req.GetText())

	peerID := req.GetPeerId()
	threadID := req.GetThreadId()

	var msgID string
	var err error

	// Check if peerID contains a "/" separator indicating teamID/channelID.
	if parts := strings.SplitN(peerID, "/", 2); len(parts) == 2 {
		// Channel message: teamID/channelID format.
		teamID := parts[0]
		channelID := parts[1]
		log.Printf("[msteams] sending channel message to team=%s channel=%s: %s",
			teamID, channelID, truncate(req.GetText(), 80))
		msgID, err = s.client.SendMessage(teamID, channelID, htmlContent, threadID)
	} else {
		// Personal or group chat: peerID is the chat conversation ID.
		log.Printf("[msteams] sending chat message to %s: %s",
			peerID, truncate(req.GetText(), 80))
		msgID, err = s.client.SendChatMessage(peerID, htmlContent)
	}

	if err != nil {
		log.Printf("[msteams] send failed: %v", err)
		return nil, fmt.Errorf("sending message to Teams: %w", err)
	}

	dur := time.Since(start)
	log.Printf("[msteams] message sent in %s: %s", dur, msgID)

	return &connectorpb.SendMessageResponse{
		MessageId: msgID,
		SentAt:    timestamppb.Now(),
	}, nil
}

// GetStatus implements ConnectorService.GetStatus.
func (s *Service) GetStatus(_ context.Context, _ *connectorpb.GetStatusRequest) (*connectorpb.GetStatusResponse, error) {
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

// GetCapabilities implements ConnectorService.GetCapabilities.
func (s *Service) GetCapabilities(_ context.Context, _ *connectorpb.GetCapabilitiesRequest) (*connectorpb.GetCapabilitiesResponse, error) {
	return &connectorpb.GetCapabilitiesResponse{
		ConnectorId:  s.base.Config.ConnectorID,
		Capabilities: teamsCapabilities(),
	}, nil
}

// ListAccounts implements ConnectorService.ListAccounts.
func (s *Service) ListAccounts(_ context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
			Enabled:    true,
			Configured: true,
			Connected:  s.connected.Load(),
		}},
	}, nil
}

// HealthCheck implements ConnectorService.HealthCheck.
func (s *Service) HealthCheck(ctx context.Context, req *connectorpb.HealthCheckRequest) (*connectorpb.HealthCheckResponse, error) {
	return s.base.HealthCheck(ctx, req)
}

// teamsCapabilities returns the declared capabilities for the Teams connector.
func teamsCapabilities() *connectorpb.ConnectorCapabilities {
	return &connectorpb.ConnectorCapabilities{
		SupportsThreads:     true,
		SupportsReactions:   true,
		SupportsEdit:        true,
		SupportsReply:       true,
		SupportsAttachments: true,
		MaxMessageLength:    28000,
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
