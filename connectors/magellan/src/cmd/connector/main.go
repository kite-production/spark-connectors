// Magellan on-demand connector entrypoint.
//
// Exposes 6 tools that bridge Spark agents to the Venus test
// messaging service. Every tool call spawns a fresh Docker container
// that runs:
//
//	/bin/connector execute <tool> <method> <args-json>
//
// The binary dispatches to the right Venus HTTP call, writes the
// Venus response to stdout, and exits.
//
// This is a pure on-demand connector. Venus webhook ingestion
// (inbound chat messages → NATS) has been moved out to a separate
// always-on service called `venus-bridge` in the Spark repo.
// Magellan here is tools-only.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kite-production/connector-magellan/internal/venus"
)

const invocationTimeout = 25 * time.Second

func main() {
	if len(os.Args) < 2 {
		writeError("usage: connector execute <tool> <method> <args-json>")
		os.Exit(2)
	}
	if os.Args[1] != "execute" {
		writeError("unknown verb: " + os.Args[1])
		os.Exit(2)
	}
	if len(os.Args) < 5 {
		writeError("execute requires <tool> <method> <args-json>")
		os.Exit(2)
	}
	tool := os.Args[2]
	_ = os.Args[3] // method — accepted for forward-compat
	argsJSON := os.Args[4]

	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			writeError("invalid args JSON: " + err.Error())
			os.Exit(2)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), invocationTimeout)
	defer cancel()

	venusURL := envOr("VENUS_URL", "http://venus:8090")
	apiKey := envOr("VENUS_API_KEY", "")
	client := venus.New(venusURL, apiKey)

	switch tool {
	case "venus-send-message":
		dispatchSendMessage(client, args)
	case "venus-list-messages":
		dispatchListMessages(client, args)
	case "venus-create-channel":
		dispatchCreateChannel(client, args)
	case "venus-list-channels":
		dispatchListChannels(client)
	case "venus-register-webhook":
		dispatchRegisterWebhook(client, args)
	case "venus-get-status":
		dispatchGetStatus(client)
	default:
		writeError("unknown tool: " + tool)
		os.Exit(2)
	}

	_ = ctx // ctx currently unused by Venus client (uses http.Client timeout);
	// keep for when the client adapts to context-aware requests.
}

// ── Tool dispatchers ─────────────────────────────────────────────────

func dispatchSendMessage(c *venus.Client, args map[string]any) {
	channelID := stringArg(args, "channel_id")
	text := stringArg(args, "text")
	if channelID == "" {
		die("missing required arg: channel_id")
	}
	if text == "" {
		die("missing required arg: text")
	}
	raw, err := c.SendMessage(channelID, text, stringArg(args, "thread_id"), stringArg(args, "reply_to_id"))
	if err != nil {
		die("send failed: " + err.Error())
	}
	writeRaw(raw)
}

func dispatchListMessages(c *venus.Client, args map[string]any) {
	channelID := stringArg(args, "channel_id")
	if channelID == "" {
		die("missing required arg: channel_id")
	}
	raw, err := c.ListMessages(channelID, stringArg(args, "since"))
	if err != nil {
		die("list failed: " + err.Error())
	}
	writeRaw(raw)
}

func dispatchCreateChannel(c *venus.Client, args map[string]any) {
	name := stringArg(args, "name")
	if name == "" {
		die("missing required arg: name")
	}
	raw, err := c.CreateChannel(name, stringArg(args, "description"))
	if err != nil {
		die("create-channel failed: " + err.Error())
	}
	writeRaw(raw)
}

func dispatchListChannels(c *venus.Client) {
	raw, err := c.ListChannels()
	if err != nil {
		die("list-channels failed: " + err.Error())
	}
	writeRaw(raw)
}

func dispatchRegisterWebhook(c *venus.Client, args map[string]any) {
	url := stringArg(args, "url")
	if url == "" {
		die("missing required arg: url")
	}
	raw, err := c.RegisterWebhook(url)
	if err != nil {
		die("register-webhook failed: " + err.Error())
	}
	writeRaw(raw)
}

func dispatchGetStatus(c *venus.Client) {
	raw, err := c.GetStatus()
	if err != nil {
		die("get-status failed: " + err.Error())
	}
	writeRaw(raw)
}

// ── Helpers ──────────────────────────────────────────────────────────

// writeRaw passes through a Venus JSON response verbatim. Venus
// already returns well-formed JSON for every tool we support, so
// re-wrapping would just add noise. Appends a newline for tidy logs.
func writeRaw(raw []byte) {
	_, _ = os.Stdout.Write(raw)
	_, _ = os.Stdout.Write([]byte("\n"))
}

func writeError(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": msg})
}

func die(msg string) {
	writeError(msg)
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
