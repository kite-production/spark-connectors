package service

import (
	"context"
	"errors"
	"testing"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/kite-production/spark/services/connector-slack/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

// mockSlackAPI implements SlackAPI for tests.
type mockSlackAPI struct {
	messages []postedMessage
	err      error
}

type postedMessage struct {
	channel string
	opts    []slack.MsgOption
}

func (m *mockSlackAPI) PostMessageContext(_ context.Context, channelID string, opts ...slack.MsgOption) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	m.messages = append(m.messages, postedMessage{channel: channelID, opts: opts})
	return channelID, "1234567890.100000", nil
}

// mockResolver implements normalizer.DisplayNameResolver.
type mockResolver struct{}

func (m *mockResolver) GetDisplayName(userID string) string { return userID }

// mockPublisher implements baseconnector.NATSPublisher.
type mockPublisher struct {
	published [][]byte
}

func (m *mockPublisher) Publish(subj string, data []byte, opts ...interface{}) (interface{}, error) {
	m.published = append(m.published, data)
	return nil, nil
}

func newTestService(api SlackAPI) (*Service, *baseconnector.BaseConnector) {
	base := baseconnector.New(baseconnector.Config{
		ConnectorID: "slack",
	})
	norm := normalizer.New("slack", &mockResolver{})
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	svc := New(base, norm, api, metrics)
	return svc, base
}

func TestSendMessage_Success(t *testing.T) {
	api := &mockSlackAPI{}
	svc, _ := newTestService(api)

	resp, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "C123",
		Text:   "Hello **world**",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageId == "" {
		t.Error("expected non-empty message ID")
	}
	if len(api.messages) != 1 {
		t.Fatalf("expected 1 message posted, got %d", len(api.messages))
	}
	if api.messages[0].channel != "C123" {
		t.Errorf("posted to %q, want %q", api.messages[0].channel, "C123")
	}
}

func TestSendMessage_WithThread(t *testing.T) {
	api := &mockSlackAPI{}
	svc, _ := newTestService(api)

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId:   "C123",
		Text:     "Reply",
		ThreadId: "1234567890.000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(api.messages))
	}
}

func TestSendMessage_APIError(t *testing.T) {
	api := &mockSlackAPI{err: errors.New("slack api error")}
	svc, _ := newTestService(api)

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "C123",
		Text:   "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSendMessage_Chunking(t *testing.T) {
	api := &mockSlackAPI{}
	svc, _ := newTestService(api)

	// Create a long message that needs chunking.
	longText := ""
	for i := 0; i < 100; i++ {
		longText += "This is a paragraph of text that fills up the message.\n\n"
	}

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "C123",
		Text:   longText,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.messages) < 2 {
		t.Errorf("expected multiple chunks, got %d messages", len(api.messages))
	}
}

func TestGetStatus(t *testing.T) {
	svc, base := newTestService(&mockSlackAPI{})
	base.SetHealthy(true)
	svc.SetWSConnected(true)

	resp, err := svc.GetStatus(context.Background(), &connectorpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ConnectorId != "slack" {
		t.Errorf("ConnectorId = %q, want %q", resp.ConnectorId, "slack")
	}
	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if !resp.Accounts[0].Connected {
		t.Error("expected account connected=true")
	}
}

func TestGetCapabilities(t *testing.T) {
	svc, _ := newTestService(&mockSlackAPI{})

	resp, err := svc.GetCapabilities(context.Background(), &connectorpb.GetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	caps := resp.Capabilities
	if !caps.SupportsThreads {
		t.Error("expected SupportsThreads=true")
	}
	if !caps.SupportsReactions {
		t.Error("expected SupportsReactions=true")
	}
	if !caps.SupportsEdit {
		t.Error("expected SupportsEdit=true")
	}
	if !caps.SupportsReply {
		t.Error("expected SupportsReply=true")
	}
	if !caps.SupportsAttachments {
		t.Error("expected SupportsAttachments=true")
	}
	if !caps.SupportsImages {
		t.Error("expected SupportsImages=true")
	}
	if caps.MaxMessageLength != 4000 {
		t.Errorf("MaxMessageLength = %d, want 4000", caps.MaxMessageLength)
	}
}

func TestListAccounts(t *testing.T) {
	svc, _ := newTestService(&mockSlackAPI{})
	svc.SetWSConnected(true)

	resp, err := svc.ListAccounts(context.Background(), &connectorpb.ListAccountsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if !resp.Accounts[0].Connected {
		t.Error("expected account connected=true")
	}
}

func TestHealthCheck(t *testing.T) {
	svc, base := newTestService(&mockSlackAPI{})

	// Not healthy initially.
	resp, err := svc.HealthCheck(context.Background(), &connectorpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "NOT_SERVING" {
		t.Errorf("Status = %q, want %q", resp.Status, "NOT_SERVING")
	}

	// Set healthy.
	base.SetHealthy(true)
	resp, err = svc.HealthCheck(context.Background(), &connectorpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "SERVING" {
		t.Errorf("Status = %q, want %q", resp.Status, "SERVING")
	}
}

func TestHandleMessageEvent_Filters(t *testing.T) {
	svc, _ := newTestService(&mockSlackAPI{})

	// Bot message should be filtered.
	svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:  "bot msg",
		BotID: "B123",
	}, "T789")

	// message_changed should be filtered.
	svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:    "edited",
		SubType: "message_changed",
	}, "T789")

	// Empty text should be filtered.
	svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text: "",
		User: "U123",
	}, "T789")
}
