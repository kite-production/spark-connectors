// connector-msteams is the Spark connector for Microsoft Teams.
// It receives Bot Framework activities via webhook and sends messages
// through the Microsoft Graph API.
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
	"github.com/kite-production/spark/services/connector-msteams/internal/msteams"
	"github.com/kite-production/spark/services/connector-msteams/internal/normalizer"
	"github.com/kite-production/spark/services/connector-msteams/internal/service"
	"github.com/kite-production/spark/services/connector-msteams/internal/webhook"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

func main() {
	// ── Environment ──────────────────────────────────────────────────────────
	appID := os.Getenv("MSTEAMS_APP_ID")
	appPassword := os.Getenv("MSTEAMS_APP_PASSWORD")
	tenantID := os.Getenv("MSTEAMS_TENANT_ID")
	if appID == "" || appPassword == "" || tenantID == "" {
		log.Fatal("MSTEAMS_APP_ID, MSTEAMS_APP_PASSWORD, and MSTEAMS_TENANT_ID must be set")
	}

	grpcPort := envOr("SPARK_GRPC_PORT", "50069")
	natsURL := envOr("SPARK_NATS_URL", "nats://localhost:4222")
	cpAddress := envOr("SPARK_CONTROL_PLANE_ADDRESS", "control-plane:50050")
	webhookPort := envOr("MSTEAMS_WEBHOOK_PORT", "8092")

	log.Printf("[msteams] starting connector-msteams")
	log.Printf("[msteams] gRPC=%s NATS=%s CP=%s webhook=:%s",
		grpcPort, natsURL, cpAddress, webhookPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── MS Teams Client ─────────────────────────────────────────────────────
	teamsClient := msteams.NewClient(appID, appPassword, tenantID)

	// Verify credentials by fetching an initial token.
	if _, err := teamsClient.GetAccessToken(); err != nil {
		log.Printf("[msteams] WARNING: initial token fetch failed: %v", err)
		log.Println("[msteams] continuing — will retry on first message send")
	} else {
		log.Println("[msteams] Azure AD token acquired successfully")
	}

	// ── BaseConnector ────────────────────────────────────────────────────────
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "msteams",
		GRPCAddress:         fmt.Sprintf("connector-msteams:%s", grpcPort),
		GRPCPort:            grpcPort,
		NATSUrl:             natsURL,
		ControlPlaneAddress: cpAddress,
		Capabilities: &connectorpb.ConnectorCapabilities{
			SupportsThreads:     true,
			SupportsReactions:   true,
			SupportsEdit:        true,
			SupportsReply:       true,
			SupportsAttachments: true,
			MaxMessageLength:    28000,
		},
	})

	// ── Normalizer ───────────────────────────────────────────────────────────
	norm := normalizer.New("msteams")

	// ── Service (gRPC) ───────────────────────────────────────────────────────
	svc := service.New(base, teamsClient)

	// ── Start BaseConnector (NATS + gRPC + control-plane registration) ──────
	if err := base.Start(ctx, svc); err != nil {
		log.Fatalf("[msteams] failed to start base connector: %v", err)
	}

	// ── gRPC listener ────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("[msteams] failed to listen on :%s: %v", grpcPort, err)
	}
	go func() {
		log.Printf("[msteams] gRPC server listening on :%s", grpcPort)
		if err := base.GRPCServer.Serve(lis); err != nil {
			log.Printf("[msteams] gRPC server stopped: %v", err)
		}
	}()

	// ── Webhook listener (receives Bot Framework activities from Teams) ─────
	whHandler := webhook.New(base, norm)
	whMux := http.NewServeMux()
	whMux.Handle("POST /api/messages", whHandler)
	whMux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"connector-msteams"}`))
	})

	whServer := &http.Server{
		Addr:              fmt.Sprintf(":%s", webhookPort),
		Handler:           whMux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("[msteams] webhook listener on :%s/api/messages", webhookPort)
		if err := whServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[msteams] webhook server stopped: %v", err)
		}
	}()

	// ── Wait for shutdown ────────────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("[msteams] received %s, shutting down...", sig)

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	_ = whServer.Shutdown(shutdownCtx)
	base.Stop(shutdownCtx)
	log.Println("[msteams] stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
