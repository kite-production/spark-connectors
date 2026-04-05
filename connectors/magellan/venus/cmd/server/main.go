// Venus is a lightweight test messaging service for the Spark platform.
// It provides channels, messages, webhooks, and WebSocket streams — all in-memory.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kite-production/spark/services/venus/internal/api"
	"github.com/kite-production/spark/services/venus/internal/store"
	"github.com/kite-production/spark/services/venus/internal/webhook"
)

func main() {
	// Health check mode for Docker healthcheck.
	if len(os.Args) > 1 && os.Args[1] == "--health-check" {
		port := os.Getenv("VENUS_HTTP_PORT")
		if port == "" {
			port = "8090"
		}
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/health", port))
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	port := os.Getenv("VENUS_HTTP_PORT")
	if port == "" {
		port = "8090"
	}

	apiKey := os.Getenv("VENUS_API_KEY")
	if apiKey == "" {
		apiKey = "venus-test-key-2026"
	}

	log.Printf("[venus] starting Venus messaging service on :%s", port)
	log.Printf("[venus] API key authentication: enabled (key prefix: %s...)", apiKey[:8])

	// Initialize in-memory store (auto-creates #general channel).
	s := store.New()

	// Initialize webhook delivery (subscribes to new messages).
	_ = webhook.New(s)

	// Initialize HTTP API handlers.
	handler := api.New(s)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Initialize WebSocket hub (subscribes to new messages).
	wsHub := api.NewWSHub(s)
	wsHub.RegisterRoutes(mux)

	// Chain: CORS → API Key auth → handlers.
	// Health endpoint is exempt from auth.
	corsHandler := corsMiddleware(apiKeyMiddleware(mux, apiKey))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      corsHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine.
	go func() {
		log.Printf("[venus] HTTP server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[venus] server failed: %v", err)
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Printf("[venus] received %s, shutting down...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[venus] shutdown error: %v", err)
	}
	log.Println("[venus] stopped")
}

// apiKeyMiddleware validates the Authorization header for API endpoints.
// Health and WebSocket endpoints are exempt.
func apiKeyMiddleware(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt health check and WebSocket endpoints from auth.
		if r.URL.Path == "/health" || r.URL.Path == "/api/status" {
			next.ServeHTTP(w, r)
			return
		}
		// Check for API key in Authorization header.
		auth := r.Header.Get("Authorization")
		if auth == "" {
			auth = r.URL.Query().Get("api_key") // Fallback: query param
		}
		if auth != "Bearer "+apiKey && auth != apiKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized","message":"Valid API key required. Set Authorization: Bearer <key>"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for browser access.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
