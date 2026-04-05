// Package service implements the ConnectorService gRPC server for Gmail.
package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/prometheus/client_golang/prometheus"
	gmailclient "github.com/kite-production/spark/services/connector-gmail/internal/gmail"
	mimecomposer "github.com/kite-production/spark/services/connector-gmail/internal/mime"
	"github.com/kite-production/spark/services/connector-gmail/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	gm "google.golang.org/api/gmail/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Metrics holds Prometheus metrics for the Gmail connector.
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
			Help: "Total inbound messages received from Gmail.",
		}),
		OutboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_outbound_messages_total",
			Help: "Total outbound messages sent via Gmail.",
		}),
		OutboundDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "spark_connector_outbound_duration_seconds",
			Help:    "Duration of outbound Gmail API calls.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.InboundTotal, m.OutboundTotal, m.OutboundDur)
	return m
}

// Service implements the ConnectorService gRPC interface for Gmail.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer

	base       *baseconnector.BaseConnector
	normalizer *normalizer.Normalizer
	gmailAPI   gmailclient.GmailAPI
	userID     string
	metrics    *Metrics
	polling    atomic.Bool

	// threadSubjects caches thread ID to subject for reply composition.
	subjectsMu sync.RWMutex
	subjects   map[string]string
}

// New creates a new Gmail connector service.
func New(base *baseconnector.BaseConnector, norm *normalizer.Normalizer, api gmailclient.GmailAPI, userID string, metrics *Metrics) *Service {
	return &Service{
		base:       base,
		normalizer: norm,
		gmailAPI:   api,
		userID:     userID,
		metrics:    metrics,
		subjects:   make(map[string]string),
	}
}

// HandleMessage processes an incoming Gmail message from the poller.
func (s *Service) HandleMessage(msg *gm.Message) {
	inbound := s.normalizer.Normalize(msg)

	// Cache the subject for thread replies.
	subject := normalizer.GetSubject(msg)
	if subject != "" && msg.ThreadId != "" {
		s.subjectsMu.Lock()
		s.subjects[msg.ThreadId] = subject
		s.subjectsMu.Unlock()
	}

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

	to := req.GetPeerId()
	subject := ""

	// Look up thread subject for replies.
	if threadID := req.GetThreadId(); threadID != "" {
		s.subjectsMu.RLock()
		cached, ok := s.subjects[threadID]
		s.subjectsMu.RUnlock()
		if ok {
			subject = mimecomposer.ReplySubject(cached)
		}
	}

	raw := mimecomposer.ComposeEmail(to, subject, req.GetText())

	gmailMsg := &gm.Message{
		Raw:      raw,
		ThreadId: req.GetThreadId(),
	}

	sent, err := s.gmailAPI.SendMessage(ctx, s.userID, gmailMsg)
	if err != nil {
		return nil, fmt.Errorf("sending email via Gmail: %w", err)
	}

	if s.metrics != nil {
		s.metrics.OutboundTotal.Inc()
		s.metrics.OutboundDur.Observe(time.Since(start).Seconds())
	}

	return &connectorpb.SendMessageResponse{
		MessageId: sent.Id,
		SentAt:    timestamppb.Now(),
	}, nil
}

// GetStatus implements ConnectorService.GetStatus.
func (s *Service) GetStatus(_ context.Context, _ *connectorpb.GetStatusRequest) (*connectorpb.GetStatusResponse, error) {
	return &connectorpb.GetStatusResponse{
		ConnectorId: s.base.Config.ConnectorID,
		Healthy:     s.base.IsHealthy(),
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  s.userID,
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
		Capabilities: gmailCapabilities(),
	}, nil
}

// ListAccounts implements ConnectorService.ListAccounts.
func (s *Service) ListAccounts(_ context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  s.userID,
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

// gmailCapabilities returns the declared capabilities for the Gmail connector.
func gmailCapabilities() *connectorpb.ConnectorCapabilities {
	return &connectorpb.ConnectorCapabilities{
		SupportsThreads:     true,
		SupportsReply:       true,
		SupportsAttachments: true,
		SupportsImages:      true,
		MaxMessageLength:    0,
	}
}
