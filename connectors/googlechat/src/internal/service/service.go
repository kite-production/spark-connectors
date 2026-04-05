// Package service implements the ConnectorService gRPC server for Google Chat.
package service

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/kite-production/spark/services/connector-googlechat/internal/chatapi"
	"github.com/kite-production/spark/services/connector-googlechat/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MaxGoogleChatMessageLength is the maximum message length for Google Chat.
const MaxGoogleChatMessageLength = 4096

// Metrics holds Prometheus metrics for the Google Chat connector.
type Metrics struct {
	InboundTotal  prometheus.Counter
	OutboundTotal prometheus.Counter
	OutboundDur   prometheus.Histogram
}

// NewMetrics creates and registers Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		InboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_inbound_messages_total",
			Help: "Total inbound messages received from Google Chat.",
		}),
		OutboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_outbound_messages_total",
			Help: "Total outbound messages sent to Google Chat.",
		}),
		OutboundDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "spark_connector_outbound_duration_seconds",
			Help:    "Duration of outbound Google Chat API calls.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.InboundTotal, m.OutboundTotal, m.OutboundDur)
	return m
}

// Service implements the ConnectorService gRPC interface for Google Chat.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer

	base       *baseconnector.BaseConnector
	normalizer *normalizer.Normalizer
	chatAPI    chatapi.ChatAPI
	metrics    *Metrics
	webhookUp  atomic.Bool
}

// New creates a new Google Chat connector service.
func New(base *baseconnector.BaseConnector, norm *normalizer.Normalizer, api chatapi.ChatAPI, metrics *Metrics) *Service {
	return &Service{
		base:       base,
		normalizer: norm,
		chatAPI:    api,
		metrics:    metrics,
	}
}

// SetWebhookUp updates the webhook listener state.
func (s *Service) SetWebhookUp(up bool) {
	s.webhookUp.Store(up)
}

// HandleEvent processes an incoming Google Chat webhook event.
func (s *Service) HandleEvent(ev *normalizer.ChatEvent) {
	if !normalizer.ShouldProcess(ev) {
		return
	}

	msg := s.normalizer.Normalize(ev)
	if err := s.base.PublishInbound(msg); err != nil {
		log.Printf("failed to publish inbound message: %v", err)
		return
	}

	if s.metrics != nil {
		s.metrics.InboundTotal.Inc()
	}
}

// SendMessage implements ConnectorService.SendMessage.
func (s *Service) SendMessage(ctx context.Context, req *connectorpb.SendMessageRequest) (*connectorpb.SendMessageResponse, error) {
	start := time.Now()

	text := req.GetText()
	if len(text) > MaxGoogleChatMessageLength {
		text = text[:MaxGoogleChatMessageLength]
	}

	resp, err := s.chatAPI.CreateMessage(ctx, req.GetPeerId(), text, req.GetThreadId())
	if err != nil {
		return nil, fmt.Errorf("sending message to Google Chat: %w", err)
	}

	if s.metrics != nil {
		s.metrics.OutboundTotal.Inc()
		s.metrics.OutboundDur.Observe(time.Since(start).Seconds())
	}

	return &connectorpb.SendMessageResponse{
		MessageId: resp.Name,
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
			Connected:  s.webhookUp.Load(),
		}},
		CheckedAt: timestamppb.Now(),
	}, nil
}

// GetCapabilities implements ConnectorService.GetCapabilities.
func (s *Service) GetCapabilities(_ context.Context, _ *connectorpb.GetCapabilitiesRequest) (*connectorpb.GetCapabilitiesResponse, error) {
	return &connectorpb.GetCapabilitiesResponse{
		ConnectorId:  s.base.Config.ConnectorID,
		Capabilities: googlechatCapabilities(),
	}, nil
}

// ListAccounts implements ConnectorService.ListAccounts.
func (s *Service) ListAccounts(_ context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
			Enabled:    true,
			Configured: true,
			Connected:  s.webhookUp.Load(),
		}},
	}, nil
}

// HealthCheck implements ConnectorService.HealthCheck.
func (s *Service) HealthCheck(ctx context.Context, req *connectorpb.HealthCheckRequest) (*connectorpb.HealthCheckResponse, error) {
	return s.base.HealthCheck(ctx, req)
}

// googlechatCapabilities returns the declared capabilities for the Google Chat connector.
func googlechatCapabilities() *connectorpb.ConnectorCapabilities {
	return &connectorpb.ConnectorCapabilities{
		SupportsThreads:   true,
		SupportsReactions: true,
		SupportsEdit:      true,
		SupportsReply:     true,
		MaxMessageLength:  MaxGoogleChatMessageLength,
	}
}
