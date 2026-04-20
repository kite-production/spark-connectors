// Hubble on-demand connector entrypoint.
//
// Spark's connector-manager spawns a fresh Docker container for each
// tool call with:
//
//	/bin/connector execute <tool-name> <method> <args-json>
//
// This binary parses those four positional arguments, dispatches to
// the right handler, writes a JSON result to stdout, and exits 0.
// On error it writes a JSON error envelope to stdout AND exits
// non-zero so connector-manager can surface a proper tool error to
// the agent while the body still contains a useful message.
//
// No HTTP server, no gRPC, no NATS, no always-on state. Each
// invocation is independent; environment variables (injected by
// connector-manager from the instance config) provide the per-call
// configuration such as SEARXNG_URL.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kite-production/connector-hubble/internal/fetch"
	"github.com/kite-production/connector-hubble/internal/searxng"
)

// Per-invocation timeout. Keep well below connector-manager's own
// 30-second HTTP client timeout so we report our own error instead
// of a generic "context deadline exceeded" from the manager side.
const invocationTimeout = 25 * time.Second

func main() {
	if len(os.Args) < 2 {
		writeError("usage: connector execute <tool> <method> <args-json>")
		os.Exit(2)
	}
	verb := os.Args[1]
	switch verb {
	case "execute":
		runExecute()
	case "--help", "-h", "help":
		fmt.Fprintln(os.Stderr, "usage: connector execute <tool> <method> <args-json>")
		os.Exit(0)
	default:
		writeError("unknown verb: " + verb)
		os.Exit(2)
	}
}

func runExecute() {
	if len(os.Args) < 5 {
		writeError("execute requires <tool> <method> <args-json>")
		os.Exit(2)
	}
	tool := os.Args[2]
	// os.Args[3] is the HTTP method ("GET"/"POST") from the YAML
	// spec. We accept any value — tool dispatch is keyed on tool
	// name alone. Leaving it in the CLI surface keeps the contract
	// open for future tools that might genuinely switch on method.
	_ = os.Args[3]
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

	switch tool {
	case "web-search":
		handleSearch(ctx, args)
	case "web-fetch":
		handleFetch(ctx, args)
	default:
		writeError("unknown tool: " + tool)
		os.Exit(2)
	}
}

// ── Tool handlers ────────────────────────────────────────────────────

func handleSearch(ctx context.Context, args map[string]any) {
	query := stringArg(args, "query")
	if query == "" {
		writeError("missing required arg: query")
		os.Exit(1)
	}
	maxResults := intArg(args, "max_results", 5)

	searxngURL := envOr("SEARXNG_URL", "http://searxng:8080")
	client := searxng.New(searxngURL)

	results, err := client.Search(ctx, query, maxResults)
	if err != nil {
		writeError("search failed: " + err.Error())
		os.Exit(1)
	}

	writeJSON(map[string]any{
		"query":   query,
		"count":   len(results),
		"results": results,
	})
}

func handleFetch(ctx context.Context, args map[string]any) {
	url := stringArg(args, "url")
	if url == "" {
		writeError("missing required arg: url")
		os.Exit(1)
	}
	maxLength := intArg(args, "max_length", 0) // 0 = use fetcher default

	result, err := fetch.New().Fetch(ctx, url, maxLength)
	if err != nil {
		writeError("fetch failed: " + err.Error())
		os.Exit(1)
	}
	writeJSON(result)
}

// ── Helpers ──────────────────────────────────────────────────────────

func writeJSON(body any) {
	data, err := json.Marshal(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal: %v\n", err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(data)
	_, _ = os.Stdout.Write([]byte("\n"))
}

// writeError emits a structured error envelope on stdout so the agent
// sees a JSON body with an `error` field even when we exit non-zero.
// The error is ALSO written to stderr so it appears in connector
// container logs.
func writeError(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": msg})
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

// intArg reads an integer arg that could arrive as a native int,
// a JSON number (float64 after unmarshal), or a stringified number
// (some MCP clients stringify everything). Returns fallback if
// unparseable or missing.
func intArg(args map[string]any, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	v, ok := args[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		if n == "" {
			return fallback
		}
		var parsed int
		if _, err := fmt.Sscanf(n, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}
