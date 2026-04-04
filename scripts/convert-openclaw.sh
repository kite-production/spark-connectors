#!/usr/bin/env bash
# Convert OpenClaw extensions to Spark connector YAML specs.
#
# Usage:
#   ./scripts/convert-openclaw.sh [openclaw-repo-path]
#
# If no path is given, clones openclaw/openclaw to /tmp/openclaw.
#
set -euo pipefail

OPENCLAW_DIR="${1:-}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)/connectors"

if [ -z "$OPENCLAW_DIR" ]; then
    OPENCLAW_DIR="/tmp/openclaw"
    if [ ! -d "$OPENCLAW_DIR/.git" ]; then
        echo "⏳ Cloning openclaw/openclaw..."
        git clone --depth 1 https://github.com/openclaw/openclaw.git "$OPENCLAW_DIR" 2>/dev/null
    else
        echo "✓ Using cached openclaw at $OPENCLAW_DIR"
    fi
fi

EXT_DIR="$OPENCLAW_DIR/extensions"
if [ ! -d "$EXT_DIR" ]; then
    echo "❌ Extensions directory not found at $EXT_DIR"
    exit 1
fi

GENERATED=0
SKIPPED=0

# ── Classification maps ──────────────────────────────────────────────────────

# Channel connectors (messaging platforms)
CHANNELS="whatsapp telegram slack discord googlechat signal imessage bluebubbles irc msteams matrix feishu line mattermost nextcloud-talk nostr synology-chat tlon twitch zalo zalouser qqbot voice-call xiaomi"

# Tool extensions
TOOLS="browser firecrawl exa"

# Service extensions (TTS, STT, image gen, etc.)
SERVICES="elevenlabs deepgram fal image-generation-core media-understanding-core memory-core memory-lancedb speech-core talk-voice"

# Search providers
SEARCHES="brave duckduckgo tavily searxng"

# Device extensions
DEVICES="device-pair phone-control"

# AI model providers (skip — these are model providers, not connectors)
PROVIDERS="anthropic openai google deepseek groq ollama microsoft amazon-bedrock anthropic-vertex nvidia mistral together xai huggingface openrouter litellm cloudflare-ai-gateway vercel-ai-gateway moonshot minimax stepfun kimi-coding qianfan volcengine byteplus vllm sglang venice chutes modelstudio zai microsoft-foundry github-copilot copilot-proxy open-prose opencode opencode-go openshell kilocode"

# Internal/utility (skip)
INTERNAL="shared diagnostics-otel diffs lobster thread-ownership synthetic"

get_type() {
    local id="$1"
    for c in $CHANNELS; do [ "$c" = "$id" ] && echo "channel" && return; done
    for t in $TOOLS; do [ "$t" = "$id" ] && echo "tool" && return; done
    for s in $SERVICES; do [ "$s" = "$id" ] && echo "service" && return; done
    for s in $SEARCHES; do [ "$s" = "$id" ] && echo "search" && return; done
    for d in $DEVICES; do [ "$d" = "$id" ] && echo "device" && return; done
    for p in $PROVIDERS; do [ "$p" = "$id" ] && echo "provider" && return; done
    for i in $INTERNAL; do [ "$i" = "$id" ] && echo "internal" && return; done
    echo "unknown"
}

get_category() {
    local type="$1"
    case "$type" in
        channel)  echo "Communication" ;;
        tool)     echo "DevTools" ;;
        service)  echo "Services" ;;
        search)   echo "Search" ;;
        device)   echo "Devices" ;;
        *)        echo "Other" ;;
    esac
}

