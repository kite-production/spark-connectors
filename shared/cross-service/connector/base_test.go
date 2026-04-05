package connector

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	controlplanepb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/controlplane"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type publishedMsg struct {
	subject string
	data    []byte
}

type fakePublisher struct {
	mu        sync.Mutex
	published []publishedMsg
	err       error
}

func (f *fakePublisher) Publish(subj string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.published = append(f.published, publishedMsg{subject: subj, data: data})
	return &nats.PubAck{Stream: "SPARK_CONNECTOR_INBOUND"}, nil
}

type fakeCPClient struct {
	mu              sync.Mutex
	registerCalls   []*controlplanepb.RegisterConnectorRequest
	deregisterCalls []*controlplanepb.DeregisterConnectorRequest
	registerErr     error
	deregisterErr   error
	registerErrs    []error // sequential errors for retry tests
}

func (f *fakeCPClient) RegisterConnector(_ context.Context, in *controlplanepb.RegisterConnectorRequest, _ ...grpc.CallOption) (*controlplanepb.RegisterConnectorResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registerCalls = append(f.registerCalls, in)

	if len(f.registerErrs) > 0 {
		err := f.registerErrs[0]
		f.registerErrs = f.registerErrs[1:]
		if err != nil {
			return nil, err
		}
	} else if f.registerErr != nil {
		return nil, f.registerErr
	}

	return &controlplanepb.RegisterConnectorResponse{
		ConnectorId:  in.GetConnectorId(),
		RegisteredAt: timestamppb.Now(),
	}, nil
}

func (f *fakeCPClient) DeregisterConnector(_ context.Context, in *controlplanepb.DeregisterConnectorRequest, _ ...grpc.CallOption) (*controlplanepb.DeregisterConnectorResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deregisterCalls = append(f.deregisterCalls, in)
	if f.deregisterErr != nil {
		return nil, f.deregisterErr
	}
	return &controlplanepb.DeregisterConnectorResponse{
		ConnectorId:    in.GetConnectorId(),
		DeregisteredAt: timestamppb.Now(),
	}, nil
}

// ---------------------------------------------------------------------------
// PublishInbound tests
// ---------------------------------------------------------------------------

