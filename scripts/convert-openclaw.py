#!/usr/bin/env python3
"""
Convert OpenClaw extensions to Spark connector YAML specs (spark.dev/v1).

Usage:
    python3 scripts/convert-openclaw.py [openclaw-repo-path]

If no path is given, expects openclaw at /tmp/openclaw.
"""

import json
import os
import re
import sys
from pathlib import Path
from datetime import date

# ── Configuration ────────────────────────────────────────────────────────────

OPENCLAW_DIR = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("/tmp/openclaw")
OUTPUT_DIR = Path(__file__).resolve().parent.parent / "connectors"

TODAY = date.today().isoformat()

# ── Type Classification ──────────────────────────────────────────────────────

TYPE_MAP = {
    # Channels (messaging platforms) — detected by openclaw.plugin.json "channels" field
    # but we also have a manual override for edge cases
    "whatsapp": "channel", "telegram": "channel", "slack": "channel",
    "discord": "channel", "googlechat": "channel", "signal": "channel",
    "imessage": "channel", "bluebubbles": "channel", "irc": "channel",
    "msteams": "channel", "matrix": "channel", "feishu": "channel",
    "line": "channel", "mattermost": "channel", "nextcloud-talk": "channel",
    "nostr": "channel", "synology-chat": "channel", "tlon": "channel",
    "twitch": "channel", "zalo": "channel", "zalouser": "channel",
    "qqbot": "channel", "voice-call": "channel", "xiaomi": "channel",

    # Tools
    "browser": "tool", "firecrawl": "tool", "exa": "tool",

    # Services
    "elevenlabs": "service", "deepgram": "service", "fal": "service",
    "image-generation-core": "service", "media-understanding-core": "service",
    "memory-core": "service", "memory-lancedb": "service",
    "speech-core": "service", "talk-voice": "service",

    # Search
    "brave": "search", "duckduckgo": "search", "tavily": "search", "searxng": "search",

    # Devices
    "device-pair": "device", "phone-control": "device",
}

# Extensions to skip (model providers, internal utils)
SKIP_IDS = {
    "anthropic", "openai", "google", "deepseek", "groq", "ollama", "microsoft",
    "amazon-bedrock", "anthropic-vertex", "nvidia", "mistral", "together", "xai",
    "huggingface", "openrouter", "litellm", "cloudflare-ai-gateway",
    "vercel-ai-gateway", "moonshot", "minimax", "stepfun", "kimi-coding",
    "qianfan", "volcengine", "byteplus", "vllm", "sglang", "venice", "chutes",
    "modelstudio", "zai", "microsoft-foundry", "github-copilot", "copilot-proxy",
    "open-prose", "opencode", "opencode-go", "openshell", "kilocode",
    # Internal
    "shared", "diagnostics-otel", "diffs", "lobster", "thread-ownership",
    "synthetic", "acpx", "llm-task",
}

CATEGORY_MAP = {
    "channel": "Communication", "tool": "DevTools", "service": "Services",
    "search": "Search", "device": "Devices",
}

ICON_MAP = {
    "whatsapp": ("chat", "#25D366"), "telegram": ("send", "#0088CC"),
    "slack": ("forum", "#4A154B"), "discord": ("headset_mic", "#5865F2"),
    "googlechat": ("question_answer", "#00AC47"), "signal": ("lock", "#3A76F0"),
    "imessage": ("message", "#34C759"), "bluebubbles": ("chat_bubble", "#007AFF"),
    "irc": ("terminal", "#8B8B8B"), "msteams": ("groups", "#6264A7"),
    "matrix": ("grid_view", "#0DBD8B"), "feishu": ("translate", "#3370FF"),
    "line": ("chat", "#06C755"), "mattermost": ("forum", "#0058CC"),
    "nextcloud-talk": ("cloud", "#0082C9"), "nostr": ("public", "#8B5CF6"),
    "synology-chat": ("storage", "#B5B5B6"), "tlon": ("language", "#000000"),
    "twitch": ("videocam", "#9146FF"), "zalo": ("chat", "#0068FF"),
    "zalouser": ("chat", "#0068FF"), "qqbot": ("smart_toy", "#12B7F5"),
    "voice-call": ("call", "#7bdc7b"), "xiaomi": ("devices", "#FF6900"),
    "browser": ("language", "#4285F4"), "firecrawl": ("local_fire_department", "#FF6B35"),
    "exa": ("search", "#a7c8ff"), "elevenlabs": ("record_voice_over", "#000000"),
    "deepgram": ("mic", "#13EF93"), "fal": ("image", "#6366F1"),
    "brave": ("search", "#FB542B"), "duckduckgo": ("search", "#DE5833"),
    "tavily": ("search", "#5B6AFF"), "searxng": ("search", "#3050FF"),
    "device-pair": ("bluetooth", "#0082FC"), "phone-control": ("phone_android", "#3DDC84"),
    "image-generation-core": ("image", "#c084fc"), "media-understanding-core": ("visibility", "#fbbc30"),
    "memory-core": ("database", "#a7c8ff"), "memory-lancedb": ("database", "#7bdc7b"),
    "speech-core": ("graphic_eq", "#a7c8ff"), "talk-voice": ("record_voice_over", "#fbbc30"),
}

