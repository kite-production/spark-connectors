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
	"github.com/kite-production/spark/services/connector-browser/internal/browser"
	"github.com/kite-production/spark/services/connector-browser/internal/service"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

func main() {
	chromePath := os.Getenv("CHROME_PATH")
	headless := os.Getenv("CHROME_HEADLESS") != "false"

	grpcPort := os.Getenv("SPARK_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50071"
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

	// Create browser instance.
	log.Printf("starting Chrome (headless=%t, path=%q)", headless, chromePath)
	b, err := browser.New(headless, chromePath)
	if err != nil {
		log.Fatalf("failed to start browser: %v", err)
	}
	defer b.Close()
	log.Printf("Chrome browser started successfully")

	// Create BaseConnector.
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "browser",
		GRPCAddress:         fmt.Sprintf(":%s", grpcPort),
		GRPCPort:            grpcPort,
		NATSUrl:             natsURL,
		ControlPlaneAddress: cpAddress,
	})

	// Create Prometheus metrics.
	reg := prometheus.DefaultRegisterer
	metrics := service.NewMetrics(reg)

	// Create service.
	svc := service.New(base, b, metrics)

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
		log.Printf("connector-browser gRPC server listening on :%s", grpcPort)
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
			if base.IsHealthy() && b.Alive() {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"status":"ok"}`)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, `{"status":"unavailable"}`)
			}
		})
		log.Printf("connector-browser metrics/health on :%s", metricsPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", metricsPort), mux); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()

	// No message listener — this is a tool connector, not a channel.

	// Wait for SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("received %s, shutting down", sig)

	// Graceful shutdown.
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	base.Stop(shutdownCtx)
	b.Close()

	log.Println("connector-browser stopped")
}
