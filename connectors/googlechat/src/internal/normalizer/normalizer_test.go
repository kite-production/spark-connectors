package normalizer

import (
	"testing"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
)

func TestShouldProcess(t *testing.T) {
	tests := []struct {
		name string
		ev   *ChatEvent
		want bool
	}{
		{
			name: "valid MESSAGE event",
			ev: &ChatEvent{
				Type:    "MESSAGE",
				Message: &ChatMessage{Name: "spaces/abc/messages/123", Text: "hello"},
				Space:   &ChatSpace{Name: "spaces/abc", Type: "ROOM"},
			},
			want: true,
		},
		{
			name: "nil event",
			ev:   nil,
			want: false,
		},
		{
			name: "non-MESSAGE type",
			ev: &ChatEvent{
				Type:    "ADDED_TO_SPACE",
				Message: &ChatMessage{Text: "hi"},
				Space:   &ChatSpace{Name: "spaces/abc"},
			},
			want: false,
		},
		{
			name: "nil message",
			ev: &ChatEvent{
				Type:  "MESSAGE",
				Space: &ChatSpace{Name: "spaces/abc"},
			},
			want: false,
		},
		{
			name: "empty text",
			ev: &ChatEvent{
				Type:    "MESSAGE",
				Message: &ChatMessage{Name: "spaces/abc/messages/123", Text: ""},
				Space:   &ChatSpace{Name: "spaces/abc"},
			},
			want: false,
		},
		{
			name: "nil space",
			ev: &ChatEvent{
				Type:    "MESSAGE",
				Message: &ChatMessage{Name: "spaces/abc/messages/123", Text: "hello"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldProcess(tt.ev); got != tt.want {
				t.Errorf("ShouldProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	n := New("googlechat")

	tests := []struct {
		name       string
		ev         *ChatEvent
		wantPeer   commonpb.PeerKind
		wantThread string
		wantKey    string
		wantPeerID string
	}{
		{
			name: "channel message in ROOM space",
			ev: &ChatEvent{
				Type: "MESSAGE",
				Message: &ChatMessage{
					Name: "spaces/SPACE1/messages/MSG1",
					Text: "hello",
				},
				Space: &ChatSpace{Name: "spaces/SPACE1", Type: "ROOM"},
				User:  &ChatUser{Name: "users/USER1", DisplayName: "Alice"},
			},
			wantPeer:   commonpb.PeerKind_PEER_KIND_CHANNEL,
			wantThread: "",
			wantKey:    "googlechat:SPACE1:MSG1",
			wantPeerID: "spaces/SPACE1",
		},
		{
			name: "DM message",
			ev: &ChatEvent{
				Type: "MESSAGE",
				Message: &ChatMessage{
					Name: "spaces/DM_SPACE/messages/MSG2",
					Text: "hi",
				},
				Space: &ChatSpace{Name: "spaces/DM_SPACE", Type: "DM"},
				User:  &ChatUser{Name: "users/USER2", DisplayName: "Bob"},
			},
			wantPeer:   commonpb.PeerKind_PEER_KIND_DM,
			wantThread: "",
			wantKey:    "googlechat:DM_SPACE:MSG2",
			wantPeerID: "spaces/DM_SPACE",
		},
		{
			name: "threaded message",
			ev: &ChatEvent{
				Type: "MESSAGE",
				Message: &ChatMessage{
					Name:   "spaces/SPACE1/messages/MSG3",
					Text:   "reply",
					Thread: &ChatThread{Name: "spaces/SPACE1/threads/THREAD1"},
				},
				Space: &ChatSpace{Name: "spaces/SPACE1", Type: "ROOM"},
				User:  &ChatUser{Name: "users/USER1", DisplayName: "Alice"},
			},
			wantPeer:   commonpb.PeerKind_PEER_KIND_CHANNEL,
			wantThread: "spaces/SPACE1/threads/THREAD1",
			wantKey:    "googlechat:SPACE1:MSG3",
			wantPeerID: "spaces/SPACE1",
		},
		{
			name: "message with nil user",
			ev: &ChatEvent{
				Type: "MESSAGE",
				Message: &ChatMessage{
					Name: "spaces/SPACE2/messages/MSG4",
					Text: "anonymous",
				},
				Space: &ChatSpace{Name: "spaces/SPACE2", Type: "ROOM"},
			},
			wantPeer:   commonpb.PeerKind_PEER_KIND_CHANNEL,
			wantThread: "",
			wantKey:    "googlechat:SPACE2:MSG4",
			wantPeerID: "spaces/SPACE2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := n.Normalize(tt.ev)

			if msg.ConnectorId != "googlechat" {
				t.Errorf("ConnectorId = %q, want %q", msg.ConnectorId, "googlechat")
			}
			if msg.PeerKind != tt.wantPeer {
				t.Errorf("PeerKind = %v, want %v", msg.PeerKind, tt.wantPeer)
			}
			if msg.PeerId != tt.wantPeerID {
				t.Errorf("PeerId = %q, want %q", msg.PeerId, tt.wantPeerID)
			}
			if msg.GroupId != tt.ev.Space.Name {
				t.Errorf("GroupId = %q, want %q", msg.GroupId, tt.ev.Space.Name)
			}
			if msg.ThreadId != tt.wantThread {
				t.Errorf("ThreadId = %q, want %q", msg.ThreadId, tt.wantThread)
			}
			if msg.IdempotencyKey.GetValue() != tt.wantKey {
				t.Errorf("IdempotencyKey = %q, want %q", msg.IdempotencyKey.GetValue(), tt.wantKey)
			}
			if msg.Text != tt.ev.Message.Text {
				t.Errorf("Text = %q, want %q", msg.Text, tt.ev.Message.Text)
			}
			if msg.ReceivedAt == nil {
				t.Error("ReceivedAt should not be nil")
			}
		})
	}
}

func TestNormalize_SenderFields(t *testing.T) {
	n := New("googlechat")
	ev := &ChatEvent{
		Type:    "MESSAGE",
		Message: &ChatMessage{Name: "spaces/S/messages/M", Text: "test"},
		Space:   &ChatSpace{Name: "spaces/S", Type: "ROOM"},
		User:    &ChatUser{Name: "users/U123", DisplayName: "Alice"},
	}

	msg := n.Normalize(ev)
	if msg.Sender.GetSenderId() != "users/U123" {
		t.Errorf("SenderId = %q, want %q", msg.Sender.GetSenderId(), "users/U123")
	}
	if msg.Sender.GetSenderName() != "Alice" {
		t.Errorf("SenderName = %q, want %q", msg.Sender.GetSenderName(), "Alice")
	}
}

func TestExtractID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"spaces/ABC123", "ABC123"},
		{"spaces/S/messages/M1", "M1"},
		{"noSlash", "noSlash"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := extractID(tt.input); got != tt.want {
				t.Errorf("extractID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
