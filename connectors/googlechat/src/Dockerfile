FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy local dependency modules first for layer caching.
COPY contracts/gen/go/ ./contracts/gen/go/
COPY pkg/ ./pkg/
COPY services/cross-service/ ./services/cross-service/

# Copy service dependency manifests and download deps.
COPY services/connector-googlechat/go.mod services/connector-googlechat/go.sum ./services/connector-googlechat/
WORKDIR /app/services/connector-googlechat
RUN go mod download

# Copy service source and build.
COPY services/connector-googlechat/ ./
RUN CGO_ENABLED=0 go build -o /bin/service ./cmd/server

# Install grpc_health_probe for container health checks.
RUN GOBIN=/usr/local/bin go install github.com/grpc-ecosystem/grpc-health-probe@latest

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/service /bin/service
COPY --from=builder /usr/local/bin/grpc-health-probe /bin/grpc_health_probe
ENTRYPOINT ["/bin/service"]
