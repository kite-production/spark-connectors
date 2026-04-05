package service

import (
	"context"
	"errors"
	"testing"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/kite-production/spark/services/connector-notion/internal/normalizer"
	"github.com/kite-production/spark/services/connector-notion/internal/notion"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

// mockNotionAPI implements notion.NotionAPI for tests.
type mockNotionAPI struct {
	appendErr    error
	appendBlocks []notion.Block
	appendID     string
}

func (m *mockNotionAPI) SearchPages(_ context.Context, _ string, _ time.Time) ([]notion.Page, error) {
	return nil, nil
}

func (m *mockNotionAPI) GetBlockChildren(_ context.Context, _ string) ([]notion.Block, error) {
	return nil, nil
}

func (m *mockNotionAPI) AppendBlocks(_ context.Context, _ string, blocks []notion.Block) (string, error) {
	if m.appendErr != nil {
		return "", m.appendErr
	}
	m.appendBlocks = blocks
	id := m.appendID
	if id == "" {
		id = "block-123"
	}
	return id, nil
}

// mockPublisher implements baseconnector.NATSPublisher.
type mockPublisher struct{}

func (m *mockPublisher) Publish(_ string, _ []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	return &nats.PubAck{}, nil
}

func newTestService(api *mockNotionAPI) (*Service, *baseconnector.BaseConnector) {
	base := baseconnector.New(baseconnector.Config{
		ConnectorID: "notion",
	})
	norm := normalizer.New("notion", "workspace-test")
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	svc := New(base, norm, api, "workspace-test", metrics)
	return svc, base
}

func TestSendMessage_Success(t *testing.T) {
	api := &mockNotionAPI{appendID: "new-block-1"}
	svc, _ := newTestService(api)

	resp, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "page-123",
		Text:   "Hello Notion!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageId != "new-block-1" {
		t.Errorf("MessageId = %q, want %q", resp.MessageId, "new-block-1")
	}
	if len(api.appendBlocks) != 1 {
		t.Fatalf("expected 1 block appended, got %d", len(api.appendBlocks))
	}
	if api.appendBlocks[0].Type != "paragraph" {
		t.Errorf("block type = %q, want %q", api.appendBlocks[0].Type, "paragraph")
	}
}

func TestSendMessage_MultipleParagraphs(t *testing.T) {
	api := &mockNotionAPI{}
	svc, _ := newTestService(api)

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "page-123",
		Text:   "First paragraph\n\nSecond paragraph",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.appendBlocks) != 2 {
		t.Fatalf("expected 2 blocks appended, got %d", len(api.appendBlocks))
	}
}

func TestSendMessage_APIError(t *testing.T) {
	api := &mockNotionAPI{appendErr: errors.New("notion api error")}
	svc, _ := newTestService(api)

	_, err := svc.SendMessage(context.Background(), &connectorpb.SendMessageRequest{
		PeerId: "page-123",
		Text:   "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetStatus(t *testing.T) {
	svc, base := newTestService(&mockNotionAPI{})
	base.SetHealthy(true)
	svc.SetPolling(true)

	resp, err := svc.GetStatus(context.Background(), &connectorpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ConnectorId != "notion" {
		t.Errorf("ConnectorId = %q, want %q", resp.ConnectorId, "notion")
	}
	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if resp.Accounts[0].AccountId != "workspace-test" {
		t.Errorf("AccountId = %q, want %q", resp.Accounts[0].AccountId, "workspace-test")
	}
	if !resp.Accounts[0].Connected {
		t.Error("expected account connected=true")
	}
}

func TestGetCapabilities(t *testing.T) {
	svc, _ := newTestService(&mockNotionAPI{})

	resp, err := svc.GetCapabilities(context.Background(), &connectorpb.GetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	caps := resp.Capabilities
	if caps.SupportsThreads {
		t.Error("expected SupportsThreads=false")
	}
	if caps.SupportsReactions {
		t.Error("expected SupportsReactions=false")
	}
	if caps.SupportsEdit {
		t.Error("expected SupportsEdit=false")
	}
	if caps.SupportsReply {
		t.Error("expected SupportsReply=false")
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
	svc, _ := newTestService(&mockNotionAPI{})
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
	if resp.Accounts[0].AccountId != "workspace-test" {
		t.Errorf("AccountId = %q, want %q", resp.Accounts[0].AccountId, "workspace-test")
	}
}

func TestHealthCheck(t *testing.T) {
	svc, base := newTestService(&mockNotionAPI{})

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

func TestHandlePageEvent(t *testing.T) {
	api := &mockNotionAPI{}
	svc, _ := newTestService(api)
	svc.base.Publisher = &mockPublisher{}

	event := &notion.PageEvent{
		Page: notion.Page{
			ID:             "page-1",
			LastEditedByID: "user-1",
		},
		Text: "Some page content",
	}

	svc.HandlePageEvent(event)
	// If we get here without panic, the handler worked.
	// The mock publisher accepted the message.
}
