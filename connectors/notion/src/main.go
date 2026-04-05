// Connector-notion root package stub. The full implementation is in cmd/server/.
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
		port = "50066"
	}

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"ok"}`)
		})
		metricsPort := os.Getenv("SPARK_METRICS_PORT")
		if metricsPort == "" {
			metricsPort = "9090"
		}
		log.Printf("connector-notion health endpoint on :%s/healthz", metricsPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%s", metricsPort), mux); err != nil {
			log.Printf("health endpoint error: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()
	log.Printf("connector-notion listening on :%s", port)

	select {}
}
