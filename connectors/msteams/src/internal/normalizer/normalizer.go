// Package normalizer converts Microsoft Teams Bot Framework activities to
// Spark InboundMessage protos.
package normalizer

import (
	"fmt"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/kite-production/spark/services/connector-msteams/internal/msteams"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Normalizer converts Teams activities to InboundMessage protos.
type Normalizer struct {
	connectorID string
}

// New creates a normalizer for the given connector ID.
func New(connectorID string) *Normalizer {
	return &Normalizer{connectorID: connectorID}
}

// ShouldProcess determines whether a Bot Framework activity should be processed.
// Filters out non-message activities, bot-originated messages, and empty text.
func ShouldProcess(activity *msteams.Activity) bool {
	if activity == nil {
		return false
	}
	// Only process message activities.
	if activity.Type != "message" {
		return false
	}
	// Skip messages from bots (From.AADObjectID is empty for bots in some cases,
	// but the definitive check is whether From.Name indicates a bot or From.ID
	// matches the recipient bot ID — we check if From equals Recipient).
	if activity.From.ID == activity.Recipient.ID {
		return false
	}
	// Skip empty messages.
	if activity.Text == "" {
		return false
	}
	return true
}

// Normalize converts a Teams Bot Framework Activity to an InboundMessage proto.
func (n *Normalizer) Normalize(activity *msteams.Activity) *connectorpb.InboundMessage {
	// Determine peer kind: personal chats are DMs, everything else is a channel.
	peerKind := commonpb.PeerKind_PEER_KIND_CHANNEL
	if activity.Conversation.ConversationType == "personal" {
		peerKind = commonpb.PeerKind_PEER_KIND_DM
	}

	// Determine group ID from team data if available.
	groupID := ""
	if activity.ChannelData != nil && activity.ChannelData.Team != nil {
		groupID = activity.ChannelData.Team.ID
	}

	msg := &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		AccountId:   "default",
		MessageId:   activity.ID,
		Text:        activity.Text,
		PeerKind:    peerKind,
		PeerId:      activity.Conversation.ID,
		GroupId:     groupID,
		Sender: &commonpb.SenderIdentity{
			SenderId:   activity.From.ID,
			SenderName: activity.From.Name,
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("msteams:%s:%s", activity.Conversation.ID, activity.ID),
		},
		ReceivedAt: timestamppb.Now(),
	}

	// Set thread context if this is a reply.
	if activity.ReplyToID != "" {
		msg.ThreadId = activity.ReplyToID
	}

	return msg
}
