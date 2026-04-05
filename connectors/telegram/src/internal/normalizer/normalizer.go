// Package normalizer converts Telegram updates to InboundMessage protos.
package normalizer

import (
	"fmt"
	"strings"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Normalizer converts Telegram messages to InboundMessage protos.
type Normalizer struct {
	connectorID string
}

// New creates a Normalizer.
func New(connectorID string) *Normalizer {
	return &Normalizer{
		connectorID: connectorID,
	}
}

// ShouldProcess returns true if this update should be processed as a message.
// Filters out bot messages, edited messages, and service messages.
func ShouldProcess(update tgbotapi.Update) bool {
	msg := update.Message
	if msg == nil {
		return false
	}
	// Skip bot messages.
	if msg.From != nil && msg.From.IsBot {
		return false
	}
	// Skip edited messages (those come through EditedMessage, not Message).
	// Skip service messages (new members, left member, etc.).
	if msg.NewChatMembers != nil || msg.LeftChatMember != nil {
		return false
	}
	if msg.GroupChatCreated || msg.SuperGroupChatCreated || msg.ChannelChatCreated {
		return false
	}
	// Skip if no text content.
	if msg.Text == "" && msg.Caption == "" {
		return false
	}
	return true
}

// Normalize converts a Telegram Message to an InboundMessage proto.
func (n *Normalizer) Normalize(msg *tgbotapi.Message) *connectorpb.InboundMessage {
	// Determine peer kind based on chat type.
	peerKind := commonpb.PeerKind_PEER_KIND_DM
	switch msg.Chat.Type {
	case "group", "supergroup", "channel":
		peerKind = commonpb.PeerKind_PEER_KIND_CHANNEL
	}

	// Build sender name from From fields.
	senderName := buildSenderName(msg.From)
	senderID := ""
	if msg.From != nil {
		senderID = fmt.Sprintf("%d", msg.From.ID)
	}

	// Use text or caption as the message body.
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}

	chatID := fmt.Sprintf("%d", msg.Chat.ID)

	inbound := &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		MessageId:   fmt.Sprintf("%d", msg.MessageID),
		Text:        text,
		PeerKind:    peerKind,
		PeerId:      chatID,
		Sender: &commonpb.SenderIdentity{
			SenderId:   senderID,
			SenderName: senderName,
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("telegram:%s:%d", chatID, msg.MessageID),
		},
		ReceivedAt: timestamppb.Now(),
	}

	// Set group_id for group chats.
	if peerKind == commonpb.PeerKind_PEER_KIND_CHANNEL {
		inbound.GroupId = chatID
	}

	// Set reply_to_id if replying to another message.
	if msg.ReplyToMessage != nil {
		inbound.ReplyToId = fmt.Sprintf("%d", msg.ReplyToMessage.MessageID)
	}

	return inbound
}

// buildSenderName constructs a display name from a Telegram User.
func buildSenderName(user *tgbotapi.User) string {
	if user == nil {
		return "unknown"
	}
	parts := []string{}
	if user.FirstName != "" {
		parts = append(parts, user.FirstName)
	}
	if user.LastName != "" {
		parts = append(parts, user.LastName)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if user.UserName != "" {
		return user.UserName
	}
	return fmt.Sprintf("user:%d", user.ID)
}
