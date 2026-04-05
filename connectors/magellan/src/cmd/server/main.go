// Magellan is the Spark connector for the Venus test messaging service.
// Named after NASA's Venus-mapping spacecraft (1989-1994).
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"github.com/kite-production/spark/services/connector-magellan/internal/normalizer"
	"github.com/kite-production/spark/services/connector-magellan/internal/service"
	"github.com/kite-production/spark/services/connector-magellan/internal/venus"
	"github.com/kite-production/spark/services/connector-magellan/internal/webhook"
)

func main() {
	// ── Environment ──────────────────────────────────────────────────────────
	grpcPort := envOr("SPARK_GRPC_PORT", "50066")
	natsURL := envOr("SPARK_NATS_URL", "nats://localhost:4222")
	cpAddress := envOr("SPARK_CONTROL_PLANE_ADDRESS", "control-plane:50050")
	venusURL := envOr("VENUS_URL", "http://localhost:8090")
	venusAPIKey := envOr("VENUS_API_KEY", "venus-test-key-2026")
	webhookPort := envOr("VENUS_WEBHOOK_PORT", "8091")

	log.Printf("[magellan] starting connector-magellan (Venus bridge)")
	log.Printf("[magellan] gRPC=%s NATS=%s CP=%s Venus=%s webhook=:%s",
		grpcPort, natsURL, cpAddress, venusURL, webhookPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Venus Client ─────────────────────────────────────────────────────────
	vc := venus.NewClient(venusURL, venusAPIKey)

	// Wait for Venus to be available (retry with backoff).
	log.Println("[magellan] waiting for Venus...")
	for i := 0; i < 30; i++ {
		if err := vc.Health(); err == nil {
			log.Println("[magellan] Venus is reachable")
			break
		}
		if i == 29 {
			log.Println("[magellan] WARNING: Venus not reachable after 30 attempts, continuing anyway")
		}
		time.Sleep(2 * time.Second)
	}

	// ── BaseConnector ────────────────────────────────────────────────────────
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "magellan",
		GRPCAddress:         fmt.Sprintf("connector-magellan:%s", grpcPort),
		GRPCPort:            grpcPort,
		NATSUrl:             natsURL,
		ControlPlaneAddress: cpAddress,
		Capabilities: &connectorpb.ConnectorCapabilities{
			SupportsThreads:     true,
			SupportsReactions:   false,
			SupportsEdit:        false,
			SupportsUnsend:      false,
			SupportsReply:       true,
			SupportsAttachments: false,
			SupportsImages:      false,
			MaxMessageLength:    10000,
		},
	})

	// ── Normalizer ───────────────────────────────────────────────────────────
	norm := normalizer.New("magellan")

	// ── Service (gRPC) ───────────────────────────────────────────────────────
	svc := service.New(base, vc)

	// ── Start BaseConnector (NATS + gRPC + control-plane registration) ──────
	if err := base.Start(ctx, svc); err != nil {
		log.Fatalf("[magellan] failed to start base connector: %v", err)
	}

	// ── gRPC listener ────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("[magellan] failed to listen on :%s: %v", grpcPort, err)
	}
	go func() {
		log.Printf("[magellan] gRPC server listening on :%s", grpcPort)
		if err := base.GRPCServer.Serve(lis); err != nil {
			log.Printf("[magellan] gRPC server stopped: %v", err)
		}
	}()

	// ── Webhook listener (receives messages from Venus) ─────────────────────
	whHandler := webhook.New(base, norm)
	whMux := http.NewServeMux()
	whMux.Handle("POST /webhook", whHandler)
	whMux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"connector-magellan"}`))
	})

	whServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", webhookPort),
		Handler: whMux,
	}
	go func() {
		log.Printf("[magellan] webhook listener on :%s", webhookPort)
		if err := whServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[magellan] webhook server stopped: %v", err)
		}
	}()

	// ── Register webhook with Venus ─────────────────────────────────────────
	webhookURL := fmt.Sprintf("http://connector-magellan:%s/webhook", webhookPort)
	wh, err := vc.RegisterWebhook(webhookURL)
	if err != nil {
		log.Printf("[magellan] WARNING: failed to register webhook with Venus: %v", err)
		log.Println("[magellan] messages will not be received until webhook is registered")
	} else {
		log.Printf("[magellan] registered webhook %s → %s", wh.ID, webhookURL)
	}

	// ── Wait for shutdown ────────────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("[magellan] received %s, shutting down...", sig)

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	_ = whServer.Shutdown(shutdownCtx)
	base.Stop(shutdownCtx)
	log.Println("[magellan] stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
