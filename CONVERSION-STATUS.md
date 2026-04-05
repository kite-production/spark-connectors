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

| # | Connector | Go SDK/API | Status | Priority | Notes |
|---|-----------|-----------|--------|----------|-------|
| 1 | **slack** | `slack-go/slack` | ✅ DONE | — | Already built in spark repo |
| 2 | **gmail** | Google API Go client | ✅ DONE | — | Already built in spark repo |
| 3 | **googlechat** | Google API Go client | ✅ DONE | — | Already built in spark repo |
| 4 | **notion** | Notion REST API | ✅ DONE | — | Already built in spark repo |
| 5 | **magellan** | Venus HTTP API | ✅ DONE | — | Already built in spark repo |
| 6 | **whatsapp** | `mattn/go-whatsapp` or Baileys REST wrapper | ⏳ PENDING | HIGH | 2B+ users, critical channel |
| 7 | **telegram** | `go-telegram-bot-api/telegram-bot-api` | ⏳ PENDING | HIGH | Well-documented Go SDK |
| 8 | **discord** | `bwmarrin/discordgo` | ⏳ PENDING | HIGH | Mature Go SDK |
| 9 | **msteams** | Microsoft Graph API Go client | ⏳ PENDING | HIGH | Enterprise channel |
| 10 | **signal** | Signal REST API / `signal-cli` | ⏳ PENDING | MEDIUM | May need signal-cli bridge |
| 11 | **matrix** | `mautrix/go` | ⏳ PENDING | MEDIUM | Good Go SDK exists |
| 12 | **imessage** | AppleScript / BlueBubbles API | ⏳ PENDING | LOW | macOS only |
| 13 | **bluebubbles** | BlueBubbles REST API | ⏳ PENDING | LOW | iMessage bridge |
| 14 | **irc** | `thoj/go-ircevent` | ⏳ PENDING | LOW | Simple protocol |
| 15 | **line** | LINE Messaging API (REST) | ⏳ PENDING | MEDIUM | REST-based |
| 16 | **mattermost** | `mattermost/model` Go pkg | ⏳ PENDING | LOW | Good Go SDK |
| 17 | **feishu** | Lark/Feishu Open API (REST) | ⏳ PENDING | LOW | REST-based |
| 18 | **twitch** | `gempir/go-twitch-irc` | ⏳ PENDING | LOW | IRC-based |
| 19 | **nostr** | `nbd-wtf/go-nostr` | ⏳ PENDING | LOW | Go SDK exists |
| 20 | **zalo** | Zalo OA REST API | ⏳ PENDING | LOW | REST-based |
| 21 | **zalouser** | Zalo User API | ⏳ PENDING | LOW | REST-based |
| 22 | **qqbot** | QQ Bot REST API | ⏳ PENDING | LOW | REST-based |
| 23 | **nextcloud-talk** | Nextcloud Talk REST API | ⏳ PENDING | LOW | REST-based |
| 24 | **synology-chat** | Synology Chat webhook API | ⏳ PENDING | LOW | Webhook-based |
| 25 | **tlon** | Tlon/Urbit API | ⏳ PENDING | LOW | Niche |
| 26 | **xiaomi** | Xiaomi IoT API | ⏳ PENDING | LOW | IoT device |
| 27 | **voice-call** | WebRTC / SIP | ⏳ PENDING | LOW | Complex protocol |

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