AUTH_MAP = {
    "whatsapp": ("qr_code", "interactive"), "telegram": ("api_key", "automatic"),
    "slack": ("oauth2", "interactive"), "discord": ("api_key", "automatic"),
    "googlechat": ("oauth2", "interactive"), "signal": ("device_token", "interactive"),
    "imessage": ("none", "automatic"), "bluebubbles": ("api_key", "automatic"),
    "msteams": ("oauth2", "interactive"), "matrix": ("api_key", "automatic"),
    "line": ("api_key", "automatic"), "twitch": ("oauth2", "interactive"),
}

INGESTION_MAP = {
    "channel": ("true", "push", "Receives messages from the platform"),
    "tool": ("false", "none", "Action-only — invoked by agents on demand"),
    "service": ("false", "none", "Service connector — provides capabilities to agents"),
    "search": ("false", "none", "Search provider — returns results on query"),
    "device": ("false", "none", "Device connector — executes commands on paired devices"),
}


# ── Helpers ──────────────────────────────────────────────────────────────────

def read_json(path: Path) -> dict:
    """Safely read a JSON file."""
    try:
        with open(path) as f:
            return json.load(f)
    except Exception:
        return {}


def scan_env_vars(ext_dir: Path) -> list[str]:
    """Scan source files for environment variable patterns."""
    env_vars = set()
    patterns = [
        r'process\.env\.([A-Z_]{3,})',
        r'env\("([A-Z_]{3,})"\)',
        r'getenv\("([A-Z_]{3,})"\)',
        r'"([A-Z][A-Z_]{2,}(?:_KEY|_TOKEN|_SECRET|_URL|_PORT|_API|_ID))"',
    ]
    for ts_file in ext_dir.rglob("*.ts"):
        try:
            content = ts_file.read_text(errors="ignore")
            for pattern in patterns:
                env_vars.update(re.findall(pattern, content))
        except Exception:
            continue
    # Filter out common non-config vars
    skip = {"NODE_ENV", "HOME", "PATH", "DEBUG", "CI", "TEST"}
    return sorted(env_vars - skip)


def scan_source_files(ext_dir: Path) -> list[dict]:
    """Read source files for the code section (top-level .ts files only)."""
    files = []
    for f in sorted(ext_dir.iterdir()):
        if f.is_file() and f.suffix in (".ts", ".json") and f.name != "package.json":
            try:
                content = f.read_text(errors="ignore")
                if len(content) > 20000:
                    content = content[:20000] + "\n# ... truncated (full source in repository)"
                files.append({
                    "path": f.name,
                    "language": "typescript" if f.suffix == ".ts" else "json",
                    "size": len(content),
                })
            except Exception:
                continue
    return files


def display_name(ext_id: str) -> str:
    """Convert kebab-case ID to display name."""
    special = {
        "whatsapp": "WhatsApp", "msteams": "Microsoft Teams",
        "googlechat": "Google Chat", "imessage": "iMessage",
        "bluebubbles": "BlueBubbles", "irc": "IRC", "qqbot": "QQ Bot",
        "searxng": "SearXNG", "duckduckgo": "DuckDuckGo",
        "elevenlabs": "ElevenLabs", "deepgram": "Deepgram",
        "fal": "FAL", "exa": "Exa", "zalouser": "Zalo Personal",
        "nextcloud-talk": "Nextcloud Talk", "synology-chat": "Synology Chat",
        "voice-call": "Voice Call", "device-pair": "Device Pair",
        "phone-control": "Phone Control",
        "image-generation-core": "Image Generation",
        "media-understanding-core": "Media Understanding",
        "memory-core": "Memory Core", "memory-lancedb": "Memory LanceDB",
        "speech-core": "Speech Core", "talk-voice": "Talk Voice",
    }
    return special.get(ext_id, ext_id.replace("-", " ").title())


