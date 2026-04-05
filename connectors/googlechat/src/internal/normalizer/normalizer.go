// Package normalizer converts Google Chat webhook events to InboundMessage protos.
package normalizer

import (
	"fmt"
	"strings"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ChatEvent represents a Google Chat webhook event payload.
type ChatEvent struct {
	Type    string       `json:"type"`
	Message *ChatMessage `json:"message"`
	Space   *ChatSpace   `json:"space"`
	User    *ChatUser    `json:"user"`
}

// ChatMessage represents a message in a Google Chat event.
type ChatMessage struct {
	Name   string      `json:"name"`
	Text   string      `json:"text"`
	Thread *ChatThread `json:"thread"`
}

// ChatThread represents a thread in a Google Chat message.
type ChatThread struct {
	Name string `json:"name"`
}

// ChatSpace represents a Google Chat space.
type ChatSpace struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ChatUser represents a Google Chat user.
type ChatUser struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// Normalizer converts Google Chat events to InboundMessage protos.
type Normalizer struct {
	connectorID string
}

// New creates a Normalizer.
func New(connectorID string) *Normalizer {
	return &Normalizer{connectorID: connectorID}
}

// ShouldProcess returns true if the event should be processed.
func ShouldProcess(ev *ChatEvent) bool {
	if ev == nil {
		return false
	}
	if ev.Type != "MESSAGE" {
		return false
	}
	if ev.Message == nil || ev.Message.Text == "" {
		return false
	}
	if ev.Space == nil {
		return false
	}
	return true
}

// Normalize converts a Google Chat event to an InboundMessage.
func (n *Normalizer) Normalize(ev *ChatEvent) *connectorpb.InboundMessage {
	spaceName := ev.Space.Name

	peerKind := commonpb.PeerKind_PEER_KIND_CHANNEL
	if ev.Space.Type == "DM" {
		peerKind = commonpb.PeerKind_PEER_KIND_DM
	}

	var senderID, senderName string
	if ev.User != nil {
		senderID = ev.User.Name
		senderName = ev.User.DisplayName
	}

	messageName := ev.Message.Name

	msg := &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		MessageId:   messageName,
		Text:        ev.Message.Text,
		PeerKind:    peerKind,
		PeerId:      spaceName,
		GroupId:     spaceName,
		Sender: &commonpb.SenderIdentity{
			SenderId:   senderID,
			SenderName: senderName,
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("googlechat:%s:%s", extractID(spaceName), extractID(messageName)),
		},
		ReceivedAt: timestamppb.Now(),
	}

	// Set thread_id if the message is threaded.
	if ev.Message.Thread != nil && ev.Message.Thread.Name != "" {
		msg.ThreadId = ev.Message.Thread.Name
	}

	return msg
}

// extractID extracts the trailing ID from a resource name like "spaces/ABC123".
func extractID(resourceName string) string {
	if idx := strings.LastIndex(resourceName, "/"); idx >= 0 && idx < len(resourceName)-1 {
		return resourceName[idx+1:]
	}
	return resourceName
}
