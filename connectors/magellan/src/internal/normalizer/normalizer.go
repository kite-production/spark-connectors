// Package normalizer converts Venus messages to Spark InboundMessage protos.
package normalizer

import (
	"fmt"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/kite-production/spark/services/connector-magellan/internal/venus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Normalizer converts Venus messages to InboundMessage protos.
type Normalizer struct {
	connectorID string
}

// New creates a normalizer for the given connector ID.
func New(connectorID string) *Normalizer {
	return &Normalizer{connectorID: connectorID}
}

// Normalize converts a Venus message to an InboundMessage proto.
func (n *Normalizer) Normalize(msg *venus.Message) *connectorpb.InboundMessage {
	peerKind := commonpb.PeerKind_PEER_KIND_CHANNEL

	inbound := &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		AccountId:   "default",
		MessageId:   msg.ID,
		Text:        msg.Text,
		PeerKind:    peerKind,
		PeerId:      msg.ChannelID,
		GroupId:     "venus",
		Sender: &commonpb.SenderIdentity{
			SenderId:   msg.SenderID,
			SenderName: msg.SenderName,
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("magellan:venus:%s", msg.ID),
		},
		ReceivedAt: timestamppb.Now(),
	}

	if msg.ThreadID != "" {
		inbound.ThreadId = msg.ThreadID
	}
	if msg.ReplyToID != "" {
		inbound.ReplyToId = msg.ReplyToID
	}

	return inbound
}

// ShouldProcess filters out messages from the connector itself (bot echo prevention).
func ShouldProcess(msg *venus.Message) bool {
	// Skip messages sent by the connector (prevents echo loops).
	if msg.SenderID == "magellan" || msg.SenderID == "spark" {
		return false
	}
	// Skip empty messages.
	if msg.Text == "" {
		return false
	}
	return true
}
