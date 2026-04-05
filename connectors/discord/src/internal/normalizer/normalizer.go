// Package normalizer converts Discord message events to InboundMessage protos.
package normalizer

import (
	"fmt"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/bwmarrin/discordgo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Normalizer converts Discord MessageCreate events to InboundMessage protos.
type Normalizer struct {
	connectorID string
}

// New creates a Normalizer.
func New(connectorID string) *Normalizer {
	return &Normalizer{
		connectorID: connectorID,
	}
}

// ShouldProcess returns true if the message should be forwarded to the platform.
// It filters out bot messages and empty messages.
func ShouldProcess(m *discordgo.MessageCreate) bool {
	if m == nil || m.Message == nil {
		return false
	}
	// Skip bot messages.
	if m.Author != nil && m.Author.Bot {
		return false
	}
	// Skip empty messages.
	if m.Content == "" {
		return false
	}
	return true
}

// Normalize converts a Discord MessageCreate event to an InboundMessage.
func (n *Normalizer) Normalize(m *discordgo.MessageCreate) *connectorpb.InboundMessage {
	peerKind := commonpb.PeerKind_PEER_KIND_CHANNEL
	if m.GuildID == "" {
		// No guild means this is a DM.
		peerKind = commonpb.PeerKind_PEER_KIND_DM
	}

	senderID := ""
	senderName := ""
	if m.Author != nil {
		senderID = m.Author.ID
		senderName = m.Author.Username
	}

	msg := &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		MessageId:   m.ID,
		Text:        m.Content,
		PeerKind:    peerKind,
		PeerId:      m.ChannelID,
		GroupId:     m.GuildID,
		Sender: &commonpb.SenderIdentity{
			SenderId:   senderID,
			SenderName: senderName,
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("discord:%s:%s", m.GuildID, m.ID),
		},
		ReceivedAt: timestamppb.Now(),
	}

	// Set thread_id if the message is in a thread (Discord thread channels).
	// In Discord, threads have a non-empty MessageReference or the channel
	// itself is a thread type. We use the message reference's ChannelID if
	// present, otherwise check if GuildID is set and the channel is a thread.
	if m.MessageReference != nil && m.MessageReference.ChannelID != "" && m.MessageReference.ChannelID != m.ChannelID {
		msg.ThreadId = m.ChannelID
	}

	return msg
}
