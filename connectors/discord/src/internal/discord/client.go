// Package discord wraps the discordgo library for the Spark connector.
package discord

import (
	"context"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

// MessageHandler is the callback invoked for each incoming Discord message.
type MessageHandler func(s *discordgo.Session, m *discordgo.MessageCreate)

// Client wraps a discordgo.Session with the intents and lifecycle methods
// needed by the Spark Discord connector.
type Client struct {
	session *discordgo.Session
	token   string
}

// NewClient creates a Discord client with the given bot token. The session
// is configured with the required Gateway intents but not yet connected.
func NewClient(token string) (*Client, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}

	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	return &Client{
		session: session,
		token:   token,
	}, nil
}

// Open starts the WebSocket connection to the Discord Gateway and registers
// the message handler. It blocks until ctx is cancelled.
func (c *Client) Open(ctx context.Context, handler MessageHandler) error {
	c.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handler(s, m)
	})

	if err := c.session.Open(); err != nil {
		return fmt.Errorf("opening discord websocket: %w", err)
	}
	log.Println("discord gateway connection established")

	<-ctx.Done()
	return nil
}

// Close gracefully disconnects from the Discord Gateway.
func (c *Client) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// SendMessage sends a text message to the specified channel. If replyToID
// is non-empty, the message is sent as a reply to that message.
func (c *Client) SendMessage(channelID, text, replyToID string) (string, error) {
	msg := &discordgo.MessageSend{
		Content: text,
	}

	if replyToID != "" {
		msg.Reference = &discordgo.MessageReference{
			MessageID: replyToID,
		}
	}

	sent, err := c.session.ChannelMessageSendComplex(channelID, msg)
	if err != nil {
		return "", fmt.Errorf("sending message to channel %s: %w", channelID, err)
	}
	return sent.ID, nil
}

// Session returns the underlying discordgo session for direct API access.
func (c *Client) Session() *discordgo.Session {
	return c.session
}
