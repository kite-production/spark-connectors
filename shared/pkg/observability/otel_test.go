package observability

import (
	"context"
	"testing"
)

func TestSetup_noExporters(t *testing.T) {
	cfg := Config{
		ServiceName:    "test-service",
		TraceEnabled:   false,
		MetricsEnabled: false,
	}
	shutdown, err := Setup(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestSetup_metricsOnly(t *testing.T) {
	cfg := Config{
		ServiceName:    "test-service",
		MetricsEnabled: true,
	}
	shutdown, err := Setup(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestMeter(t *testing.T) {
	m := Meter("test")
	if m == nil {
		t.Fatal("expected non-nil meter")
	}
}
