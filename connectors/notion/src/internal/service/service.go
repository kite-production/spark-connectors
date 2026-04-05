// Package service implements the ConnectorService gRPC server for Notion.
package service

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/kite-production/spark/services/connector-notion/internal/normalizer"
	"github.com/kite-production/spark/services/connector-notion/internal/notion"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Metrics holds Prometheus metrics for the Notion connector.
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
			Help: "Total inbound messages received from Notion.",
		}),
		OutboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_outbound_messages_total",
			Help: "Total outbound messages sent via Notion.",
		}),
		OutboundDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "spark_connector_outbound_duration_seconds",
			Help:    "Duration of outbound Notion API calls.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.InboundTotal, m.OutboundTotal, m.OutboundDur)
	return m
}

// Service implements the ConnectorService gRPC interface for Notion.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer

	base       *baseconnector.BaseConnector
	normalizer *normalizer.Normalizer
	notionAPI  notion.NotionAPI
	accountID  string
	metrics    *Metrics
	polling    atomic.Bool
}

// New creates a new Notion connector service.
func New(base *baseconnector.BaseConnector, norm *normalizer.Normalizer, api notion.NotionAPI, accountID string, metrics *Metrics) *Service {
	return &Service{
		base:       base,
		normalizer: norm,
		notionAPI:  api,
		accountID:  accountID,
		metrics:    metrics,
	}
}

// HandlePageEvent processes an incoming Notion page event from the poller.
func (s *Service) HandlePageEvent(event *notion.PageEvent) {
	inbound := s.normalizer.Normalize(event)
	if err := s.base.PublishInbound(inbound); err != nil {
		log.Printf("failed to publish inbound message: %v", err)
		return
	}
	if s.metrics != nil {
		s.metrics.InboundTotal.Inc()
	}
}

// SetPolling updates the polling state.
func (s *Service) SetPolling(polling bool) {
	s.polling.Store(polling)
}

// SendMessage implements ConnectorService.SendMessage.
func (s *Service) SendMessage(ctx context.Context, req *connectorpb.SendMessageRequest) (*connectorpb.SendMessageResponse, error) {
	start := time.Now()

	blocks := notion.ComposeTextBlocks(req.GetText())
	blockID, err := s.notionAPI.AppendBlocks(ctx, req.GetPeerId(), blocks)
	if err != nil {
		return nil, fmt.Errorf("appending blocks to Notion page: %w", err)
	}

	if s.metrics != nil {
		s.metrics.OutboundTotal.Inc()
		s.metrics.OutboundDur.Observe(time.Since(start).Seconds())
	}

	return &connectorpb.SendMessageResponse{
		MessageId: blockID,
		SentAt:    timestamppb.Now(),
	}, nil
}

// GetStatus implements ConnectorService.GetStatus.
func (s *Service) GetStatus(_ context.Context, _ *connectorpb.GetStatusRequest) (*connectorpb.GetStatusResponse, error) {
	return &connectorpb.GetStatusResponse{
		ConnectorId: s.base.Config.ConnectorID,
		Healthy:     s.base.IsHealthy(),
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  s.accountID,
			Enabled:    true,
			Configured: true,
			Connected:  s.polling.Load(),
		}},
		CheckedAt: timestamppb.Now(),
	}, nil
}

// GetCapabilities implements ConnectorService.GetCapabilities.
func (s *Service) GetCapabilities(_ context.Context, _ *connectorpb.GetCapabilitiesRequest) (*connectorpb.GetCapabilitiesResponse, error) {
	return &connectorpb.GetCapabilitiesResponse{
		ConnectorId:  s.base.Config.ConnectorID,
		Capabilities: NotionCapabilities(),
	}, nil
}

// ListAccounts implements ConnectorService.ListAccounts.
func (s *Service) ListAccounts(_ context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  s.accountID,
			Enabled:    true,
			Configured: true,
			Connected:  s.polling.Load(),
		}},
	}, nil
}

// HealthCheck implements ConnectorService.HealthCheck.
func (s *Service) HealthCheck(ctx context.Context, req *connectorpb.HealthCheckRequest) (*connectorpb.HealthCheckResponse, error) {
	return s.base.HealthCheck(ctx, req)
}

// NotionCapabilities returns the declared capabilities for the Notion connector.
func NotionCapabilities() *connectorpb.ConnectorCapabilities {
	return &connectorpb.ConnectorCapabilities{
		SupportsThreads:     false,
		SupportsReply:       false,
		SupportsAttachments: true,
		SupportsImages:      true,
		MaxMessageLength:    0,
	}
}
