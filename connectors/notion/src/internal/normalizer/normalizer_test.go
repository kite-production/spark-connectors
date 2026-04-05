package normalizer

import (
	"testing"
	"time"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	"github.com/kite-production/spark/services/connector-notion/internal/notion"
)

func TestNormalize(t *testing.T) {
	n := New("notion", "workspace-1")

	edited := time.Date(2026, 3, 20, 10, 30, 0, 0, time.UTC)
	event := &notion.PageEvent{
		Page: notion.Page{
			ID:             "page-abc",
			LastEditedTime: edited,
			LastEditedByID: "user-xyz",
			Title:          "Test Page",
		},
		Text: "Hello from Notion",
	}

	inbound := n.Normalize(event)

	if inbound.ConnectorId != "notion" {
		t.Errorf("ConnectorId = %q, want %q", inbound.ConnectorId, "notion")
	}
	if inbound.AccountId != "workspace-1" {
		t.Errorf("AccountId = %q, want %q", inbound.AccountId, "workspace-1")
	}
	if inbound.MessageId != "page-abc" {
		t.Errorf("MessageId = %q, want %q", inbound.MessageId, "page-abc")
	}
	if inbound.PeerKind != commonpb.PeerKind_PEER_KIND_CHANNEL {
		t.Errorf("PeerKind = %v, want CHANNEL", inbound.PeerKind)
	}
	if inbound.PeerId != "page-abc" {
		t.Errorf("PeerId = %q, want %q", inbound.PeerId, "page-abc")
	}
	if inbound.Sender.GetSenderId() != "user-xyz" {
		t.Errorf("SenderId = %q, want %q", inbound.Sender.GetSenderId(), "user-xyz")
	}
	if inbound.Text != "Hello from Notion" {
		t.Errorf("Text = %q, want %q", inbound.Text, "Hello from Notion")
	}

	wantKey := "notion:page-abc:2026-03-20T10:30:00.000Z"
	if inbound.IdempotencyKey.GetValue() != wantKey {
		t.Errorf("IdempotencyKey = %q, want %q", inbound.IdempotencyKey.GetValue(), wantKey)
	}
	if inbound.ReceivedAt == nil {
		t.Error("ReceivedAt should not be nil")
	}
}

func TestNormalize_EmptyText(t *testing.T) {
	n := New("notion", "ws")

	event := &notion.PageEvent{
		Page: notion.Page{
			ID:             "page-empty",
			LastEditedTime: time.Now(),
		},
		Text: "",
	}

	inbound := n.Normalize(event)
	if inbound.Text != "" {
		t.Errorf("Text = %q, want empty", inbound.Text)
	}
}
