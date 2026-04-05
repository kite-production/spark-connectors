// Package service implements the ConnectorService gRPC server for the browser
// tool connector. This connector does not receive inbound messages — agents
// invoke it via SendMessage with JSON-encoded browser commands.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/kite-production/spark/services/connector-browser/internal/browser"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Metrics holds Prometheus metrics for the browser connector.
type Metrics struct {
	CommandsTotal prometheus.Counter
	CommandDur    prometheus.Histogram
	ErrorsTotal   prometheus.Counter
}

// NewMetrics creates and registers Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		CommandsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_browser_commands_total",
			Help: "Total browser commands executed.",
		}),
		CommandDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "spark_connector_browser_command_duration_seconds",
			Help:    "Duration of browser command execution.",
			Buckets: prometheus.DefBuckets,
		}),
		ErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "spark_connector_browser_errors_total",
			Help: "Total browser command errors.",
		}),
	}
	reg.MustRegister(m.CommandsTotal, m.CommandDur, m.ErrorsTotal)
	return m
}

// Command represents a browser action sent via SendMessage.
type Command struct {
	Action   string `json:"action"`
	URL      string `json:"url,omitempty"`
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text,omitempty"`
	Script   string `json:"script,omitempty"`
}

// CommandResult is the JSON response returned from a browser command.
type CommandResult struct {
	Success bool   `json:"success"`
	Action  string `json:"action"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Service implements the ConnectorService gRPC interface for the browser connector.
type Service struct {
	connectorpb.UnimplementedConnectorServiceServer

	base    *baseconnector.BaseConnector
	browser *browser.Browser
	metrics *Metrics
}

// New creates a new browser connector service.
func New(base *baseconnector.BaseConnector, b *browser.Browser, metrics *Metrics) *Service {
	return &Service{
		base:    base,
		browser: b,
		metrics: metrics,
	}
}

// SendMessage implements ConnectorService.SendMessage.
// The request text field contains a JSON-encoded browser command.
func (s *Service) SendMessage(ctx context.Context, req *connectorpb.SendMessageRequest) (*connectorpb.SendMessageResponse, error) {
	start := time.Now()

	if s.metrics != nil {
		s.metrics.CommandsTotal.Inc()
	}

	var cmd Command
	if err := json.Unmarshal([]byte(req.GetText()), &cmd); err != nil {
		return nil, fmt.Errorf("parsing browser command: %w", err)
	}

	result, err := s.executeCommand(ctx, &cmd)
	if err != nil {
		if s.metrics != nil {
			s.metrics.ErrorsTotal.Inc()
		}
		errResult := CommandResult{
			Success: false,
			Action:  cmd.Action,
			Error:   err.Error(),
		}
		respJSON, _ := json.Marshal(errResult)
		return &connectorpb.SendMessageResponse{
			MessageId: string(respJSON),
			SentAt:    timestamppb.Now(),
		}, nil
	}

	if s.metrics != nil {
		s.metrics.CommandDur.Observe(time.Since(start).Seconds())
	}

	successResult := CommandResult{
		Success: true,
		Action:  cmd.Action,
		Result:  result,
	}
	respJSON, _ := json.Marshal(successResult)

	return &connectorpb.SendMessageResponse{
		MessageId: string(respJSON),
		SentAt:    timestamppb.Now(),
	}, nil
}

// executeCommand dispatches a browser command to the appropriate method.
func (s *Service) executeCommand(ctx context.Context, cmd *Command) (string, error) {
	switch cmd.Action {
	case "navigate":
		if cmd.URL == "" {
			return "", fmt.Errorf("navigate requires 'url' field")
		}
		if err := s.browser.Navigate(ctx, cmd.URL); err != nil {
			return "", fmt.Errorf("navigate: %w", err)
		}
		return cmd.URL, nil

	case "screenshot":
		data, err := s.browser.Screenshot(ctx)
		if err != nil {
			return "", fmt.Errorf("screenshot: %w", err)
		}
		return data, nil

	case "click":
		if cmd.Selector == "" {
			return "", fmt.Errorf("click requires 'selector' field")
		}
		if err := s.browser.Click(ctx, cmd.Selector); err != nil {
			return "", fmt.Errorf("click: %w", err)
		}
		return "clicked", nil

	case "type":
		if cmd.Selector == "" {
			return "", fmt.Errorf("type requires 'selector' field")
		}
		if err := s.browser.Type(ctx, cmd.Selector, cmd.Text); err != nil {
			return "", fmt.Errorf("type: %w", err)
		}
		return "typed", nil

	case "evaluate":
		if cmd.Script == "" {
			return "", fmt.Errorf("evaluate requires 'script' field")
		}
		result, err := s.browser.Evaluate(ctx, cmd.Script)
		if err != nil {
			return "", fmt.Errorf("evaluate: %w", err)
		}
		return result, nil

	case "get_text":
		if cmd.Selector == "" {
			return "", fmt.Errorf("get_text requires 'selector' field")
		}
		text, err := s.browser.GetText(ctx, cmd.Selector)
		if err != nil {
			return "", fmt.Errorf("get_text: %w", err)
		}
		return text, nil

	default:
		return "", fmt.Errorf("unknown action: %s", cmd.Action)
	}
}

// GetStatus implements ConnectorService.GetStatus.
func (s *Service) GetStatus(_ context.Context, _ *connectorpb.GetStatusRequest) (*connectorpb.GetStatusResponse, error) {
	browserAlive := s.browser.Alive()

	return &connectorpb.GetStatusResponse{
		ConnectorId: s.base.Config.ConnectorID,
		Healthy:     s.base.IsHealthy() && browserAlive,
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
			Enabled:    true,
			Configured: true,
			Connected:  browserAlive,
		}},
		CheckedAt: timestamppb.Now(),
	}, nil
}

// GetCapabilities implements ConnectorService.GetCapabilities.
// The browser connector is a tool, not a messaging channel.
func (s *Service) GetCapabilities(_ context.Context, _ *connectorpb.GetCapabilitiesRequest) (*connectorpb.GetCapabilitiesResponse, error) {
	return &connectorpb.GetCapabilitiesResponse{
		ConnectorId: s.base.Config.ConnectorID,
		Capabilities: &connectorpb.ConnectorCapabilities{
			SupportsThreads:     false,
			SupportsReactions:   false,
			SupportsEdit:        false,
			SupportsReply:       false,
			SupportsAttachments: false,
			SupportsImages:      false,
			MaxMessageLength:    0,
		},
	}, nil
}

// ListAccounts implements ConnectorService.ListAccounts.
func (s *Service) ListAccounts(_ context.Context, _ *connectorpb.ListAccountsRequest) (*connectorpb.ListAccountsResponse, error) {
	return &connectorpb.ListAccountsResponse{
		Accounts: []*connectorpb.ConnectorAccount{{
			AccountId:  "default",
			Enabled:    true,
			Configured: true,
			Connected:  s.browser.Alive(),
		}},
	}, nil
}

// HealthCheck implements ConnectorService.HealthCheck.
func (s *Service) HealthCheck(ctx context.Context, req *connectorpb.HealthCheckRequest) (*connectorpb.HealthCheckResponse, error) {
	resp, err := s.base.HealthCheck(ctx, req)
	if err != nil {
		return nil, err
	}

	// Override status if browser is not responsive.
	if !s.browser.Alive() {
		resp.Status = "NOT_SERVING"
		log.Printf("browser health check: Chrome process not responsive")
	}

	return resp, nil
}