def yaml_str(s: str, indent: int = 0) -> str:
    """Safely format a string for YAML output."""
    s = s.replace('"', '\\"')
    return s


def config_type_from_env(var_name: str) -> str:
    """Determine config type from env var name."""
    var_upper = var_name.upper()
    if any(k in var_upper for k in ("KEY", "TOKEN", "SECRET", "PASSWORD")):
        return "secret"
    if any(k in var_upper for k in ("PORT", "TIMEOUT", "LIMIT", "MAX", "MIN")):
        return "number"
    if any(k in var_upper for k in ("ENABLE", "DISABLE", "AUTO", "VERBOSE")):
        return "boolean"
    return "text"


# ── YAML Generator ───────────────────────────────────────────────────────────

def generate_yaml(ext_id: str, ext_dir: Path) -> str:
    """Generate the spark.dev/v1 YAML spec for an extension."""

    ext_type = TYPE_MAP.get(ext_id, "tool")
    category = CATEGORY_MAP.get(ext_type, "Other")
    icon, icon_color = ICON_MAP.get(ext_id, ("extension", "#a7c8ff"))
    name = display_name(ext_id)

    # Read metadata
    plugin = read_json(ext_dir / "openclaw.plugin.json")
    pkg = read_json(ext_dir / "package.json")

    version = pkg.get("version", "1.0.0")
    description = pkg.get("description", f"Spark {name} connector")
    description = description.replace('"', "'")

    # Auth
    auth_type, auth_flow = AUTH_MAP.get(ext_id, ("api_key", "automatic"))

    # Ingestion
    ing_enabled, ing_mode, ing_desc = INGESTION_MAP.get(ext_type, ("false", "none", ""))

    # Override ingestion for channels with known push/pull
    if ext_type == "channel":
        ing_enabled = "true"
        ing_mode = "push"
        ing_desc = f"Receives messages from {name}"

    # Config from plugin.json schema
    config_schema = plugin.get("configSchema", {}).get("properties", {})
    # Config from env vars
    env_vars = scan_env_vars(ext_dir)

    # Tools from plugin.json
    plugin_tools = plugin.get("tools", [])

    # Source files for code section
    source_files = scan_source_files(ext_dir)

    # Channels from plugin.json
    channels = plugin.get("channels", [])

    # Dependencies from package.json
    deps = list(pkg.get("dependencies", {}).keys())

    # ── Build YAML ──

    lines = []
    w = lines.append

    w(f"# Spark Connector Spec — {name}")
    w(f"# Generated from OpenClaw extension: {ext_id}")
    w(f"apiVersion: spark.dev/v1")
    w(f"kind: Connector")
    w(f"")
    w(f"# ── METADATA")
    w(f"metadata:")
    w(f"  id: {ext_id}")
    w(f'  name: "{name}"')
    w(f"  type: {ext_type}")
    w(f'  version: "{version}"')
    w(f"  source: spark")
    w(f'  publisher: "Spark"')
    w(f"  maintainers:")
    w(f'    - name: "Spark Team"')
    w(f'      email: "spark@kite.ai"')
    w(f'  category: "{category}"')

    # Tags
    tags = list(set(plugin.get("tags", []) + [ext_type.title()]))
    if not tags:
        tags = [ext_type.title()]
    tag_str = ", ".join(f'"{t}"' for t in tags[:5])
    w(f"  tags: [{tag_str}]")

    w(f"  icon: {icon}")
    w(f'  iconColor: "{icon_color}"')

    # Descriptions
    w(f"  description: >")
    w(f"    {description}")
    w(f"  license: MIT")
    w(f"  homepage: https://github.com/kite-production/spark")
    w(f"  repository: https://github.com/kite-production/spark-connectors")
    w(f"")

    # ── SPEC
    w(f"# ── SPEC")
    w(f"spec:")

    # Runtime
    w(f"  runtime:")
    w(f"    type: docker")
    w(f"    language: typescript")
    if deps:
        framework = deps[0].split("/")[-1] if "/" in deps[0] else deps[0]
        w(f'    framework: "{framework}"')
    w(f"    docker:")
    w(f"      image: spark/connector-{ext_id}")
    w(f'      tag: "{version}"')
    grpc_port = 50070 + hash(ext_id) % 100
    w(f"      ports:")
    w(f"        grpc: {grpc_port}")

    # Auth
    w(f"")
    w(f"  auth:")
    w(f"    type: {auth_type}")
    w(f"    flow: {auth_flow}")

    # Ingestion
    w(f"")
    w(f"  ingestion:")
    w(f"    enabled: {ing_enabled}")
    w(f"    mode: {ing_mode}")
    w(f'    description: "{ing_desc}"')

    # Capabilities (for channels)
    if ext_type == "channel":
        w(f"")
        w(f"  capabilities:")
        w(f"    supports_text: true")
        w(f"    supports_images: true")
        w(f"    supports_audio: {str(ext_id in ('whatsapp', 'telegram', 'discord', 'slack')).lower()}")
        w(f"    supports_documents: {str(ext_id in ('whatsapp', 'telegram', 'slack', 'discord', 'msteams')).lower()}")
        w(f"    supports_threads: {str(ext_id in ('slack', 'discord', 'msteams', 'mattermost', 'matrix')).lower()}")
        w(f"    supports_reactions: {str(ext_id in ('slack', 'discord', 'telegram', 'whatsapp', 'msteams')).lower()}")
        w(f"    supports_reply: true")
        w(f"    supports_edit: {str(ext_id in ('slack', 'discord', 'telegram', 'whatsapp', 'msteams', 'matrix')).lower()}")
        w(f"    supports_typing_indicator: {str(ext_id in ('whatsapp', 'telegram', 'slack', 'discord', 'matrix')).lower()}")
        w(f"    supports_groups: {str(ext_id in ('whatsapp', 'telegram', 'slack', 'discord', 'msteams', 'matrix', 'googlechat', 'line', 'mattermost')).lower()}")
        w(f"    max_message_length: 4096")
        w(f"    supported_media_types:")
        w(f'      - "image/jpeg"')
        w(f'      - "image/png"')

    # Config
    w(f"")
    w(f"  config:")
    config_items = []

    # From plugin.json configSchema
    for key, val in config_schema.items():
        desc = val.get("description", key)
        ptype = "text"
        if "key" in key.lower() or "secret" in key.lower() or "token" in key.lower():
            ptype = "secret"
        elif val.get("type") == "boolean":
            ptype = "boolean"
        elif val.get("type") == "number":
            ptype = "number"
        group = "connection"
        if any(k in key.lower() for k in ("timeout", "retry", "max", "min")):
            group = "advanced"
        config_items.append((key, key, ptype, desc, group))

    # From env vars (if no config schema)
    if not config_items and env_vars:
        for var in env_vars[:10]:
            lvar = var.lower()
            ptype = config_type_from_env(var)
            group = "connection" if any(k in lvar for k in ("url", "host", "port", "key", "token")) else "advanced"
            config_items.append((lvar, var, ptype, f"Environment variable: {var}", group))

    if config_items:
        for name_key, display_key, ptype, desc, group in config_items:
            w(f'    - name: "{name_key}"')
            w(f'      display: "{display_key}"')
            w(f"      type: {ptype}")
            w(f"      required: {str(ptype == 'secret').lower()}")
            w(f'      description: "{yaml_str(desc)}"')
            w(f"      group: {group}")
    else:
        w(f"    []")

    # Tools
    w(f"")
    w(f"  tools:")
    if plugin_tools:
        for tool_name in plugin_tools:
            w(f'    - name: "{tool_name}"')
            w(f"      method: POST")
            w(f'      description: "{tool_name} operation"')
            w(f"      category: action")
            w(f"      args: []")
            w(f"      returns:")
            w(f"        type: object")
            w(f'        description: "Operation result"')
    else:
        # Generate default tools based on type
        if ext_type == "channel":
            w(f'    - name: "{ext_id}-send-message"')
            w(f"      method: POST")
            w(f'      description: "Send a message via {name}"')
            w(f"      category: messaging")
            w(f"      args:")
            w(f'        - name: to')
            w(f"          type: string")
            w(f'          description: "Recipient ID or channel"')
            w(f"          required: true")
            w(f'        - name: text')
            w(f"          type: string")
            w(f'          description: "Message content"')
            w(f"          required: true")
            w(f"      returns:")
            w(f"        type: object")
            w(f"        properties:")
            w(f"          message_id: {{ type: string }}")
            w(f'    - name: "{ext_id}-list-messages"')
            w(f"      method: GET")
            w(f'      description: "List recent messages from {name}"')
            w(f"      category: query")
            w(f"      args:")
            w(f'        - name: chat_id')
            w(f"          type: string")
            w(f"          required: true")
            w(f'        - name: limit')
            w(f"          type: number")
            w(f"          required: false")
            w(f'          default: "50"')
            w(f"      returns:")
            w(f"        type: array")
            w(f'        description: "List of messages"')
        elif ext_type == "search":
            w(f'    - name: "{ext_id}-search"')
            w(f"      method: POST")
            w(f'      description: "Search via {name}"')
            w(f"      category: query")
            w(f"      args:")
            w(f'        - name: query')
            w(f"          type: string")
            w(f"          required: true")
            w(f'        - name: limit')
            w(f"          type: number")
            w(f"          required: false")
            w(f'          default: "10"')
            w(f"      returns:")
            w(f"        type: array")
            w(f'        description: "Search results"')
        else:
            w(f"    []")

    # Events (only for channels)
    w(f"")
    w(f"  events:")
    if ext_type == "channel":
        w(f"    - name: message.received")
        w(f'      description: "New message received"')
        w(f"      payload:")
        w(f"        message_id: string")
        w(f"        chat_id: string")
        w(f"        sender: string")
        w(f"        text: string")
        w(f"        timestamp: string")
        w(f"    - name: connection.status")
        w(f'      description: "Connection state changed"')
        w(f"      payload:")
        w(f"        status: string")
        w(f"        reason: string")
    else:
        w(f"    []")

    # Health & Testing
    w(f"")
    w(f"  health:")
    w(f'    endpoint: "/health"')
    w(f"    interval: 15s")
    w(f"    timeout: 5s")
    w(f"    unhealthy_threshold: 3")
    w(f"    test:")
    w(f'      endpoint: "/test"')
    w(f'      description: "Verify connectivity to {name}"')
    w(f"      timeout: 10s")
    w(f"      expected_status: 200")

    # Dependencies
    w(f"")
    w(f"  dependencies:")
    w(f'    services: ["nats", "control-plane"]')
    if ext_type == "channel":
        w(f'    optional: ["minio"]')

    # Versions
    w(f"")
    w(f"  versions:")
    w(f'    - version: "{version}"')
    w(f'      date: "{TODAY}"')
    w(f"      breaking: false")
    w(f"      changes:")
    w(f'        - "Initial Spark marketplace release"')
    w(f'        - "Adapted from OpenClaw {ext_id} extension"')

    # Setup
    w(f"")
    w(f"  setup: |")
    w(f"    ## {name} Connector Setup")
    w(f"    1. Install the connector from the Spark Marketplace")
    w(f"    2. Create an instance in your workspace")
    w(f"    3. Configure the required parameters")
    w(f"    4. Click 'Test Connection' to verify")
    w(f"    5. Enable the connector for your agents")

    # Code section (file listing — not full content to keep YAML manageable)
    w(f"")
    w(f"  code:")
    if source_files:
        entry = source_files[0]["path"] if source_files else "index.ts"
        w(f'    entrypoint: "{entry}"')
        w(f"    files:")
        for sf in source_files[:15]:  # Limit to 15 files
            w(f'      - path: "{sf["path"]}"')
            w(f'        language: {sf["language"]}')
            w(f"        size: {sf['size']}")
    else:
        w(f'    entrypoint: "index.ts"')
        w(f"    files: []")

    return "\n".join(lines) + "\n"


