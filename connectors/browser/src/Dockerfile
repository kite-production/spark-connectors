FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY shared/ ./shared/
COPY connectors/browser/src/ ./connectors/browser/src/
WORKDIR /app/connectors/browser/src
RUN go mod download
RUN CGO_ENABLED=0 go build -o /bin/connector ./cmd/server
RUN GOBIN=/usr/local/bin go install github.com/grpc-ecosystem/grpc-health-probe@latest

FROM alpine:3.21
RUN apk add --no-cache chromium
COPY --from=builder /bin/connector /bin/connector
COPY --from=builder /usr/local/bin/grpc-health-probe /bin/grpc_health_probe
ENV CHROME_PATH=/usr/bin/chromium-browser
ENTRYPOINT ["/bin/connector"]
