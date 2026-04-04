# Spark Connectors Marketplace

Official connector specifications for the [Spark AI Platform](https://github.com/kite-production/spark).

## Structure

Each connector has its own directory under `connectors/`:

```
connectors/
├── magellan/          # Venus test messaging bridge
│   └── connector.yaml
├── slack/             # Slack workspace integration
│   └── connector.yaml
├── gmail/             # Gmail thread management
│   └── connector.yaml
├── notion/            # Notion page sync
│   └── connector.yaml
├── github/            # GitHub repos, issues, PRs
│   └── connector.yaml
└── alpaca/            # Stock trading API
    └── connector.yaml
```

## Connector Spec Format

Each `connector.yaml` follows the `spark.dev/v1` spec:

```yaml
apiVersion: spark.dev/v1
kind: Connector
metadata:
  id: connector-id
  name: "Display Name"
  version: "1.0.0"
  publisher: "Publisher"
  category: "Communication"
  tags: ["tag1", "tag2"]
  icon: material_icon_name
  description: "Short description"
spec:
  docker:
    image: spark/connector-name
    tag: "1.0.0"
  auth:
    type: api_key | oauth2 | none
  capabilities:
    supports_threads: true
    # ...
  config:
    - name: param_name
      type: text | secret | boolean | select
      required: true
  tools:
    - name: tool-name
      method: GET | POST
      description: "What it does"
      args: [...]
  versions:
    - version: "1.0.0"
      changes: [...]
  setup: |
    Setup instructions...
```

## Installation

The Spark platform downloads connector specs from GitHub Releases:

```bash
# Download a specific connector spec
curl -L https://github.com/kite-production/spark-connectors/releases/download/magellan-v1.0.0/connector.yaml

# Or use the GitHub CLI
gh release download magellan-v1.0.0 --repo kite-production/spark-connectors
```

## Contributing

1. Fork this repository
2. Create a new directory under `connectors/`
3. Add your `connector.yaml` following the spec format
4. Open a pull request

## License

MIT
