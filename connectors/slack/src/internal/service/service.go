// Package service implements the ConnectorService gRPC server for Slack.
package service

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/kite-production/spark/services/connector-slack/internal/chunker"
	"github.com/kite-production/spark/services/connector-slack/internal/formatter"
	"github.com/kite-production/spark/services/connector-slack/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SlackAPI abstracts Slack API calls for testability.
type SlackAPI interface {
	PostMessageContext(ctx context.Context, channelID string, opts ...slack.MsgOption) (string, string, error)
}

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
			Name: "spark_connector_inbound_messages_total",
			Help: "Total inbound messages received from Slack.",
		}),
		OutboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_outbound_messages_total",
			Help: "Total outbound messages sent to Slack.",
		}),
		OutboundDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "spark_connector_outbound_duration_seconds",
			Help:    "Duration of outbound Slack API calls.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.InboundTotal, m.OutboundTotal, m.OutboundDur)
	return m
}

// Service implements the ConnectorService gRPC interface for Slack.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer

	base        *baseconnector.BaseConnector
	normalizer  *normalizer.Normalizer
	slackAPI    SlackAPI
	metrics     *Metrics
	wsConnected atomic.Bool
}

// New creates a new Slack connector service.
func New(base *baseconnector.BaseConnector, norm *normalizer.Normalizer, api SlackAPI, metrics *Metrics) *Service {
	return &Service{
		base:       base,
		normalizer: norm,
		slackAPI:   api,
		metrics:    metrics,
	}
}

// SetWSConnected updates the WebSocket connection state.
func (s *Service) SetWSConnected(connected bool) {
	s.wsConnected.Store(connected)
}

// HandleMessageEvent processes an incoming Slack message event.
func (s *Service) HandleMessageEvent(ev *slackevents.MessageEvent, teamID string) {
	if !normalizer.ShouldProcess(ev) {
		return
	}

	msg := s.normalizer.Normalize(ev, teamID)
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

	mrkdwn := formatter.MarkdownToMrkdwn(req.GetText())
	chunks := chunker.ChunkMessage(mrkdwn, chunker.MaxSlackMessageLength)

	var lastTS string
	threadTS := req.GetThreadId()

	for _, chunk := range chunks {
		opts := []slack.MsgOption{
			slack.MsgOptionText(chunk, false),
		}
		if threadTS != "" {
			opts = append(opts, slack.MsgOptionTS(threadTS))
		}

		_, ts, err := s.slackAPI.PostMessageContext(ctx, req.GetPeerId(), opts...)
		if err != nil {
			return nil, fmt.Errorf("posting message to Slack: %w", err)
		}
		lastTS = ts
	}

	if s.metrics != nil {
		s.metrics.OutboundTotal.Inc()
		s.metrics.OutboundDur.Observe(time.Since(start).Seconds())
	}

	return &connectorpb.SendMessageResponse{
		MessageId: lastTS,
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
		Capabilities: slackCapabilities(),
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

// slackCapabilities returns the declared capabilities for the Slack connector.
func slackCapabilities() *connectorpb.ConnectorCapabilities {
	return &connectorpb.ConnectorCapabilities{
		SupportsThreads:     true,
		SupportsReactions:   true,
		SupportsEdit:        true,
		SupportsReply:       true,
		SupportsAttachments: true,
		SupportsImages:      true,
		MaxMessageLength:    4000,
	}
}
