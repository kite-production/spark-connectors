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
	gmailclient "github.com/kite-production/spark/services/connector-gmail/internal/gmail"
	"github.com/kite-production/spark/services/connector-gmail/internal/normalizer"
	"github.com/kite-production/spark/services/connector-gmail/internal/service"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

func main() {
	credJSON := os.Getenv("GMAIL_CREDENTIALS_JSON")
	if credJSON == "" {
		log.Fatal("GMAIL_CREDENTIALS_JSON must be set")
	}

	userID := os.Getenv("GMAIL_USER_ID")
	if userID == "" {
		userID = "me"
	}

	accountID := os.Getenv("GMAIL_ACCOUNT_ID")
	if accountID == "" {
		accountID = userID
	}

	grpcPort := os.Getenv("SPARK_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50063"
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

	// Create Gmail API client.
	gmailAPI, err := gmailclient.NewClient(ctx, credJSON)
	if err != nil {
		log.Fatalf("failed to create Gmail API client: %v", err)
	}

	// Create BaseConnector.
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "gmail",
		GRPCAddress:         fmt.Sprintf(":%s", grpcPort),
		GRPCPort:            grpcPort,
		NATSUrl:             natsURL,
		ControlPlaneAddress: cpAddress,
	})

	// Create normalizer.
	norm := normalizer.New("gmail", accountID)

	// Create Prometheus metrics.
	reg := prometheus.DefaultRegisterer
	metrics := service.NewMetrics(reg)

	// Create service.
	svc := service.New(base, norm, gmailAPI, userID, metrics)

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
		log.Printf("connector-gmail gRPC server listening on :%s", grpcPort)
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
		log.Printf("connector-gmail metrics/health on :%s", metricsPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", metricsPort), mux); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()

	// Start Gmail poller in a goroutine.
	poller := gmailclient.NewPoller(gmailAPI, userID, 30*time.Second, svc.HandleMessage)
	go func() {
		svc.SetPolling(true)
		err := poller.Run(ctx)
		svc.SetPolling(false)
		if err != nil && ctx.Err() == nil {
			log.Printf("poller error: %v", err)
		}
	}()

	// Wait for SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("received %s, shutting down", sig)

	// Graceful shutdown: cancel context (stops poller), then stop base
	// (deregisters from control-plane, drains NATS, stops gRPC).
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	base.Stop(shutdownCtx)

	log.Println("connector-gmail stopped")
}
