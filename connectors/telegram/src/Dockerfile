FROM golang:1.24-alpine AS builder
WORKDIR /app

# Copy shared dependencies
COPY shared/ ./shared/

# Copy connector source
COPY connectors/telegram/src/ ./connectors/telegram/src/

WORKDIR /app/connectors/telegram/src
RUN go mod download
RUN CGO_ENABLED=0 go build -o /bin/connector ./cmd/server

# Install grpc_health_probe
RUN GOBIN=/usr/local/bin go install github.com/grpc-ecosystem/grpc-health-probe@latest

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/connector /bin/connector
COPY --from=builder /usr/local/bin/grpc-health-probe /bin/grpc_health_probe
ENTRYPOINT ["/bin/connector"]
