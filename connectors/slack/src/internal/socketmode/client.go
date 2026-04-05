// Package socketmode wraps the Slack Socket Mode client for event listening.
package socketmode

import (
	"context"
	"log"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// EventHandler is called for each incoming message event.
type EventHandler func(ev *slackevents.MessageEvent, teamID string)

// Client wraps the Slack Socket Mode client.
type Client struct {
	api    *slack.Client
	sm     *socketmode.Client
	teamID string
}

// New creates a Socket Mode client with the given tokens.
func New(appToken, botToken string) *Client {
	api := slack.New(botToken,
		slack.OptionAppLevelToken(appToken),
	)
	sm := socketmode.New(api)
	return &Client{
		api: api,
		sm:  sm,
	}
}

// API returns the underlying Slack API client for making API calls.
func (c *Client) API() *slack.Client {
	return c.api
}

// Connected returns whether the WebSocket connection is active.
func (c *Client) Connected() bool {
	// The socketmode client manages its own connection state.
	// We track this based on whether Run has been called and hasn't returned.
	return c.teamID != ""
}

// TeamID returns the team ID discovered during connection.
func (c *Client) TeamID() string {
	return c.teamID
}

// Run starts listening for Socket Mode events and blocks until the context
// is cancelled. It calls handler for each message event received.
func (c *Client) Run(ctx context.Context, handler EventHandler) error {
	// Fetch team info to get team ID.
	info, err := c.api.AuthTestContext(ctx)
	if err != nil {
		return err
	}
	c.teamID = info.TeamID
	log.Printf("slack socket mode connected to team %s (%s)", info.Team, info.TeamID)

	go c.listenEvents(handler)

	return c.sm.RunContext(ctx)
}

// listenEvents processes incoming Socket Mode events.
func (c *Client) listenEvents(handler EventHandler) {
	for evt := range c.sm.Events {
		switch evt.Type {
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			c.sm.Ack(*evt.Request)

			if eventsAPIEvent.Type == slackevents.CallbackEvent {
				innerEvent := eventsAPIEvent.InnerEvent
				if msgEvt, ok := innerEvent.Data.(*slackevents.MessageEvent); ok {
					handler(msgEvt, c.teamID)
				}
			}

		case socketmode.EventTypeConnectionError:
			log.Printf("slack socket mode connection error")

		case socketmode.EventTypeConnected:
			log.Printf("slack socket mode connected")

		default:
			// Acknowledge other events we don't process.
			if evt.Request != nil {
				c.sm.Ack(*evt.Request)
			}
		}
	}
}
