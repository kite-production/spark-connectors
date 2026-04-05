package connector

import (
	"fmt"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"google.golang.org/protobuf/proto"
)

// InboundSubject returns the NATS subject for inbound messages from
// the given connector ID.
func InboundSubject(connectorID string) string {
	return fmt.Sprintf("spark.connector.inbound.%s", connectorID)
}

// PublishInbound marshals an InboundMessage to protobuf and publishes it
// to the NATS subject spark.connector.inbound.{connector_id}. It waits
// for JetStream acknowledgment before returning.
func (b *BaseConnector) PublishInbound(msg *connectorpb.InboundMessage) error {
	if b.Publisher == nil {
		return fmt.Errorf("NATS publisher not initialized")
	}
	if msg == nil {
		return fmt.Errorf("inbound message is nil")
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling InboundMessage: %w", err)
	}

	subject := InboundSubject(msg.GetConnectorId())
	ack, err := b.Publisher.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}
	_ = ack // ACK received — publish confirmed by JetStream.
	return nil
}
