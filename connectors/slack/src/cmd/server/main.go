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
	"github.com/kite-production/spark/services/connector-slack/internal/normalizer"
	"github.com/kite-production/spark/services/connector-slack/internal/service"
	sm "github.com/kite-production/spark/services/connector-slack/internal/socketmode"
	"github.com/kite-production/spark/services/connector-slack/internal/usercache"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
)

func main() {
	appToken := os.Getenv("SLACK_APP_TOKEN")
	botToken := os.Getenv("SLACK_BOT_TOKEN")
	if appToken == "" || botToken == "" {
		log.Fatal("SLACK_APP_TOKEN and SLACK_BOT_TOKEN must be set")
	}

	grpcPort := os.Getenv("SPARK_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50061"
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
		ConnectorID:         "slack",
		GRPCAddress:         fmt.Sprintf(":%s", grpcPort),
		GRPCPort:            grpcPort,
		NATSUrl:             natsURL,
		ControlPlaneAddress: cpAddress,
	})

	// Create Socket Mode client.
	smClient := sm.New(appToken, botToken)

	// Create user cache backed by Slack API.
	cache := usercache.New(10*time.Minute, func(userID string) (string, error) {
		user, err := smClient.API().GetUserInfo(userID)
		if err != nil {
			return "", err
		}
		if user.Profile.DisplayName != "" {
			return user.Profile.DisplayName, nil
		}
		return user.RealName, nil
	})

	// Create normalizer.
	norm := normalizer.New("slack", cache)

	// Create Prometheus metrics.
	reg := prometheus.DefaultRegisterer
	metrics := service.NewMetrics(reg)

	// Create service.
	svc := service.New(base, norm, smClient.API(), metrics)

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
		log.Printf("connector-slack gRPC server listening on :%s", grpcPort)
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
		log.Printf("connector-slack metrics/health on :%s", metricsPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", metricsPort), mux); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()

	// Start Socket Mode listener in a goroutine.
	go func() {
		svc.SetWSConnected(true)
		err := smClient.Run(ctx, svc.HandleMessageEvent)
		svc.SetWSConnected(false)
		if err != nil && ctx.Err() == nil {
			log.Printf("socket mode error: %v", err)
		}
	}()

	// Wait for SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("received %s, shutting down", sig)

	// Graceful shutdown: cancel context (stops Socket Mode), then stop base
	// (deregisters from control-plane, drains NATS, stops gRPC).
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	base.Stop(shutdownCtx)

	log.Println("connector-slack stopped")
}
