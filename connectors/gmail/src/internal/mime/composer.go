// Package mime composes MIME-formatted emails for sending via the Gmail API.
package mime

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// ComposeEmail builds a raw RFC 2822 email and returns it as a
// URL-safe base64-encoded string suitable for the Gmail API send endpoint.
func ComposeEmail(to, subject, body string) string {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	return base64.URLEncoding.EncodeToString([]byte(msg.String()))
}

// ReplySubject returns the subject with a "Re: " prefix if it doesn't
// already have one.
func ReplySubject(subject string) string {
	if subject == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(subject), "re: ") {
		return subject
	}
	return "Re: " + subject
}
