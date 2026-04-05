package normalizer

import (
	"encoding/base64"
	"testing"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	"google.golang.org/api/gmail/v1"
)

func TestParseFrom(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		wantEmail   string
		wantDisplay string
	}{
		{
			name:        "name and email",
			from:        "Alice Smith <alice@example.com>",
			wantEmail:   "alice@example.com",
			wantDisplay: "Alice Smith",
		},
		{
			name:        "quoted name",
			from:        `"Bob Jones" <bob@example.com>`,
			wantEmail:   "bob@example.com",
			wantDisplay: "Bob Jones",
		},
		{
			name:        "bare email",
			from:        "charlie@example.com",
			wantEmail:   "charlie@example.com",
			wantDisplay: "",
		},
		{
			name:        "empty",
			from:        "",
			wantEmail:   "",
			wantDisplay: "",
		},
		{
			name:        "whitespace only",
			from:        "   ",
			wantEmail:   "",
			wantDisplay: "",
		},
		{
			name:        "name with special chars",
			from:        "O'Brien, Diane <diane@example.com>",
			wantEmail:   "diane@example.com",
			wantDisplay: "O'Brien, Diane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, display := ParseFrom(tt.from)
			if email != tt.wantEmail {
				t.Errorf("email = %q, want %q", email, tt.wantEmail)
			}
			if display != tt.wantDisplay {
				t.Errorf("displayName = %q, want %q", display, tt.wantDisplay)
			}
		})
	}
}

func b64(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

func TestNormalize(t *testing.T) {
	n := New("gmail", "user@example.com")

	msg := &gmail.Message{
		Id:       "msg123",
		ThreadId: "thread456",
		Payload: &gmail.MessagePart{
			MimeType: "multipart/alternative",
			Headers: []*gmail.MessagePartHeader{
				{Name: "From", Value: "Alice <alice@example.com>"},
				{Name: "Subject", Value: "Hello"},
			},
			Parts: []*gmail.MessagePart{
				{
					MimeType: "text/plain",
					Body:     &gmail.MessagePartBody{Data: b64("Hello, world!")},
				},
				{
					MimeType: "text/html",
					Body:     &gmail.MessagePartBody{Data: b64("<p>Hello, world!</p>")},
				},
			},
		},
	}

	inbound := n.Normalize(msg)

	if inbound.ConnectorId != "gmail" {
		t.Errorf("ConnectorId = %q, want %q", inbound.ConnectorId, "gmail")
	}
	if inbound.AccountId != "user@example.com" {
		t.Errorf("AccountId = %q, want %q", inbound.AccountId, "user@example.com")
	}
	if inbound.MessageId != "msg123" {
		t.Errorf("MessageId = %q, want %q", inbound.MessageId, "msg123")
	}
	if inbound.ThreadId != "thread456" {
		t.Errorf("ThreadId = %q, want %q", inbound.ThreadId, "thread456")
	}
	if inbound.PeerKind != commonpb.PeerKind_PEER_KIND_DM {
		t.Errorf("PeerKind = %v, want DM", inbound.PeerKind)
	}
	if inbound.PeerId != "alice@example.com" {
		t.Errorf("PeerId = %q, want %q", inbound.PeerId, "alice@example.com")
	}
	if inbound.Sender.GetSenderId() != "alice@example.com" {
		t.Errorf("SenderId = %q, want %q", inbound.Sender.GetSenderId(), "alice@example.com")
	}
	if inbound.Sender.GetSenderName() != "Alice" {
		t.Errorf("SenderName = %q, want %q", inbound.Sender.GetSenderName(), "Alice")
	}
	if inbound.Text != "Hello, world!" {
		t.Errorf("Text = %q, want %q", inbound.Text, "Hello, world!")
	}
	if inbound.IdempotencyKey.GetValue() != "gmail:user@example.com:msg123" {
		t.Errorf("IdempotencyKey = %q, want %q", inbound.IdempotencyKey.GetValue(), "gmail:user@example.com:msg123")
	}
	if inbound.ReceivedAt == nil {
		t.Error("ReceivedAt should not be nil")
	}
}

func TestNormalize_HTMLFallback(t *testing.T) {
	n := New("gmail", "user@example.com")

	msg := &gmail.Message{
		Id:       "msg456",
		ThreadId: "thread789",
		Payload: &gmail.MessagePart{
			MimeType: "text/html",
			Headers: []*gmail.MessagePartHeader{
				{Name: "From", Value: "bob@example.com"},
			},
			Body: &gmail.MessagePartBody{Data: b64("<p>HTML only</p>")},
		},
	}

	inbound := n.Normalize(msg)

	if inbound.Text != "HTML only" {
		t.Errorf("Text = %q, want %q (HTML should be stripped)", inbound.Text, "HTML only")
	}
	if inbound.Sender.GetSenderName() != "" {
		t.Errorf("SenderName = %q, want empty for bare email", inbound.Sender.GetSenderName())
	}
}

func TestNormalize_NilPayload(t *testing.T) {
	n := New("gmail", "user@example.com")

	msg := &gmail.Message{
		Id:       "msg789",
		ThreadId: "thread000",
		Payload:  nil,
	}

	inbound := n.Normalize(msg)
	if inbound.Text != "" {
		t.Errorf("Text = %q, want empty for nil payload", inbound.Text)
	}
}

func TestGetSubject(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Test Subject"},
				{Name: "From", Value: "test@example.com"},
			},
		},
	}

	subject := GetSubject(msg)
	if subject != "Test Subject" {
		t.Errorf("GetSubject() = %q, want %q", subject, "Test Subject")
	}
}

func TestGetSubject_NilPayload(t *testing.T) {
	msg := &gmail.Message{Payload: nil}
	if got := GetSubject(msg); got != "" {
		t.Errorf("GetSubject() = %q, want empty", got)
	}
}