get_icon() {
    local id="$1"
    case "$id" in
        whatsapp)       echo "chat" ;;
        telegram)       echo "send" ;;
        slack)          echo "forum" ;;
        discord)        echo "headset_mic" ;;
        googlechat)     echo "question_answer" ;;
        signal)         echo "lock" ;;
        imessage)       echo "message" ;;
        bluebubbles)    echo "chat_bubble" ;;
        irc)            echo "terminal" ;;
        msteams)        echo "groups" ;;
        matrix)         echo "grid_view" ;;
        feishu)         echo "translate" ;;
        line)           echo "chat" ;;
        mattermost)     echo "forum" ;;
        nextcloud-talk) echo "cloud" ;;
        nostr)          echo "public" ;;
        synology-chat)  echo "storage" ;;
        tlon)           echo "language" ;;
        twitch)         echo "videocam" ;;
        zalo|zalouser)  echo "chat" ;;
        qqbot)          echo "smart_toy" ;;
        voice-call)     echo "call" ;;
        xiaomi)         echo "devices" ;;
        browser)        echo "language" ;;
        firecrawl)      echo "web" ;;
        exa)            echo "search" ;;
        elevenlabs)     echo "record_voice_over" ;;
        deepgram)       echo "mic" ;;
        fal)            echo "image" ;;
        brave)          echo "search" ;;
        duckduckgo)     echo "search" ;;
        tavily)         echo "search" ;;
        searxng)        echo "search" ;;
        device-pair)    echo "bluetooth" ;;
        phone-control)  echo "phone_android" ;;
        *)              echo "extension" ;;
    esac
}

# ── Process each extension ───────────────────────────────────────────────────

echo ""
echo "════════════════════════════════════════════════════════════"
echo "  OpenClaw → Spark Connector Conversion"
echo "════════════════════════════════════════════════════════════"
echo ""

for ext_path in "$EXT_DIR"/*/; do
    ext_id=$(basename "$ext_path")
    ext_type=$(get_type "$ext_id")

    # Skip model providers and internal utilities
    if [ "$ext_type" = "provider" ] || [ "$ext_type" = "internal" ] || [ "$ext_type" = "unknown" ]; then
        ((SKIPPED++))
        continue
    fi

    category=$(get_category "$ext_type")
    icon=$(get_icon "$ext_id")

    # Read package.json for metadata
    pkg_json="$ext_path/package.json"
    plugin_json="$ext_path/openclaw.plugin.json"

    name="$ext_id"
    version="1.0.0"
    description="OpenClaw ${ext_id} integration"

    if [ -f "$pkg_json" ]; then
        # Extract fields from package.json
        pkg_name=$(python3 -c "import json; d=json.load(open('$pkg_json')); print(d.get('name',''))" 2>/dev/null || echo "")
        pkg_version=$(python3 -c "import json; d=json.load(open('$pkg_json')); print(d.get('version','1.0.0'))" 2>/dev/null || echo "1.0.0")
        pkg_desc=$(python3 -c "import json; d=json.load(open('$pkg_json')); print(d.get('description',''))" 2>/dev/null || echo "")

        if [ -n "$pkg_desc" ]; then
            description="$pkg_desc"
        fi
        version="$pkg_version"
        # Clean name: @openclaw/whatsapp → WhatsApp
        display_name=$(echo "$ext_id" | sed 's/-/ /g' | sed 's/\b\(.\)/\u\1/g')
    else
        display_name=$(echo "$ext_id" | sed 's/-/ /g' | sed 's/\b\(.\)/\u\1/g')
    fi

    # Read plugin.json for config schema and capabilities
    config_props=""
    channels_list=""
    tools_list=""
    if [ -f "$plugin_json" ]; then
        channels_list=$(python3 -c "
import json
d = json.load(open('$plugin_json'))
ch = d.get('channels', [])
print(','.join(ch))
" 2>/dev/null || echo "")

        tools_list=$(python3 -c "
import json
d = json.load(open('$plugin_json'))
tools = d.get('tools', [])
print(','.join(tools))
" 2>/dev/null || echo "")

        config_props=$(python3 -c "
import json, sys
d = json.load(open('$plugin_json'))
schema = d.get('configSchema', {})
props = schema.get('properties', {})
for key, val in props.items():
    ptype = val.get('type', 'string')
    desc = val.get('description', key)
    print(f'{key}|{ptype}|{desc}')
" 2>/dev/null || echo "")
    fi

    # Look for environment variables in source files
    env_vars=$(grep -rhoE '[A-Z_]{3,}_(?:KEY|TOKEN|SECRET|URL|PORT|API)' "$ext_path" 2>/dev/null | sort -u | head -10 || echo "")

    # Create output directory
    mkdir -p "$OUTPUT_DIR/$ext_id"
    out_file="$OUTPUT_DIR/$ext_id/connector.yaml"

    # ── Generate YAML ──
    cat > "$out_file" << YAML
# Spark Connector Spec — ${display_name}
# Auto-generated from OpenClaw extension: ${ext_id}
apiVersion: spark.dev/v1
kind: Connector
metadata:
  id: ${ext_id}
  name: "${display_name}"
  type: ${ext_type}
  version: "${version}"
  publisher: "OpenClaw Community"
  category: "${category}"
  tags: [$(echo "$ext_type" | sed 's/.*/"\u&"/')]
  icon: ${icon}
  description: >
    ${description}
  source: "https://github.com/openclaw/openclaw/tree/main/extensions/${ext_id}"
  license: MIT

