// Package telegram provides a thin wrapper around the Telegram Bot API.
package telegram

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UpdateHandler is called for each incoming Telegram update.
type UpdateHandler func(update tgbotapi.Update)

// Client wraps the Telegram Bot API for the connector.
type Client struct {
	bot *tgbotapi.BotAPI
}

// NewClient creates a new Telegram bot client with the given token.
func NewClient(token string) (*Client, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("creating telegram bot: %w", err)
	}
	return &Client{bot: bot}, nil
}

// GetMe returns information about the bot for health checks.
func (c *Client) GetMe() (tgbotapi.User, error) {
	return c.bot.GetMe()
}

// BotUsername returns the bot's username.
func (c *Client) BotUsername() string {
	return c.bot.Self.UserName
}

// StartPolling runs a long-poll loop that delivers updates to the handler.
// It blocks until the context is cancelled.
func (c *Client) StartPolling(ctx context.Context, handler UpdateHandler) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := c.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			c.bot.StopReceivingUpdates()
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			handler(update)
		}
	}
}

// SendMessage sends a text message to a Telegram chat.
// If replyToID is non-zero, the message is sent as a reply.
func (c *Client) SendMessage(chatID int64, text string, replyToID int, parseMode string) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyToID != 0 {
		msg.ReplyToMessageID = replyToID
	}
	if parseMode != "" {
		msg.ParseMode = parseMode
	}
	sent, err := c.bot.Send(msg)
	if err != nil {
		return tgbotapi.Message{}, fmt.Errorf("sending telegram message: %w", err)
	}
	log.Printf("sent message %d to chat %d", sent.MessageID, chatID)
	return sent, nil
}
