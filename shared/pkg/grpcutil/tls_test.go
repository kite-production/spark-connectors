package grpcutil

import (
	"os"
	"testing"
)

func TestLoadServerTLS_MissingFiles(t *testing.T) {
	_, err := LoadServerTLS("/nonexistent/cert.pem", "/nonexistent/key.pem", "/nonexistent/ca.pem")
	if err == nil {
		t.Fatal("expected error for missing cert files")
	}
}

func TestLoadClientTLS_MissingFiles(t *testing.T) {
	_, err := LoadClientTLS("/nonexistent/cert.pem", "/nonexistent/key.pem", "/nonexistent/ca.pem")
	if err == nil {
		t.Fatal("expected error for missing cert files")
	}
}

func TestTLSFromEnv_NotConfigured(t *testing.T) {
	os.Unsetenv("SPARK_TLS_CERT")
	serverOpt, clientOpt, err := TLSFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if serverOpt != nil {
		t.Fatal("expected nil server option when TLS not configured")
	}
	if clientOpt != nil {
		t.Fatal("expected nil client option when TLS not configured")
	}
}
