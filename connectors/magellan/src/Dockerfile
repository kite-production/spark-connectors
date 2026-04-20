# Multi-stage build for the magellan on-demand connector. Tool-only
# binary — webhook/NATS ingestion for inbound Venus messages has been
# moved to the `venus-bridge` service in the Spark repo.
FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY connectors/magellan/src/ ./connectors/magellan/src/
WORKDIR /app/connectors/magellan/src

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/connector ./cmd/connector

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/connector /bin/connector
ENTRYPOINT ["/bin/connector"]
