package normalizer

import (
	"testing"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	"github.com/slack-go/slack/slackevents"
)

// mockResolver implements DisplayNameResolver for tests.
type mockResolver struct {
	names map[string]string
}

func (m *mockResolver) GetDisplayName(userID string) string {
	if name, ok := m.names[userID]; ok {
		return name
	}
	return userID
}

func TestShouldProcess(t *testing.T) {
	tests := []struct {
		name string
		ev   *slackevents.MessageEvent
		want bool
	}{
		{
			name: "normal message",
			ev:   &slackevents.MessageEvent{Text: "hello", User: "U123"},
			want: true,
		},
		{
			name: "nil event",
			ev:   nil,
			want: false,
		},
		{
			name: "bot message by bot_id",
			ev:   &slackevents.MessageEvent{Text: "hi", BotID: "B123"},
			want: false,
		},
		{
			name: "message_changed subtype",
			ev:   &slackevents.MessageEvent{Text: "edited", SubType: "message_changed"},
			want: false,
		},
		{
			name: "message_deleted subtype",
			ev:   &slackevents.MessageEvent{Text: "deleted", SubType: "message_deleted"},
			want: false,
		},
		{
			name: "bot_message subtype",
			ev:   &slackevents.MessageEvent{Text: "bot", SubType: "bot_message"},
			want: false,
		},
		{
			name: "channel_join subtype",
			ev:   &slackevents.MessageEvent{Text: "joined", SubType: "channel_join"},
			want: false,
		},
		{
			name: "channel_leave subtype",
			ev:   &slackevents.MessageEvent{Text: "left", SubType: "channel_leave"},
			want: false,
		},
		{
			name: "empty text",
			ev:   &slackevents.MessageEvent{Text: "", User: "U123"},
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
	resolver := &mockResolver{names: map[string]string{
		"U123": "Alice",
	}}
	n := New("slack", resolver)

	tests := []struct {
		name       string
		ev         *slackevents.MessageEvent
		teamID     string
		wantPeer   commonpb.PeerKind
		wantThread string
		wantKey    string
	}{
		{
			name: "channel message",
			ev: &slackevents.MessageEvent{
				User:            "U123",
				Text:            "hello",
				Channel:         "C456",
				ChannelType:     "channel",
				TimeStamp:       "1234567890.123456",
				ThreadTimeStamp: "",
			},
			teamID:     "T789",
			wantPeer:   commonpb.PeerKind_PEER_KIND_CHANNEL,
			wantThread: "",
			wantKey:    "slack:T789:1234567890.123456",
		},
		{
			name: "DM message",
			ev: &slackevents.MessageEvent{
				User:        "U123",
				Text:        "hi",
				Channel:     "D456",
				ChannelType: "im",
				TimeStamp:   "1234567890.654321",
			},
			teamID:     "T789",
			wantPeer:   commonpb.PeerKind_PEER_KIND_DM,
			wantThread: "",
			wantKey:    "slack:T789:1234567890.654321",
		},
		{
			name: "threaded message",
			ev: &slackevents.MessageEvent{
				User:            "U123",
				Text:            "reply",
				Channel:         "C456",
				ChannelType:     "channel",
				TimeStamp:       "1234567890.999999",
				ThreadTimeStamp: "1234567890.111111",
			},
			teamID:     "T789",
			wantPeer:   commonpb.PeerKind_PEER_KIND_CHANNEL,
			wantThread: "1234567890.111111",
			wantKey:    "slack:T789:1234567890.999999",
		},
		{
			name: "thread parent message not treated as threaded",
			ev: &slackevents.MessageEvent{
				User:            "U123",
				Text:            "parent",
				Channel:         "C456",
				ChannelType:     "channel",
				TimeStamp:       "1234567890.111111",
				ThreadTimeStamp: "1234567890.111111",
			},
			teamID:     "T789",
			wantPeer:   commonpb.PeerKind_PEER_KIND_CHANNEL,
			wantThread: "",
			wantKey:    "slack:T789:1234567890.111111",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := n.Normalize(tt.ev, tt.teamID)

			if msg.ConnectorId != "slack" {
				t.Errorf("ConnectorId = %q, want %q", msg.ConnectorId, "slack")
			}
			if msg.PeerKind != tt.wantPeer {
				t.Errorf("PeerKind = %v, want %v", msg.PeerKind, tt.wantPeer)
			}
			if msg.PeerId != tt.ev.Channel {
				t.Errorf("PeerId = %q, want %q", msg.PeerId, tt.ev.Channel)
			}
			if msg.GroupId != tt.teamID {
				t.Errorf("GroupId = %q, want %q", msg.GroupId, tt.teamID)
			}
			if msg.ThreadId != tt.wantThread {
				t.Errorf("ThreadId = %q, want %q", msg.ThreadId, tt.wantThread)
			}
			if msg.IdempotencyKey.GetValue() != tt.wantKey {
				t.Errorf("IdempotencyKey = %q, want %q", msg.IdempotencyKey.GetValue(), tt.wantKey)
			}
			if msg.Sender.GetSenderId() != tt.ev.User {
				t.Errorf("SenderId = %q, want %q", msg.Sender.GetSenderId(), tt.ev.User)
			}
			if msg.Text != tt.ev.Text {
				t.Errorf("Text = %q, want %q", msg.Text, tt.ev.Text)
			}
			if msg.ReceivedAt == nil {
				t.Error("ReceivedAt should not be nil")
			}
		})
	}
}

func TestNormalize_DisplayName(t *testing.T) {
	resolver := &mockResolver{names: map[string]string{
		"U123": "Alice",
	}}
	n := New("slack", resolver)

	ev := &slackevents.MessageEvent{
		User:      "U123",
		Text:      "hello",
		Channel:   "C456",
		TimeStamp: "1234567890.123456",
	}

	msg := n.Normalize(ev, "T789")
	if msg.Sender.GetSenderName() != "Alice" {
		t.Errorf("SenderName = %q, want %q", msg.Sender.GetSenderName(), "Alice")
	}
}
