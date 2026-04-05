// Package service implements the ConnectorService gRPC server for Telegram.
package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/kite-production/spark/services/connector-telegram/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TelegramAPI abstracts Telegram Bot API calls for testability.
type TelegramAPI interface {
	SendMessage(chatID int64, text string, replyToID int, parseMode string) (tgbotapi.Message, error)
	BotUsername() string
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
			Name: "spark_connector_telegram_inbound_messages_total",
			Help: "Total inbound messages received from Telegram.",
		}),
		OutboundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_telegram_outbound_messages_total",
			Help: "Total outbound messages sent to Telegram.",
		}),
		OutboundDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "spark_connector_telegram_outbound_duration_seconds",
			Help:    "Duration of outbound Telegram API calls.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.InboundTotal, m.OutboundTotal, m.OutboundDur)
	return m
}

// Service implements the ConnectorService gRPC interface for Telegram.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer

	base       *baseconnector.BaseConnector
	normalizer *normalizer.Normalizer
	tgAPI      TelegramAPI
	metrics    *Metrics
	polling    atomic.Bool
}

// New creates a new Telegram connector service.
func New(base *baseconnector.BaseConnector, norm *normalizer.Normalizer, api TelegramAPI, metrics *Metrics) *Service {
	return &Service{
		base:       base,
		normalizer: norm,
		tgAPI:      api,
		metrics:    metrics,
	}
}

// SetPolling updates the polling connection state.
func (s *Service) SetPolling(active bool) {
	s.polling.Store(active)
}

// HandleUpdate processes an incoming Telegram update.
func (s *Service) HandleUpdate(update tgbotapi.Update) {
	if !normalizer.ShouldProcess(update) {
		return
	}

	msg := s.normalizer.Normalize(update.Message)
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

	chatID, err := strconv.ParseInt(req.GetPeerId(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid peer_id (chat ID): %w", err)
	}

	var replyToID int
	if req.GetThreadId() != "" {
		id, err := strconv.Atoi(req.GetThreadId())
		if err != nil {
			return nil, fmt.Errorf("invalid thread_id (reply_to_message_id): %w", err)
		}
		replyToID = id
	}

	// Use Markdown parse mode for formatting support.
	sent, err := s.tgAPI.SendMessage(chatID, req.GetText(), replyToID, "Markdown")
	if err != nil {
		return nil, fmt.Errorf("sending message to Telegram: %w", err)
	}

	if s.metrics != nil {
		s.metrics.OutboundTotal.Inc()
		s.metrics.OutboundDur.Observe(time.Since(start).Seconds())
	}

	return &connectorpb.SendMessageResponse{
		MessageId: fmt.Sprintf("%d", sent.MessageID),
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
			Connected:  s.polling.Load(),
		}},
		CheckedAt: timestamppb.Now(),
	}, nil
}

// GetCapabilities implements ConnectorService.GetCapabilities.
func (s *Service) GetCapabilities(_ context.Context, _ *connectorpb.GetCapabilitiesRequest) (*connectorpb.GetCapabilitiesResponse, error) {
	return &connectorpb.GetCapabilitiesResponse{
		ConnectorId:  s.base.Config.ConnectorID,
		Capabilities: telegramCapabilities(),
	}, nil
}

// ListAccounts implements ConnectorService.ListAccounts.
func (s *Service) ListAccounts(_ context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
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

// telegramCapabilities returns the declared capabilities for the Telegram connector.
func telegramCapabilities() *connectorpb.ConnectorCapabilities {
	return &connectorpb.ConnectorCapabilities{
		SupportsThreads:     false,
		SupportsReactions:   true,
		SupportsEdit:        true,
		SupportsReply:       true,
		SupportsAttachments: true,
		SupportsImages:      true,
		MaxMessageLength:    4096,
	}
}
