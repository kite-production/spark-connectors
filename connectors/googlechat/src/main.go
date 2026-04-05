package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("SPARK_GRPC_PORT")
	if port == "" {
		port = "50065"
	}

	// Health check endpoint on a separate HTTP port.
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"ok"}`)
		})
		metricsPort := os.Getenv("SPARK_METRICS_PORT")
		if metricsPort == "" {
			metricsPort = "9090"
		}
		log.Printf("connector-googlechat health endpoint on :%s/healthz", metricsPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", metricsPort), mux); err != nil {
			log.Printf("health endpoint error: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()
	log.Printf("connector-googlechat listening on :%s", port)

	// TODO: register gRPC services and call Serve(lis)
	select {}
}
