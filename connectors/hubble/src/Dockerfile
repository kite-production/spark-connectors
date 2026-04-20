# Multi-stage build for the hubble on-demand connector. The final
# image is a minimal distroless that runs `/bin/connector execute ...`
# as its entrypoint — each Spark tool call spawns a fresh container
# from this image, runs one tool, and exits.
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Copy source. No go mod download needed — hubble is stdlib-only so
# the go.sum is empty and there are no modules to fetch.
COPY connectors/hubble/src/ ./connectors/hubble/src/
WORKDIR /app/connectors/hubble/src

# Build a static binary so the distroless image works.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/connector ./cmd/connector

# Runtime image — nonroot distroless. No shell, no package manager,
# minimal attack surface. Good fit for per-call ephemeral containers.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/connector /bin/connector
ENTRYPOINT ["/bin/connector"]
