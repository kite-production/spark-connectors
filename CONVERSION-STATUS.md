# Connector Go Conversion Status

Each connector needs its TypeScript OpenClaw implementation reviewed and
rewritten as a Go module using the Spark `BaseConnector` pattern.

**Reference implementation**: `connector-magellan` in the main [spark repo](https://github.com/kite-production/spark/tree/main/services/connector-magellan)

## Structure per connector

```
connectors/{id}/
├── connector.yaml                  ← spec (done for all 47)
└── src/                            ← Go implementation (pending)
    ├── cmd/server/main.go          ← entry point (BaseConnector + platform client)
    ├── internal/
    │   ├── service/service.go      ← ConnectorService gRPC (SendMessage, GetStatus, etc.)
    │   ├── normalizer/normalizer.go ← platform events → InboundMessage proto
    │   └── {client}/client.go      ← platform-specific API client
    ├── Dockerfile
    ├── go.mod
    └── go.sum
```

## Conversion Checklist per connector

- [ ] Review OpenClaw TypeScript source at `extensions/{id}/`
- [ ] Identify required config params (API keys, tokens, URLs)
- [ ] Identify platform API/SDK (e.g., Baileys for WhatsApp, grammY for Telegram)
- [ ] Find equivalent Go SDK or REST API
- [ ] Implement `cmd/server/main.go` (BaseConnector pattern)
- [ ] Implement `internal/service/service.go` (5 gRPC methods)
- [ ] Implement `internal/normalizer/normalizer.go` (platform → InboundMessage)
- [ ] Implement platform client (API calls to external service)
- [ ] Create `Dockerfile` (multi-stage, distroless)
- [ ] Create `go.mod` with dependencies
- [ ] Test locally with `go build ./...`
- [ ] Add to `docker-compose.yml` in main spark repo
- [ ] Smoke test: send message → receive in Spark

---

## Channel Connectors (24)

| # | Connector | SDK Language | SDK Package | Status | Priority | Notes |
|---|-----------|-------------|------------|--------|----------|-------|
| 1 | **slack** | Go | `slack-go/slack` | ✅ DONE | — | Already in spark repo |
| 2 | **gmail** | Go | `google.golang.org/api` | ✅ DONE | — | Already in spark repo |
| 3 | **googlechat** | Go | `google.golang.org/api` | ✅ DONE | — | Already in spark repo |
| 4 | **notion** | Go | `net/http` (REST) | ✅ DONE | — | Already in spark repo |
| 5 | **magellan** | Go | `net/http` | ✅ DONE | — | Already in spark repo |
| 6 | **telegram** | Go | `go-telegram-bot-api/telegram-bot-api` | ⏳ PENDING | HIGH | Mature Go SDK |
| 7 | **discord** | Go | `bwmarrin/discordgo` | ⏳ PENDING | HIGH | Mature Go SDK |
| 8 | **msteams** | Go | Microsoft Graph REST API | ⏳ PENDING | HIGH | REST, no SDK needed |
| 9 | **whatsapp** | **TypeScript** | `@whiskeysockets/baileys` | ⏳ PENDING | HIGH | No mature Go SDK — use TS |
| 10 | **matrix** | Go | `mautrix/go` | ⏳ PENDING | MEDIUM | Good Go SDK |
| 11 | **signal** | **Python** | `signal-cli` (Java CLI, Python wrapper) | ⏳ PENDING | MEDIUM | Needs signal-cli bridge |
| 12 | **line** | **TypeScript** | `@line/bot-sdk` | ⏳ PENDING | MEDIUM | TS SDK preferred |
| 13 | **irc** | Go | `thoj/go-ircevent` | ⏳ PENDING | LOW | Simple protocol |
| 14 | **mattermost** | Go | `mattermost/model` | ⏳ PENDING | LOW | Good Go SDK |
| 15 | **twitch** | Go | `gempir/go-twitch-irc` | ⏳ PENDING | LOW | IRC-based |
| 16 | **nostr** | Go | `nbd-wtf/go-nostr` | ⏳ PENDING | LOW | Go SDK exists |
| 17 | **feishu** | **TypeScript** | `@larksuiteoapi/node-sdk` | ⏳ PENDING | LOW | TS SDK preferred |
| 18 | **imessage** | Go | osascript bridge | ⏳ PENDING | LOW | macOS only |
| 19 | **bluebubbles** | **TypeScript** | BlueBubbles API | ⏳ PENDING | LOW | TS SDK only |
| 20 | **zalo** | Go | REST API (`net/http`) | ⏳ PENDING | LOW | REST-based |
| 21 | **zalouser** | Go | REST API (`net/http`) | ⏳ PENDING | LOW | REST-based |
| 22 | **qqbot** | Go | REST API (`net/http`) | ⏳ PENDING | LOW | REST-based |
| 23 | **nextcloud-talk** | Go | REST API (`net/http`) | ⏳ PENDING | LOW | REST-based |
| 24 | **synology-chat** | Go | Webhook API (`net/http`) | ⏳ PENDING | LOW | Webhook-based |
| 25 | **tlon** | Go | REST API (`net/http`) | ⏳ PENDING | LOW | Niche |
| 26 | **xiaomi** | **TypeScript** | `xiaomi-cloud` | ⏳ PENDING | LOW | TS SDK only |
| 27 | **voice-call** | Go | `pion/webrtc` | ⏳ PENDING | LOW | Complex protocol |

## Search Providers (4)

| # | Connector | Go SDK/API | Status | Priority | Notes |
|---|-----------|-----------|--------|----------|-------|
| 1 | **brave** | Brave Search REST API | ⏳ PENDING | MEDIUM | Simple REST |
| 2 | **duckduckgo** | DuckDuckGo Instant Answer API | ⏳ PENDING | MEDIUM | Simple REST |
| 3 | **tavily** | Tavily REST API | ⏳ PENDING | MEDIUM | Simple REST |
| 4 | **searxng** | SearXNG REST API | ⏳ PENDING | LOW | Self-hosted |

## Tools (3)

| # | Connector | Go SDK/API | Status | Priority | Notes |
|---|-----------|-----------|--------|----------|-------|
| 1 | **browser** | `chromedp/chromedp` | ⏳ PENDING | HIGH | Chrome DevTools Protocol |
| 2 | **firecrawl** | Firecrawl REST API | ⏳ PENDING | MEDIUM | Simple REST |
| 3 | **exa** | Exa REST API | ⏳ PENDING | MEDIUM | Simple REST |

## Services (9)

| # | Connector | Go SDK/API | Status | Priority | Notes |
|---|-----------|-----------|--------|----------|-------|
| 1 | **elevenlabs** | ElevenLabs REST API | ⏳ PENDING | MEDIUM | TTS service |
| 2 | **deepgram** | Deepgram REST/WebSocket API | ⏳ PENDING | MEDIUM | STT service |
| 3 | **fal** | FAL REST API | ⏳ PENDING | LOW | Image generation |
| 4 | **image-generation-core** | Internal service | ⏳ PENDING | LOW | Wrapper |
| 5 | **media-understanding-core** | Internal service | ⏳ PENDING | LOW | Wrapper |
| 6 | **memory-core** | Internal service | ⏳ PENDING | LOW | Wrapper |
| 7 | **memory-lancedb** | LanceDB Go bindings | ⏳ PENDING | LOW | Vector DB |
| 8 | **speech-core** | Internal service | ⏳ PENDING | LOW | Wrapper |
| 9 | **talk-voice** | WebRTC / audio streaming | ⏳ PENDING | LOW | Voice |

## Devices (2)

| # | Connector | Go SDK/API | Status | Priority | Notes |
|---|-----------|-----------|--------|----------|-------|
| 1 | **device-pair** | Bluetooth / mDNS | ⏳ PENDING | LOW | Device discovery |
| 2 | **phone-control** | ADB / Device API | ⏳ PENDING | LOW | Android control |

---

## Priority Order

1. **HIGH** — WhatsApp, Telegram, Discord, MS Teams, Browser (most requested channels + critical tool)
2. **MEDIUM** — Signal, Matrix, LINE, Brave, Tavily, Firecrawl, Exa, ElevenLabs, Deepgram
3. **LOW** — Everything else (niche channels, internal services, devices)

## Go SDK Reference

| Platform | Recommended Go Package |
|----------|----------------------|
| WhatsApp | `github.com/mattn/go-whatsapp` or REST wrapper for Baileys |
| Telegram | `github.com/go-telegram-bot-api/telegram-bot-api/v5` |
| Discord | `github.com/bwmarrin/discordgo` |
| MS Teams | `github.com/microsoftgraph/msgraph-sdk-go` |
| Signal | REST API via `signal-cli` |
| Matrix | `maunium.net/go/mautrix` |
| IRC | `github.com/thoj/go-ircevent` |
| LINE | LINE Messaging API (REST, `net/http`) |
| Twitch | `github.com/gempir/go-twitch-irc/v4` |
| Nostr | `github.com/nbd-wtf/go-nostr` |
| Browser | `github.com/chromedp/chromedp` |
| Mattermost | `github.com/mattermost/mattermost/server/public/model` |
