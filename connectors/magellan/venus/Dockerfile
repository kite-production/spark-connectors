FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go module files first for better caching.
COPY services/venus/go.mod services/venus/go.sum ./services/venus/
WORKDIR /app/services/venus
RUN go mod download

# Copy source and build.
COPY services/venus/ ./
RUN CGO_ENABLED=0 go build -o /bin/venus ./cmd/server

# Final image.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/venus /bin/venus
EXPOSE 8090
ENTRYPOINT ["/bin/venus"]
