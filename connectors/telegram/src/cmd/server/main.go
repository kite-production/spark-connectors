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
	"github.com/kite-production/spark/services/connector-telegram/internal/normalizer"
	"github.com/kite-production/spark/services/connector-telegram/internal/service"
	"github.com/kite-production/spark/services/connector-telegram/internal/telegram"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN must be set")
	}

	grpcPort := os.Getenv("SPARK_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50067"
	}
	metricsPort := os.Getenv("SPARK_METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "9090"
	}
	natsURL := os.Getenv("SPARK_NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	cpAddress := os.Getenv("SPARK_CONTROL_PLANE_ADDRESS")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create BaseConnector.
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "telegram",
		GRPCAddress:         fmt.Sprintf(":%s", grpcPort),
		GRPCPort:            grpcPort,
		NATSUrl:             natsURL,
		ControlPlaneAddress: cpAddress,
	})

	// Create Telegram bot client.
	tgClient, err := telegram.NewClient(botToken)
	if err != nil {
		log.Fatalf("failed to create Telegram client: %v", err)
	}
	log.Printf("authorized as Telegram bot @%s", tgClient.BotUsername())

	// Create normalizer.
	norm := normalizer.New("telegram")

	// Create Prometheus metrics.
	reg := prometheus.DefaultRegisterer
	metrics := service.NewMetrics(reg)

	// Create service.
	svc := service.New(base, norm, tgClient, metrics)

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
		log.Printf("connector-telegram gRPC server listening on :%s", grpcPort)
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
		log.Printf("connector-telegram metrics/health on :%s", metricsPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", metricsPort), mux); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()

	// Start Telegram long-polling in a goroutine.
	go func() {
		svc.SetPolling(true)
		err := tgClient.StartPolling(ctx, svc.HandleUpdate)
		svc.SetPolling(false)
		if err != nil && ctx.Err() == nil {
			log.Printf("telegram polling error: %v", err)
		}
	}()

	// Wait for SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("received %s, shutting down", sig)

	// Graceful shutdown: cancel context (stops polling), then stop base
	// (deregisters from control-plane, drains NATS, stops gRPC).
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	base.Stop(shutdownCtx)

	log.Println("connector-telegram stopped")
}
