package service

import (
	"context"
	"errors"
	"testing"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/kite-production/spark/services/connector-googlechat/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	chat "google.golang.org/api/chat/v1"
)

// mockChatAPI implements chatapi.ChatAPI for tests.
type mockChatAPI struct {
	messages []sentMessage
	err      error
}

type sentMessage struct {
	spaceName  string
	text       string
	threadName string
}

func (m *mockChatAPI) CreateMessage(_ context.Context, spaceName, text, threadName string) (*chat.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.messages = append(m.messages, sentMessage{
		spaceName:  spaceName,
		text:       text,
		threadName: threadName,
	})
	return &chat.Message{
		Name: "spaces/S/messages/RESP1",
	}, nil
}

func newTestService(api *mockChatAPI) (*Service, *baseconnector.BaseConnector) {
	base := baseconnector.New(baseconnector.Config{
		ConnectorID: "googlechat",
	})
	norm := normalizer.New("googlechat")
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	svc := New(base, norm, api, metrics)
	return svc, base
}

func TestSendMessage_Success(t *testing.T) {
	api := &mockChatAPI{}
	svc, _ := newTestService(api)

	resp, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "spaces/S",
		Text:   "Hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageId == "" {
		t.Error("expected non-empty message ID")
	}
	if len(api.messages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(api.messages))
	}
	if api.messages[0].spaceName != "spaces/S" {
		t.Errorf("sent to %q, want %q", api.messages[0].spaceName, "spaces/S")
	}
	if api.messages[0].text != "Hello world" {
		t.Errorf("text = %q, want %q", api.messages[0].text, "Hello world")
	}
}

func TestSendMessage_WithThread(t *testing.T) {
	api := &mockChatAPI{}
	svc, _ := newTestService(api)

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId:   "spaces/S",
		Text:     "Reply",
		ThreadId: "spaces/S/threads/T1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(api.messages))
	}
	if api.messages[0].threadName != "spaces/S/threads/T1" {
		t.Errorf("threadName = %q, want %q", api.messages[0].threadName, "spaces/S/threads/T1")
	}
}

func TestSendMessage_APIError(t *testing.T) {
	api := &mockChatAPI{err: errors.New("chat api error")}
	svc, _ := newTestService(api)

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "spaces/S",
		Text:   "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSendMessage_TruncatesLongMessage(t *testing.T) {
	api := &mockChatAPI{}
	svc, _ := newTestService(api)

	longText := make([]byte, 5000)
	for i := range longText {
		longText[i] = 'a'
	}

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "spaces/S",
		Text:   string(longText),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.messages[0].text) != MaxGoogleChatMessageLength {
		t.Errorf("text length = %d, want %d", len(api.messages[0].text), MaxGoogleChatMessageLength)
	}
}

func TestGetStatus(t *testing.T) {
	svc, base := newTestService(&mockChatAPI{})
	base.SetHealthy(true)
	svc.SetWebhookUp(true)

	resp, err := svc.GetStatus(context.Background(), &connectorpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ConnectorId != "googlechat" {
		t.Errorf("ConnectorId = %q, want %q", resp.ConnectorId, "googlechat")
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
	svc, _ := newTestService(&mockChatAPI{})

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
	if caps.MaxMessageLength != 4096 {
		t.Errorf("MaxMessageLength = %d, want 4096", caps.MaxMessageLength)
	}
}

func TestListAccounts(t *testing.T) {
	svc, _ := newTestService(&mockChatAPI{})
	svc.SetWebhookUp(true)

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
	svc, base := newTestService(&mockChatAPI{})

	resp, err := svc.HealthCheck(context.Background(), &connectorpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "NOT_SERVING" {
		t.Errorf("Status = %q, want %q", resp.Status, "NOT_SERVING")
	}

	base.SetHealthy(true)
	resp, err = svc.HealthCheck(context.Background(), &connectorpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "SERVING" {
		t.Errorf("Status = %q, want %q", resp.Status, "SERVING")
	}
}

func TestHandleEvent_Filters(t *testing.T) {
	svc, _ := newTestService(&mockChatAPI{})

	// Non-MESSAGE type should be filtered.
	svc.HandleEvent(&normalizer.ChatEvent{
		Type:    "ADDED_TO_SPACE",
		Message: &normalizer.ChatMessage{Text: "hi"},
		Space:   &normalizer.ChatSpace{Name: "spaces/S"},
	})

	// Empty text should be filtered.
	svc.HandleEvent(&normalizer.ChatEvent{
		Type:    "MESSAGE",
		Message: &normalizer.ChatMessage{Text: ""},
		Space:   &normalizer.ChatSpace{Name: "spaces/S"},
	})

	// Nil message should be filtered.
	svc.HandleEvent(&normalizer.ChatEvent{
		Type:  "MESSAGE",
		Space: &normalizer.ChatSpace{Name: "spaces/S"},
	})
}
