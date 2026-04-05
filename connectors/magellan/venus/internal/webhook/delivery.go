// Package webhook handles delivering new messages to registered webhook URLs.
package webhook

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/kite-production/spark/services/venus/internal/store"
)

// Deliverer sends new messages to registered webhooks.
type Deliverer struct {
	store  *store.Store
	client *http.Client
}

// New creates a webhook deliverer and subscribes to new messages.
func New(s *store.Store) *Deliverer {
	d := &Deliverer{
		store: s,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	// Subscribe to all new messages from the store.
	s.Subscribe(d.deliver)
	return d
}

// deliver sends a message to all registered webhooks.
func (d *Deliverer) deliver(msg *store.Message) {
	webhooks := d.store.ListWebhooks()
	if len(webhooks) == 0 {
		return
	}

	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[webhook] failed to marshal message: %v", err)
		return
	}

	for _, wh := range webhooks {
		go d.post(wh.URL, body)
	}
}

// post sends the message body to a single webhook URL.
func (d *Deliverer) post(url string, body []byte) {
	resp, err := d.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[webhook] delivery failed to %s: %v", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[webhook] delivery to %s returned %d", url, resp.StatusCode)
	}
}
