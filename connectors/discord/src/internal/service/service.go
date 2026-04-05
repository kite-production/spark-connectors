// Package service implements the ConnectorService gRPC server for Discord.
package service

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	discord "github.com/kite-production/spark/services/connector-discord/internal/discord"
	"github.com/kite-production/spark/services/connector-discord/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Metrics holds Prometheus metrics for the connector.
type Metrics struct {
	InboundTotal  prometheus.Counter
	OutboundTotal prometheus.Counter
	OutboundDur   prometheus.Histogram
}

// NewMetrics creates and registers Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		InboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_discord_inbound_messages_total",
			Help: "Total inbound messages received from Discord.",
		}),
		OutboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_discord_outbound_messages_total",
			Help: "Total outbound messages sent to Discord.",
		}),
		OutboundDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "spark_connector_discord_outbound_duration_seconds",
			Help:    "Duration of outbound Discord API calls.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.InboundTotal, m.OutboundTotal, m.OutboundDur)
	return m
}

// Service implements the ConnectorService gRPC interface for Discord.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer

	base        *baseconnector.BaseConnector
	normalizer  *normalizer.Normalizer
	client      *discord.Client
	metrics     *Metrics
	wsConnected atomic.Bool
}

// New creates a new Discord connector service.
func New(base *baseconnector.BaseConnector, norm *normalizer.Normalizer, client *discord.Client, metrics *Metrics) *Service {
	return &Service{
		base:       base,
		normalizer: norm,
		client:     client,
		metrics:    metrics,
	}
}

// SetWSConnected updates the WebSocket connection state.
func (s *Service) SetWSConnected(connected bool) {
	s.wsConnected.Store(connected)
}

// HandleMessageCreate processes an incoming Discord message event.
func (s *Service) HandleMessageCreate(_ *discordgo.Session, m *discordgo.MessageCreate) {
	if !normalizer.ShouldProcess(m) {
		return
	}

	msg := s.normalizer.Normalize(m)
	if err := s.base.PublishInbound(msg); err != nil {
		log.Printf("failed to publish inbound message: %v", err)
		return
	}

	if s.metrics != nil {
		s.metrics.InboundTotal.Inc()
	}
}

// SendMessage implements ConnectorService.SendMessage.
func (s *Service) SendMessage(_ context.Context, req *connectorpb.SendMessageRequest) (*connectorpb.SendMessageResponse, error) {
	start := time.Now()

	text := req.GetText()

	// Discord has a 2000-character limit; chunk if needed.
	chunks := chunkMessage(text, maxDiscordMessageLength)

	var lastID string
	replyTo := req.GetThreadId()

	for _, chunk := range chunks {
		msgID, err := s.client.SendMessage(req.GetPeerId(), chunk, replyTo)
		if err != nil {
			return nil, fmt.Errorf("sending message to Discord: %w", err)
		}
		lastID = msgID
		// Only reply-reference the first chunk.
		replyTo = ""
	}

	if s.metrics != nil {
		s.metrics.OutboundTotal.Inc()
		s.metrics.OutboundDur.Observe(time.Since(start).Seconds())
	}

	return &connectorpb.SendMessageResponse{
		MessageId: lastID,
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
			Connected:  s.wsConnected.Load(),
		}},
		CheckedAt: timestamppb.Now(),
	}, nil
}

// GetCapabilities implements ConnectorService.GetCapabilities.
func (s *Service) GetCapabilities(_ context.Context, _ *connectorpb.GetCapabilitiesRequest) (*connectorpb.GetCapabilitiesResponse, error) {
	return &connectorpb.GetCapabilitiesResponse{
		ConnectorId:  s.base.Config.ConnectorID,
		Capabilities: discordCapabilities(),
	}, nil
}

// ListAccounts implements ConnectorService.ListAccounts.
func (s *Service) ListAccounts(_ context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
			Enabled:    true,
			Configured: true,
			Connected:  s.wsConnected.Load(),
		}},
	}, nil
}

// HealthCheck implements ConnectorService.HealthCheck.
func (s *Service) HealthCheck(ctx context.Context, req *connectorpb.HealthCheckRequest) (*connectorpb.HealthCheckResponse, error) {
	return s.base.HealthCheck(ctx, req)
}

// discordCapabilities returns the declared capabilities for the Discord connector.
func discordCapabilities() *connectorpb.ConnectorCapabilities {
	return &connectorpb.ConnectorCapabilities{
		SupportsThreads:     true,
		SupportsReactions:   true,
		SupportsEdit:        true,
		SupportsReply:       true,
		SupportsAttachments: true,
		SupportsImages:      true,
		MaxMessageLength:    2000,
	}
}

const maxDiscordMessageLength = 2000

// chunkMessage splits text into chunks of at most maxLen characters,
// breaking at newline boundaries when possible.
func chunkMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		// Try to break at a newline within the limit.
		cutoff := maxLen
		for i := maxLen - 1; i > maxLen/2; i-- {
			if text[i] == '\n' {
				cutoff = i + 1
				break
			}
		}

		chunks = append(chunks, text[:cutoff])
		text = text[cutoff:]
	}
	return chunks
}