func TestPublishInbound_Success(t *testing.T) {
	pub := &fakePublisher{}
	bc := New(Config{ConnectorID: "slack"})
	bc.Publisher = pub

	msg := &connectorpb.InboundMessage{
		ConnectorId: "slack",
		AccountId:   "acct-1",
		MessageId:   "msg-123",
		Text:        "hello",
	}

	if err := bc.PublishInbound(msg); err != nil {
		t.Fatalf("PublishInbound: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(pub.published))
	}
	if pub.published[0].subject != "spark.connector.inbound.slack" {
		t.Errorf("subject = %q, want %q", pub.published[0].subject, "spark.connector.inbound.slack")
	}

	// Verify the published data deserializes correctly.
	var decoded connectorpb.InboundMessage
	if err := proto.Unmarshal(pub.published[0].data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.GetText() != "hello" {
		t.Errorf("text = %q, want %q", decoded.GetText(), "hello")
	}
	if decoded.GetConnectorId() != "slack" {
		t.Errorf("connector_id = %q, want %q", decoded.GetConnectorId(), "slack")
	}
}

func TestPublishInbound_NilPublisher(t *testing.T) {
	bc := New(Config{ConnectorID: "slack"})
	err := bc.PublishInbound(&connectorpb.InboundMessage{ConnectorId: "slack"})
	if err == nil {
		t.Fatal("expected error for nil publisher")
	}
}

func TestPublishInbound_NilMessage(t *testing.T) {
	pub := &fakePublisher{}
	bc := New(Config{ConnectorID: "slack"})
	bc.Publisher = pub

	err := bc.PublishInbound(nil)
	if err == nil {
		t.Fatal("expected error for nil message")
	}
}

func TestPublishInbound_PublishError(t *testing.T) {
	pub := &fakePublisher{err: fmt.Errorf("nats timeout")}
	bc := New(Config{ConnectorID: "slack"})
	bc.Publisher = pub

	msg := &connectorpb.InboundMessage{
		ConnectorId: "slack",
		MessageId:   "msg-1",
		Text:        "hello",
	}

	err := bc.PublishInbound(msg)
	if err == nil {
		t.Fatal("expected error on publish failure")
	}
}

// ---------------------------------------------------------------------------
// Registration tests
// ---------------------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	cp := &fakeCPClient{}
	bc := New(Config{
		ConnectorID: "slack",
		GRPCAddress: "localhost:50061",
		Capabilities: &connectorpb.ConnectorCapabilities{
			SupportsThreads: true,
		},
	})
	bc.CPClient = cp

	if err := bc.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !bc.IsRegistered() {
		t.Error("expected registered=true")
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()
	if len(cp.registerCalls) != 1 {
		t.Fatalf("expected 1 register call, got %d", len(cp.registerCalls))
	}
	req := cp.registerCalls[0]
	if req.GetConnectorId() != "slack" {
		t.Errorf("connector_id = %q, want %q", req.GetConnectorId(), "slack")
	}
	if req.GetGrpcAddress() != "localhost:50061" {
		t.Errorf("grpc_address = %q, want %q", req.GetGrpcAddress(), "localhost:50061")
	}
	if !req.GetCapabilities().GetSupportsThreads() {
		t.Error("expected supports_threads=true in capabilities")
	}
}

func TestRegister_Error(t *testing.T) {
	cp := &fakeCPClient{registerErr: fmt.Errorf("unavailable")}
	bc := New(Config{ConnectorID: "slack"})
	bc.CPClient = cp

	err := bc.Register(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if bc.IsRegistered() {
		t.Error("should not be registered after error")
	}
}

func TestRegister_NilClient(t *testing.T) {
	bc := New(Config{ConnectorID: "slack"})
	err := bc.Register(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestDeregister_Success(t *testing.T) {
	cp := &fakeCPClient{}
	bc := New(Config{ConnectorID: "gmail"})
	bc.CPClient = cp
	bc.registered = true

	if err := bc.Deregister(context.Background()); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	if bc.IsRegistered() {
		t.Error("expected registered=false after deregister")
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()
	if len(cp.deregisterCalls) != 1 {
		t.Fatalf("expected 1 deregister call, got %d", len(cp.deregisterCalls))
	}
	if cp.deregisterCalls[0].GetConnectorId() != "gmail" {
		t.Errorf("connector_id = %q, want %q", cp.deregisterCalls[0].GetConnectorId(), "gmail")
	}
}

func TestDeregister_Error(t *testing.T) {
	cp := &fakeCPClient{deregisterErr: fmt.Errorf("unavailable")}
	bc := New(Config{ConnectorID: "gmail"})
	bc.CPClient = cp
	bc.registered = true

	err := bc.Deregister(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterWithRetry_RetriesThenSucceeds(t *testing.T) {
	cp := &fakeCPClient{
		registerErrs: []error{
			fmt.Errorf("attempt 1 fail"),
			fmt.Errorf("attempt 2 fail"),
			nil, // succeed on attempt 3
		},
	}
	bc := New(Config{
		ConnectorID: "slack",
		GRPCAddress: "localhost:50061",
		BackoffConfig: BackoffConfig{
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     50 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      5,
		},
	})
	bc.CPClient = cp

	if err := bc.RegisterWithRetry(context.Background()); err != nil {
		t.Fatalf("RegisterWithRetry: %v", err)
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()
	if len(cp.registerCalls) != 3 {
		t.Errorf("expected 3 register attempts, got %d", len(cp.registerCalls))
	}
	if !bc.IsRegistered() {
		t.Error("expected registered=true")
	}
}

func TestRegisterWithRetry_ExceedsMaxRetries(t *testing.T) {
	cp := &fakeCPClient{registerErr: fmt.Errorf("always fails")}
	bc := New(Config{
		ConnectorID: "slack",
		BackoffConfig: BackoffConfig{
			InitialInterval: 5 * time.Millisecond,
			MaxInterval:     20 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      3,
		},
	})
	bc.CPClient = cp

	err := bc.RegisterWithRetry(context.Background())
	if err == nil {
		t.Fatal("expected error after max retries")
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()
	if len(cp.registerCalls) != 3 {
		t.Errorf("expected 3 register attempts, got %d", len(cp.registerCalls))
	}
}

// ---------------------------------------------------------------------------
// Reconnection / backoff tests
// ---------------------------------------------------------------------------

func TestBackoffConfig_NextInterval(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     10 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      5,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 10 * time.Second}, // capped at MaxInterval
		{5, 10 * time.Second}, // still capped
	}

	for _, tt := range tests {
		got := cfg.NextInterval(tt.attempt)
		if got != tt.expected {
			t.Errorf("NextInterval(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestWithBackoff_SucceedsImmediately(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     100 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      3,
	}

	calls := 0
	err := WithBackoff(context.Background(), cfg, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatalf("WithBackoff: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestWithBackoff_RetriesThenSucceeds(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 5 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
	}

	calls := 0
	err := WithBackoff(context.Background(), cfg, func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("not yet")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithBackoff: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithBackoff_MaxRetriesExceeded(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 5 * time.Millisecond,
		MaxInterval:     20 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      3,
	}

	calls := 0
	err := WithBackoff(context.Background(), cfg, func() error {
		calls++
		return fmt.Errorf("always fails")
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithBackoff_ContextCancelled(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     5 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := WithBackoff(ctx, cfg, func() error {
		calls++
		return fmt.Errorf("fail")
	})

	if err == nil {
		t.Fatal("expected error on context cancel")
	}
}

func TestWithBackoff_BackoffTimingVerification(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 50 * time.Millisecond,
		MaxInterval:     200 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      4,
	}

	var timestamps []time.Time
	_ = WithBackoff(context.Background(), cfg, func() error {
		timestamps = append(timestamps, time.Now())
		return fmt.Errorf("fail")
	})

	if len(timestamps) != 4 {
		t.Fatalf("expected 4 timestamps, got %d", len(timestamps))
	}

	// Verify that intervals grow roughly as expected.
	// Allow 30ms tolerance for scheduling jitter.
	tolerance := 30 * time.Millisecond
	expectedIntervals := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond, // capped at MaxInterval
	}

	for i := 0; i < len(expectedIntervals); i++ {
		actual := timestamps[i+1].Sub(timestamps[i])
		expected := expectedIntervals[i]
		if actual < expected-tolerance || actual > expected+tolerance {
			t.Errorf("interval %d: got %v, want ~%v (±%v)", i, actual, expected, tolerance)
		}
	}
}

// ---------------------------------------------------------------------------
// Health and lifecycle tests
// ---------------------------------------------------------------------------

func TestHealthCheck(t *testing.T) {
	bc := New(Config{ConnectorID: "slack"})

	resp, err := bc.HealthCheck(context.Background(), &connectorpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if resp.GetStatus() != "NOT_SERVING" {
		t.Errorf("status = %q, want %q (unhealthy by default)", resp.GetStatus(), "NOT_SERVING")
	}
	if resp.GetService() != "slack" {
		t.Errorf("service = %q, want %q", resp.GetService(), "slack")
	}

	bc.SetHealthy(true)
	resp, _ = bc.HealthCheck(context.Background(), &connectorpb.HealthCheckRequest{})
	if resp.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want %q", resp.GetStatus(), "SERVING")
	}
}

func TestNewDefaults(t *testing.T) {
	bc := New(Config{ConnectorID: "test"})

	if bc.Config.BackoffConfig.InitialInterval == 0 {
		t.Error("expected default backoff config to be set")
	}
	if bc.IsHealthy() {
		t.Error("expected healthy=false initially")
	}
	if bc.IsRegistered() {
		t.Error("expected registered=false initially")
	}
}

func TestInboundSubject(t *testing.T) {
	tests := []struct {
		connectorID string
		want        string
	}{
		{"slack", "spark.connector.inbound.slack"},
		{"gmail", "spark.connector.inbound.gmail"},
		{"google-chat", "spark.connector.inbound.google-chat"},
	}
	for _, tt := range tests {
		got := InboundSubject(tt.connectorID)
		if got != tt.want {
			t.Errorf("InboundSubject(%q) = %q, want %q", tt.connectorID, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NewGRPCServer test
// ---------------------------------------------------------------------------

func TestNewGRPCServer(t *testing.T) {
	svc := &connectorpb.UnimplementedConnectorServiceServer{}
	srv := NewGRPCServer(svc)
	if srv == nil {
		t.Fatal("NewGRPCServer returned nil")
	}
	srv.Stop()
}

// ---------------------------------------------------------------------------
// Start / Stop tests
// ---------------------------------------------------------------------------

func TestStart_Success(t *testing.T) {
	cp := &fakeCPClient{}
	pub := &fakePublisher{}

	bc := New(Config{
		ConnectorID:         "slack",
		GRPCAddress:         "localhost:50061",
		ControlPlaneAddress: "localhost:50050",
		BackoffConfig: BackoffConfig{
			InitialInterval: 5 * time.Millisecond,
			MaxInterval:     20 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      3,
		},
	})
	bc.ConnectNATS = func(_ Config) (*nats.Conn, NATSPublisher, error) {
		return nil, pub, nil
	}
	bc.DialCP = func(_ string) (*grpc.ClientConn, ControlPlaneClient, error) {
		return nil, cp, nil
	}

	err := bc.Start(context.Background(), &connectorpb.UnimplementedConnectorServiceServer{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !bc.IsHealthy() {
		t.Error("expected healthy=true after start")
	}
	if !bc.IsRegistered() {
		t.Error("expected registered=true after start")
	}
	if bc.GRPCServer == nil {
		t.Error("expected GRPCServer to be set")
	}
	if bc.Publisher != pub {
		t.Error("expected Publisher to be set")
	}

	bc.GRPCServer.Stop()
}

func TestStart_NATSFailure(t *testing.T) {
	bc := New(Config{
		ConnectorID: "slack",
		BackoffConfig: BackoffConfig{
			InitialInterval: 5 * time.Millisecond,
			MaxInterval:     10 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      2,
		},
	})
	bc.ConnectNATS = func(_ Config) (*nats.Conn, NATSPublisher, error) {
		return nil, nil, fmt.Errorf("nats down")
	}

	err := bc.Start(context.Background(), &connectorpb.UnimplementedConnectorServiceServer{})
	if err == nil {
		t.Fatal("expected error when NATS fails")
	}
	if bc.IsHealthy() {
		t.Error("should not be healthy after NATS failure")
	}
}

func TestStart_RegistrationFailure(t *testing.T) {
	pub := &fakePublisher{}
	cp := &fakeCPClient{registerErr: fmt.Errorf("cp down")}

	bc := New(Config{
		ConnectorID:         "slack",
		ControlPlaneAddress: "localhost:50050",
		BackoffConfig: BackoffConfig{
			InitialInterval: 5 * time.Millisecond,
			MaxInterval:     10 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      2,
		},
	})
	bc.ConnectNATS = func(_ Config) (*nats.Conn, NATSPublisher, error) {
		return nil, pub, nil
	}
	bc.DialCP = func(_ string) (*grpc.ClientConn, ControlPlaneClient, error) {
		return nil, cp, nil
	}

	err := bc.Start(context.Background(), &connectorpb.UnimplementedConnectorServiceServer{})
	if err == nil {
		t.Fatal("expected error when registration fails")
	}
	if bc.IsHealthy() {
		t.Error("should not be healthy after registration failure")
	}
	if bc.GRPCServer != nil {
		bc.GRPCServer.Stop()
	}
}

func TestStart_MissingControlPlaneAddress(t *testing.T) {
	pub := &fakePublisher{}

	bc := New(Config{
		ConnectorID:         "slack",
		ControlPlaneAddress: "", // empty
		BackoffConfig: BackoffConfig{
			InitialInterval: 5 * time.Millisecond,
			MaxInterval:     10 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      2,
		},
	})
	bc.ConnectNATS = func(_ Config) (*nats.Conn, NATSPublisher, error) {
		return nil, pub, nil
	}

	err := bc.Start(context.Background(), &connectorpb.UnimplementedConnectorServiceServer{})
	if err == nil {
		t.Fatal("expected error for missing control-plane address")
	}
	if bc.GRPCServer != nil {
		bc.GRPCServer.Stop()
	}
}

func TestStop_WithRegisteredConnector(t *testing.T) {
	cp := &fakeCPClient{}
	bc := New(Config{ConnectorID: "slack"})
	bc.CPClient = cp
	bc.registered = true
	bc.healthy = true

	srv := NewGRPCServer(&connectorpb.UnimplementedConnectorServiceServer{})
	bc.GRPCServer = srv

	bc.Stop(context.Background())

	if bc.IsHealthy() {
		t.Error("expected healthy=false after stop")
	}
	if bc.IsRegistered() {
		t.Error("expected registered=false after stop")
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if len(cp.deregisterCalls) != 1 {
		t.Errorf("expected 1 deregister call, got %d", len(cp.deregisterCalls))
	}
}

func TestStop_WithoutRegistration(t *testing.T) {
	bc := New(Config{ConnectorID: "slack"})
	bc.healthy = true

	// Should not panic with nil fields.
	bc.Stop(context.Background())

	if bc.IsHealthy() {
		t.Error("expected healthy=false after stop")
	}
}

func TestStop_DeregisterError(t *testing.T) {
	cp := &fakeCPClient{deregisterErr: fmt.Errorf("cp gone")}
	bc := New(Config{ConnectorID: "slack"})
	bc.CPClient = cp
	bc.registered = true
	bc.healthy = true

	bc.Stop(context.Background())

	if bc.IsHealthy() {
		t.Error("expected healthy=false after stop")
	}
}
