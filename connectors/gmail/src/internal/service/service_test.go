package service

import (
	"context"
	"errors"
	"testing"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/kite-production/spark/services/connector-gmail/internal/normalizer"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	gm "google.golang.org/api/gmail/v1"
)

// mockGmailAPI implements gmailclient.GmailAPI for tests.
type mockGmailAPI struct {
	sentMsgs []*gm.Message
	sendErr  error
}

func (m *mockGmailAPI) ListMessages(_ context.Context, _, _ string) ([]*gm.Message, error) {
	return nil, nil
}

func (m *mockGmailAPI) GetMessage(_ context.Context, _, _ string) (*gm.Message, error) {
	return nil, nil
}

func (m *mockGmailAPI) SendMessage(_ context.Context, _ string, msg *gm.Message) (*gm.Message, error) {
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return &gm.Message{Id: "sent123"}, nil
}

func newTestService(api *mockGmailAPI) (*Service, *baseconnector.BaseConnector) {
	base := baseconnector.New(baseconnector.Config{
		ConnectorID: "gmail",
	})
	norm := normalizer.New("gmail", "test@example.com")
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	svc := New(base, norm, api, "test@example.com", metrics)
	return svc, base
}

func TestSendMessage_Success(t *testing.T) {
	api := &mockGmailAPI{}
	svc, _ := newTestService(api)

	resp, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "bob@example.com",
		Text:   "Hello Bob!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageId != "sent123" {
		t.Errorf("MessageId = %q, want %q", resp.MessageId, "sent123")
	}
	if len(api.sentMsgs) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(api.sentMsgs))
	}
	if api.sentMsgs[0].Raw == "" {
		t.Error("expected non-empty raw MIME content")
	}
}

func TestSendMessage_WithThread(t *testing.T) {
	api := &mockGmailAPI{}
	svc, _ := newTestService(api)

	// Pre-cache a thread subject.
	svc.subjectsMu.Lock()
	svc.subjects["thread123"] = "Original Subject"
	svc.subjectsMu.Unlock()

	resp, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId:   "bob@example.com",
		Text:     "Reply text",
		ThreadId: "thread123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageId == "" {
		t.Error("expected non-empty message ID")
	}
	if len(api.sentMsgs) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(api.sentMsgs))
	}
	if api.sentMsgs[0].ThreadId != "thread123" {
		t.Errorf("ThreadId = %q, want %q", api.sentMsgs[0].ThreadId, "thread123")
	}
}

func TestSendMessage_APIError(t *testing.T) {
	api := &mockGmailAPI{sendErr: errors.New("gmail api error")}
	svc, _ := newTestService(api)

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "bob@example.com",
		Text:   "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetStatus(t *testing.T) {
	svc, base := newTestService(&mockGmailAPI{})
	base.SetHealthy(true)
	svc.SetPolling(true)

	resp, err := svc.GetStatus(context.Background(), &connectorpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ConnectorId != "gmail" {
		t.Errorf("ConnectorId = %q, want %q", resp.ConnectorId, "gmail")
	}
	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if resp.Accounts[0].AccountId != "test@example.com" {
		t.Errorf("AccountId = %q, want %q", resp.Accounts[0].AccountId, "test@example.com")
	}
	if !resp.Accounts[0].Connected {
		t.Error("expected account connected=true")
	}
}

func TestGetCapabilities(t *testing.T) {
	svc, _ := newTestService(&mockGmailAPI{})

	resp, err := svc.GetCapabilities(context.Background(), &connectorpb.GetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	caps := resp.Capabilities
	if !caps.SupportsThreads {
		t.Error("expected SupportsThreads=true")
	}
	if caps.SupportsReactions {
		t.Error("expected SupportsReactions=false")
	}
	if caps.SupportsEdit {
		t.Error("expected SupportsEdit=false")
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
	if caps.MaxMessageLength != 0 {
		t.Errorf("MaxMessageLength = %d, want 0", caps.MaxMessageLength)
	}
}

func TestListAccounts(t *testing.T) {
	svc, _ := newTestService(&mockGmailAPI{})
	svc.SetPolling(true)

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
	if resp.Accounts[0].AccountId != "test@example.com" {
		t.Errorf("AccountId = %q, want %q", resp.Accounts[0].AccountId, "test@example.com")
	}
}

func TestHealthCheck(t *testing.T) {
	svc, base := newTestService(&mockGmailAPI{})

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

func TestHandleMessage_CachesSubject(t *testing.T) {
	api := &mockGmailAPI{}
	svc, _ := newTestService(api)

	// Set up a mock publisher so PublishInbound doesn't fail.
	svc.base.Publisher = &mockPublisher{}

	msg := &gm.Message{
		Id:       "msg1",
		ThreadId: "thread1",
		Payload: &gm.MessagePart{
			Headers: []*gm.MessagePartHeader{
				{Name: "From", Value: "alice@example.com"},
				{Name: "Subject", Value: "Important Topic"},
			},
		},
	}

	svc.HandleMessage(msg)

	svc.subjectsMu.RLock()
	cached, ok := svc.subjects["thread1"]
	svc.subjectsMu.RUnlock()

	if !ok {
		t.Fatal("expected subject to be cached")
	}
	if cached != "Important Topic" {
		t.Errorf("cached subject = %q, want %q", cached, "Important Topic")
	}
}

// mockPublisher implements baseconnector.NATSPublisher.
type mockPublisher struct{}

func (m *mockPublisher) Publish(_ string, _ []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	return &nats.PubAck{}, nil
}
