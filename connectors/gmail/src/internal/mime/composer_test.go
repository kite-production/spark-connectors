package mime

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestComposeEmail(t *testing.T) {
	raw := ComposeEmail("bob@example.com", "Hello", "Hi Bob!")

	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	email := string(decoded)

	if !strings.Contains(email, "To: bob@example.com\r\n") {
		t.Error("missing or incorrect To header")
	}
	if !strings.Contains(email, "Subject: Hello\r\n") {
		t.Error("missing or incorrect Subject header")
	}
	if !strings.Contains(email, "MIME-Version: 1.0\r\n") {
		t.Error("missing MIME-Version header")
	}
	if !strings.Contains(email, "Content-Type: text/plain; charset=\"UTF-8\"\r\n") {
		t.Error("missing Content-Type header")
	}
	if !strings.Contains(email, "\r\n\r\nHi Bob!") {
		t.Error("missing body or incorrect header/body separator")
	}
}

func TestReplySubject(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		want    string
	}{
		{
			name:    "adds Re prefix",
			subject: "Hello",
			want:    "Re: Hello",
		},
		{
			name:    "already has Re prefix",
			subject: "Re: Hello",
			want:    "Re: Hello",
		},
		{
			name:    "case insensitive Re check",
			subject: "RE: Hello",
			want:    "RE: Hello",
		},
		{
			name:    "re lowercase",
			subject: "re: Hello",
			want:    "re: Hello",
		},
		{
			name:    "empty subject",
			subject: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReplySubject(tt.subject)
			if got != tt.want {
				t.Errorf("ReplySubject(%q) = %q, want %q", tt.subject, got, tt.want)
			}
		})
	}
}
