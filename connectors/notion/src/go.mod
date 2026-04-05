module github.com/kite-production/spark/services/connector-notion

go 1.24.0

require (
	buf.build/gen/go/thekite/kite/protocolbuffers/go v0.0.0
	github.com/nats-io/nats.go v1.37.0
	github.com/prometheus/client_golang v1.22.0
	github.com/kite-production/spark/services/cross-service/connector v0.0.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/kite-production/spark/pkg v0.0.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/grpc v1.79.3 // indirect
)

replace (
	buf.build/gen/go/thekite/kite/protocolbuffers/go => ../../../shared/contracts/gen/go
	github.com/kite-production/spark/pkg => ../../../shared/pkg
	github.com/kite-production/spark/services/cross-service/connector => ../../../shared/cross-service/connector
)
