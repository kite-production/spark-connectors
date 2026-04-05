// Package normalizer converts Notion page events to InboundMessage protos.
package normalizer

import (
	"fmt"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/kite-production/spark/services/connector-notion/internal/notion"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Normalizer converts Notion page events to InboundMessage protos.
type Normalizer struct {
	connectorID string
	accountID   string
}

// New creates a Normalizer for the given connector and account.
func New(connectorID, accountID string) *Normalizer {
	return &Normalizer{
		connectorID: connectorID,
		accountID:   accountID,
	}
}

// Normalize converts a Notion PageEvent to an InboundMessage.
func (n *Normalizer) Normalize(event *notion.PageEvent) *connectorpb.InboundMessage {
	return &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		AccountId:   n.accountID,
		MessageId:   event.Page.ID,
		Text:        event.Text,
		PeerKind:    commonpb.PeerKind_PEER_KIND_CHANNEL,
		PeerId:      event.Page.ID,
		Sender: &commonpb.SenderIdentity{
			SenderId:   event.Page.LastEditedByID,
			SenderName: "",
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("notion:%s:%s",
				event.Page.ID,
				event.Page.LastEditedTime.Format("2006-01-02T15:04:05.000Z"),
			),
		},
		ReceivedAt: timestamppb.Now(),
	}
}
