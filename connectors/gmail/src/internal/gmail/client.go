// Package gmail wraps the Gmail API for polling and sending emails.
package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
	gm "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailAPI abstracts Gmail API operations for testability.
type GmailAPI interface {
	ListMessages(ctx context.Context, userID, query string) ([]*gm.Message, error)
	GetMessage(ctx context.Context, userID, messageID string) (*gm.Message, error)
	SendMessage(ctx context.Context, userID string, msg *gm.Message) (*gm.Message, error)
}

// Client implements GmailAPI using the real Gmail API SDK.
type Client struct {
	service *gm.Service
}

// NewClient creates a Gmail API client authenticated with service account credentials.
func NewClient(ctx context.Context, credentialsJSON string) (*Client, error) {
	creds, err := google.CredentialsFromJSON(ctx, []byte(credentialsJSON),
		gm.GmailReadonlyScope,
		gm.GmailSendScope,
	)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	// For domain-wide delegation, extract the subject from credentials config
	// and recreate with impersonation.
	var credConfig struct {
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON), &credConfig); err == nil && credConfig.Subject != "" {
		conf, err := google.JWTConfigFromJSON([]byte(credentialsJSON),
			gm.GmailReadonlyScope,
			gm.GmailSendScope,
		)
		if err != nil {
			return nil, fmt.Errorf("JWT config from credentials: %w", err)
		}
		conf.Subject = credConfig.Subject
		svc, err := gm.NewService(ctx, option.WithHTTPClient(conf.Client(ctx)))
		if err != nil {
			return nil, fmt.Errorf("creating Gmail service: %w", err)
		}
		return &Client{service: svc}, nil
	}

	svc, err := gm.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("creating Gmail service: %w", err)
	}
	return &Client{service: svc}, nil
}

// ListMessages lists messages matching the query.
func (c *Client) ListMessages(ctx context.Context, userID, query string) ([]*gm.Message, error) {
	resp, err := c.service.Users.Messages.List(userID).Q(query).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	return resp.Messages, nil
}

// GetMessage retrieves a full message by ID.
func (c *Client) GetMessage(ctx context.Context, userID, messageID string) (*gm.Message, error) {
	msg, err := c.service.Users.Messages.Get(userID, messageID).Format("full").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("getting message %s: %w", messageID, err)
	}
	return msg, nil
}

// SendMessage sends an email via the Gmail API.
func (c *Client) SendMessage(ctx context.Context, userID string, msg *gm.Message) (*gm.Message, error) {
	sent, err := c.service.Users.Messages.Send(userID, msg).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("sending message: %w", err)
	}
	return sent, nil
}

// Poller polls the Gmail API for new messages at a configured interval.
type Poller struct {
	api       GmailAPI
	userID    string
	interval  time.Duration
	onMessage func(msg *gm.Message)

	mu            sync.Mutex
	lastHistoryID uint64
	lastCheckTime int64
}

// NewPoller creates a Poller that checks for new messages.
func NewPoller(api GmailAPI, userID string, interval time.Duration, handler func(msg *gm.Message)) *Poller {
	return &Poller{
		api:       api,
		userID:    userID,
		interval:  interval,
		onMessage: handler,
	}
}

// Run starts the polling loop. It blocks until the context is cancelled.
func (p *Poller) Run(ctx context.Context) error {
	// Do an initial poll immediately.
	p.poll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// LastHistoryID returns the last processed history ID.
func (p *Poller) LastHistoryID() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastHistoryID
}

func (p *Poller) poll(ctx context.Context) {
	p.mu.Lock()
	lastCheck := p.lastCheckTime
	p.mu.Unlock()

	now := time.Now().Unix()
	if lastCheck == 0 {
		// First poll: look back 1 minute.
		lastCheck = now - 60
	}

	query := fmt.Sprintf("is:unread after:%d", lastCheck)
	messages, err := p.api.ListMessages(ctx, p.userID, query)
	if err != nil {
		return
	}

	for _, stub := range messages {
		msg, err := p.api.GetMessage(ctx, p.userID, stub.Id)
		if err != nil {
			continue
		}

		// Track historyId.
		if msg.HistoryId > 0 {
			p.mu.Lock()
			hid := msg.HistoryId
			if hid > p.lastHistoryID {
				p.lastHistoryID = hid
			}
			p.mu.Unlock()
		}

		p.onMessage(msg)
	}

	p.mu.Lock()
	p.lastCheckTime = now
	p.mu.Unlock()
}

// ParseHistoryID converts a string history ID to uint64.
func ParseHistoryID(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}