spec:
  docker:
    image: spark/connector-${ext_id}
    tag: "${version}"

  auth:
    type: $([ -n "$env_vars" ] && echo "api_key" || echo "none")
YAML

    # Add capabilities for channels
    if [ "$ext_type" = "channel" ]; then
        cat >> "$out_file" << 'YAML'

  capabilities:
    supports_threads: true
    supports_reactions: false
    supports_edit: false
    supports_unsend: false
    supports_reply: true
    supports_attachments: true
    supports_images: true
    max_message_length: 4096
YAML
    fi

    # Add config from plugin.json or env vars
    echo "" >> "$out_file"
    echo "  config:" >> "$out_file"
    if [ -n "$config_props" ]; then
        echo "$config_props" | while IFS='|' read -r cname ctype cdesc; do
            stype="text"
            [[ "$cname" == *key* || "$cname" == *token* || "$cname" == *secret* ]] && stype="secret"
            cat >> "$out_file" << YAML
    - name: ${cname}
      display: "${cname}"
      type: ${stype}
      required: false
      description: "${cdesc}"
YAML
        done
    elif [ -n "$env_vars" ]; then
        echo "$env_vars" | while read -r evar; do
            lvar=$(echo "$evar" | tr '[:upper:]' '[:lower:]')
            stype="text"
            [[ "$evar" == *KEY* || "$evar" == *TOKEN* || "$evar" == *SECRET* ]] && stype="secret"
            cat >> "$out_file" << YAML
    - name: ${lvar}
      display: "${evar}"
      type: ${stype}
      required: true
      description: "Environment variable: ${evar}"
YAML
        done
    else
        echo "    []" >> "$out_file"
    fi

    # Add tools
    echo "" >> "$out_file"
    echo "  tools:" >> "$out_file"
    if [ -n "$tools_list" ]; then
        echo "$tools_list" | tr ',' '\n' | while read -r tool; do
            [ -z "$tool" ] && continue
            cat >> "$out_file" << YAML
    - name: ${tool}
      method: POST
      description: "${tool} operation"
      args: []
YAML
        done
    else
        echo "    []" >> "$out_file"
    fi

    # Add versions
    cat >> "$out_file" << YAML

  versions:
    - version: "${version}"
      date: "2026-04-04"
      changes:
        - "Auto-generated from OpenClaw extension"
YAML

    echo "  ✓ ${ext_id} (${ext_type}) → ${out_file}"
    ((GENERATED++))
done

echo ""
echo "════════════════════════════════════════════════════════════"
echo "  Generated: ${GENERATED} | Skipped: ${SKIPPED}"
echo "════════════════════════════════════════════════════════════"