# ── Main ─────────────────────────────────────────────────────────────────────

def main():
    ext_dir = OPENCLAW_DIR / "extensions"
    if not ext_dir.exists():
        print(f"❌ Extensions directory not found at {ext_dir}")
        sys.exit(1)

    print()
    print("═" * 60)
    print("  OpenClaw → Spark Connector Conversion (Enhanced v2)")
    print("═" * 60)
    print()

    generated = 0
    skipped = 0

    for ext_path in sorted(ext_dir.iterdir()):
        if not ext_path.is_dir():
            continue

        ext_id = ext_path.name

        if ext_id in SKIP_IDS:
            skipped += 1
            continue

        if ext_id not in TYPE_MAP:
            skipped += 1
            continue

        # Generate YAML
        yaml_content = generate_yaml(ext_id, ext_path)

        # Write output
        out_dir = OUTPUT_DIR / ext_id
        out_dir.mkdir(parents=True, exist_ok=True)
        out_file = out_dir / "connector.yaml"
        out_file.write_text(yaml_content)

        ext_type = TYPE_MAP[ext_id]
        print(f"  ✓ {ext_id:25s} ({ext_type:8s}) → {out_file}")
        generated += 1

    print()
    print("═" * 60)
    print(f"  Generated: {generated} | Skipped: {skipped}")
    print("═" * 60)


if __name__ == "__main__":
    main()
