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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/kite-production/spark/services/connector-googlechat/internal/chatapi"
	"github.com/kite-production/spark/services/connector-googlechat/internal/normalizer"
	"github.com/kite-production/spark/services/connector-googlechat/internal/service"
	"github.com/kite-production/spark/services/connector-googlechat/internal/webhook"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

func main() {
	credJSON := os.Getenv("GOOGLE_CHAT_CREDENTIALS_JSON")
	if credJSON == "" {
		log.Fatal("GOOGLE_CHAT_CREDENTIALS_JSON must be set")
	}

	projectNumber := os.Getenv("GOOGLE_CHAT_PROJECT_NUMBER")

	grpcPort := os.Getenv("SPARK_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50064"
	}
	metricsPort := os.Getenv("SPARK_METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "9090"
	}
	webhookPort := os.Getenv("SPARK_WEBHOOK_PORT")
	if webhookPort == "" {
		webhookPort = "8065"
	}
	natsURL := os.Getenv("SPARK_NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	cpAddress := os.Getenv("SPARK_CONTROL_PLANE_ADDRESS")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Google Chat API client.
	chatAPI, err := chatapi.NewClient(ctx, credJSON)
	if err != nil {
		log.Fatalf("failed to create Google Chat API client: %v", err)
	}

	// Create BaseConnector.
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "googlechat",
		GRPCAddress:         fmt.Sprintf(":%s", grpcPort),
		GRPCPort:            grpcPort,
		NATSUrl:             natsURL,
		ControlPlaneAddress: cpAddress,
	})

	// Create normalizer.
	norm := normalizer.New("googlechat")

	// Create Prometheus metrics.
	reg := prometheus.DefaultRegisterer
	metrics := service.NewMetrics(reg)

	// Create service.
	svc := service.New(base, norm, chatAPI, metrics)

	// Start BaseConnector (NATS, gRPC server, control-plane registration).
	if err := base.Start(ctx, svc); err != nil {
		log.Fatalf("failed to start connector: %v", err)
	}

	// Start gRPC listener.
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("failed to listen on :%s: %v", grpcPort, err)
	}
	go func() {
		log.Printf("connector-googlechat gRPC server listening on :%s", grpcPort)
		if err := base.GRPCServer.Serve(lis); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Start Prometheus metrics HTTP server.
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if base.IsHealthy() {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"status":"ok"}`)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, `{"status":"unavailable"}`)
			}
		})
		log.Printf("connector-googlechat metrics/health on :%s", metricsPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", metricsPort), mux); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()

	// Start webhook HTTP server for Google Chat events.
	webhookHandler := webhook.New(projectNumber, svc.HandleEvent)
	webhookMux := http.NewServeMux()
	webhookMux.Handle("/webhook", webhookHandler)
	webhookServer := &http.Server{
		Addr:              fmt.Sprintf(":%s", webhookPort),
		Handler:           webhookMux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		svc.SetWebhookUp(true)
		log.Printf("connector-googlechat webhook listening on :%s", webhookPort)
		if err := webhookServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			svc.SetWebhookUp(false)
			log.Printf("webhook server error: %v", err)
		}
	}()

	// Wait for SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("received %s, shutting down", sig)

	// Graceful shutdown: stop webhook, cancel context, stop base.
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	svc.SetWebhookUp(false)
	if err := webhookServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("webhook server shutdown error: %v", err)
	}

	base.Stop(shutdownCtx)

	log.Println("connector-googlechat stopped")
}
