// Package normalizer converts Slack events to InboundMessage protos.
package normalizer

import (
	"fmt"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/slack-go/slack/slackevents"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DisplayNameResolver looks up a user's display name from their ID.
type DisplayNameResolver interface {
	GetDisplayName(userID string) string
}

// Normalizer converts Slack message events to InboundMessage protos.
type Normalizer struct {
	connectorID string
	resolver    DisplayNameResolver
}

// New creates a Normalizer.
func New(connectorID string, resolver DisplayNameResolver) *Normalizer {
	return &Normalizer{
		connectorID: connectorID,
		resolver:    resolver,
	}
}

// ShouldProcess returns true if this event should be processed.
// Filters out bot messages, edits, deletes, and reactions.
func ShouldProcess(ev *slackevents.MessageEvent) bool {
	if ev == nil {
		return false
	}
	// Skip bot messages.
	if ev.BotID != "" {
		return false
	}
	// Skip subtypes we don't handle.
	switch ev.SubType {
	case "message_changed", "message_deleted",
		"bot_message", "channel_join", "channel_leave":
		return false
	}
	// Skip if no text.
	if ev.Text == "" {
		return false
	}
	return true
}

// Normalize converts a Slack MessageEvent to an InboundMessage.
func (n *Normalizer) Normalize(ev *slackevents.MessageEvent, teamID string) *connectorpb.InboundMessage {
	peerKind := commonpb.PeerKind_PEER_KIND_CHANNEL
	if ev.ChannelType == "im" {
		peerKind = commonpb.PeerKind_PEER_KIND_DM
	}

	displayName := n.resolver.GetDisplayName(ev.User)

	msg := &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		MessageId:   ev.TimeStamp,
		Text:        ev.Text,
		PeerKind:    peerKind,
		PeerId:      ev.Channel,
		GroupId:     teamID,
		Sender: &commonpb.SenderIdentity{
			SenderId:   ev.User,
			SenderName: displayName,
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("slack:%s:%s", teamID, ev.TimeStamp),
		},
		ReceivedAt: timestamppb.Now(),
	}

	// Set thread_id if the message is threaded.
	if ev.ThreadTimeStamp != "" && ev.ThreadTimeStamp != ev.TimeStamp {
		msg.ThreadId = ev.ThreadTimeStamp
	}

	return msg
}
