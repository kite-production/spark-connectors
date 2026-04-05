// Package normalizer converts Gmail API messages to InboundMessage protos.
package normalizer

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	"github.com/kite-production/spark/services/connector-gmail/internal/htmlstrip"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fromHeaderRe parses "Display Name <email@example.com>" format.
var fromHeaderRe = regexp.MustCompile(`^(.+?)\s*<([^>]+)>$`)

// Normalizer converts Gmail messages to InboundMessage protos.
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

// Normalize converts a Gmail message to an InboundMessage.
func (n *Normalizer) Normalize(msg *gmail.Message) *connectorpb.InboundMessage {
	from := getHeader(msg, "From")
	senderEmail, senderName := ParseFrom(from)

	text := extractPlainText(msg)

	return &connectorpb.InboundMessage{
		ConnectorId: n.connectorID,
		AccountId:   n.accountID,
		MessageId:   msg.Id,
		ThreadId:    msg.ThreadId,
		Text:        text,
		PeerKind:    commonpb.PeerKind_PEER_KIND_DM,
		PeerId:      senderEmail,
		Sender: &commonpb.SenderIdentity{
			SenderId:   senderEmail,
			SenderName: senderName,
		},
		IdempotencyKey: &commonpb.IdempotencyKey{
			Value: fmt.Sprintf("gmail:%s:%s", n.accountID, msg.Id),
		},
		ReceivedAt: timestamppb.Now(),
	}
}

// ParseFrom extracts the email and display name from a From header value.
// Handles formats like "Display Name <email@example.com>" and bare "email@example.com".
func ParseFrom(from string) (email, displayName string) {
	from = strings.TrimSpace(from)
	if from == "" {
		return "", ""
	}

	matches := fromHeaderRe.FindStringSubmatch(from)
	if matches != nil {
		name := strings.Trim(matches[1], `"' `)
		return matches[2], name
	}

	// Bare email address.
	return from, ""
}

// extractPlainText extracts plain text from a Gmail message.
// Prefers text/plain parts, falls back to stripped HTML.
func extractPlainText(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}

	// Try to find text/plain in the payload tree.
	if text := findPartText(msg.Payload, "text/plain"); text != "" {
		return text
	}

	// Fall back to stripped HTML.
	if text := findPartText(msg.Payload, "text/html"); text != "" {
		return htmlstrip.Strip(text)
	}

	return ""
}

// findPartText recursively searches MIME parts for the given type and decodes body.
func findPartText(part *gmail.MessagePart, mimeType string) string {
	if part.MimeType == mimeType && part.Body != nil && part.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(decoded))
	}

	for _, child := range part.Parts {
		if text := findPartText(child, mimeType); text != "" {
			return text
		}
	}
	return ""
}

// getHeader returns the value of the first matching header.
func getHeader(msg *gmail.Message, name string) string {
	if msg.Payload == nil {
		return ""
	}
	for _, h := range msg.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// GetSubject returns the Subject header from a Gmail message.
func GetSubject(msg *gmail.Message) string {
	return getHeader(msg, "Subject")
}
