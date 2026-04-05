// Package chatapi wraps the Google Chat API for sending messages.
package chatapi

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
	chat "google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

// ChatAPI abstracts Google Chat API operations for testability.
type ChatAPI interface {
	CreateMessage(ctx context.Context, spaceName, text, threadName string) (*chat.Message, error)
}

// Client implements ChatAPI using the real Google Chat API.
type Client struct {
	service *chat.Service
}

// NewClient creates a Google Chat API client authenticated with service account credentials.
func NewClient(ctx context.Context, credentialsJSON string) (*Client, error) {
	creds, err := google.CredentialsFromJSON(ctx, []byte(credentialsJSON),
		"https://www.googleapis.com/auth/chat.bot",
	)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	svc, err := chat.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("creating Chat service: %w", err)
	}
	return &Client{service: svc}, nil
}

// CreateMessage sends a text message to a Google Chat space.
// If threadName is non-empty, the message is sent as a threaded reply.
func (c *Client) CreateMessage(ctx context.Context, spaceName, text, threadName string) (*chat.Message, error) {
	msg := &chat.Message{
		Text: text,
	}
	if threadName != "" {
		msg.Thread = &chat.Thread{
			Name: threadName,
		}
	}

	call := c.service.Spaces.Messages.Create(spaceName, msg)
	if threadName != "" {
		call = call.MessageReplyOption("REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD")
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("creating message in %s: %w", spaceName, err)
	}
	return resp, nil
}
