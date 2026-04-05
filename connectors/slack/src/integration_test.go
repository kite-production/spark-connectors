package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	controlplanepb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/controlplane"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/kite-production/spark/services/connector-slack/internal/normalizer"
	"github.com/kite-production/spark/services/connector-slack/internal/service"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Mock NATS publisher
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

func (f *fakePublisher) getPublished() []publishedMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]publishedMsg, len(f.published))
	copy(out, f.published)
	return out
}

// ---------------------------------------------------------------------------
// Mock control-plane client
// ---------------------------------------------------------------------------

type fakeCPClient struct {
	mu              sync.Mutex
	registerCalls   []*controlplanepb.RegisterConnectorRequest
	deregisterCalls []*controlplanepb.DeregisterConnectorRequest
	registerErr     error
	deregisterErr   error
}

func (f *fakeCPClient) RegisterConnector(_ context.Context, in *controlplanepb.RegisterConnectorRequest, _ ...grpc.CallOption) (*controlplanepb.RegisterConnectorResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registerCalls = append(f.registerCalls, in)
	if f.registerErr != nil {
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

func (f *fakeCPClient) getRegisterCalls() []*controlplanepb.RegisterConnectorRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*controlplanepb.RegisterConnectorRequest, len(f.registerCalls))
	copy(out, f.registerCalls)
	return out
}

func (f *fakeCPClient) getDeregisterCalls() []*controlplanepb.DeregisterConnectorRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*controlplanepb.DeregisterConnectorRequest, len(f.deregisterCalls))
	copy(out, f.deregisterCalls)
	return out
}

// ---------------------------------------------------------------------------
// Mock Slack API server
// ---------------------------------------------------------------------------

type slackAPICall struct {
	method  string
	channel string
	text    string
	threadTS string
}

type mockSlackAPIServer struct {
	mu    sync.Mutex
	calls []slackAPICall
	users map[string]string // userID → display name
}

func newMockSlackAPIServer() *mockSlackAPIServer {
	return &mockSlackAPIServer{
		users: map[string]string{
			"U001": "alice",
			"U002": "bob",
		},
	}
}

func (m *mockSlackAPIServer) getCalls() []slackAPICall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]slackAPICall, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *mockSlackAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/auth.test":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"team":    "TestTeam",
			"team_id": "T123",
			"user_id": "UBOT",
		})

	case "/users.info":
		userID := r.FormValue("user")
		if userID == "" {
			// Parse from JSON body or URL params.
			userID = r.URL.Query().Get("user")
		}
		m.mu.Lock()
		name, ok := m.users[userID]
		m.mu.Unlock()
		if !ok {
			name = userID
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"user": map[string]interface{}{
				"id":        userID,
				"real_name": name,
				"profile": map[string]interface{}{
					"display_name": name,
				},
			},
		})

	case "/chat.postMessage":
		r.ParseForm()
		channel := r.FormValue("channel")
		text := r.FormValue("text")
		threadTS := r.FormValue("thread_ts")

		m.mu.Lock()
		m.calls = append(m.calls, slackAPICall{
			method:   "chat.postMessage",
			channel:  channel,
			text:     text,
			threadTS: threadTS,
		})
		m.mu.Unlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"channel": channel,
			"ts":      fmt.Sprintf("%d.000100", time.Now().UnixMicro()),
			"message": map[string]interface{}{
				"text": text,
			},
		})

	case "/apps.connections.open":
		// Socket Mode connection endpoint — return a WebSocket URL.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":  true,
			"url": "ws://localhost:0/ws", // placeholder
		})

	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "unknown_method",
		})
	}
}

// ---------------------------------------------------------------------------
// Mock Socket Mode WebSocket server
// ---------------------------------------------------------------------------

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// wsConn wraps a WebSocket connection with a write mutex.
type wsConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

func (wc *wsConn) writeMessage(msgType int, data []byte) error {
	wc.wmu.Lock()
	defer wc.wmu.Unlock()
	return wc.conn.WriteMessage(msgType, data)
}

type mockSocketModeServer struct {
	mu       sync.Mutex
	conns    []*wsConn
	server   *httptest.Server
	eventSeq int
	ready    chan struct{} // closed when first connection is established
}

func newMockSocketModeServer() *mockSocketModeServer {
	ms := &mockSocketModeServer{
		ready: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", ms.handleWS)
	ms.server = httptest.NewServer(mux)
	return ms
}

func (ms *mockSocketModeServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	wc := &wsConn{conn: conn}

	// Send hello event before adding to conns (serialized with per-conn mutex).
	hello := map[string]interface{}{
		"type": "hello",
		"connection_info": map[string]interface{}{
			"app_id": "A001",
		},
	}
	data, _ := json.Marshal(hello)
	wc.writeMessage(websocket.TextMessage, data)

	ms.mu.Lock()
	first := len(ms.conns) == 0
	ms.conns = append(ms.conns, wc)
	ms.mu.Unlock()

	if first {
		close(ms.ready)
	}

	// Keep connection open, read acks.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (ms *mockSocketModeServer) sendMessageEvent(userID, channel, text, channelType, threadTS string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.eventSeq++
	ts := fmt.Sprintf("1700000000.%06d", ms.eventSeq)

	innerEvent := map[string]interface{}{
		"type":         "message",
		"user":         userID,
		"text":         text,
		"channel":      channel,
		"ts":           ts,
		"channel_type": channelType,
	}
	if threadTS != "" {
		innerEvent["thread_ts"] = threadTS
	}

	envelope := map[string]interface{}{
		"type":        "events_api",
		"envelope_id": fmt.Sprintf("env-%d", ms.eventSeq),
		"payload": map[string]interface{}{
			"type": "event_callback",
			"event": innerEvent,
		},
	}

	data, _ := json.Marshal(envelope)
	for _, wc := range ms.conns {
		wc.writeMessage(websocket.TextMessage, data)
	}
}

func (ms *mockSocketModeServer) close() {
	ms.mu.Lock()
	for _, wc := range ms.conns {
		wc.conn.Close()
	}
	ms.mu.Unlock()
	ms.server.Close()
}

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type testHarness struct {
	slackServer *mockSlackAPIServer
	httpServer  *httptest.Server
	smServer    *mockSocketModeServer
	base        *baseconnector.BaseConnector
	svc         *service.Service
	publisher   *fakePublisher
	cpClient    *fakeCPClient
	grpcConn    *grpc.ClientConn
	grpcClient  connectorpb.ConnectorServiceClient
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	// Start mock Slack API server.
	slackAPI := newMockSlackAPIServer()
	httpServer := httptest.NewServer(slackAPI)

	// Start mock Socket Mode WebSocket server.
	smServer := newMockSocketModeServer()

	// Create Slack client pointed at mock server.
	api := slack.New("xoxb-test-token",
		slack.OptionAPIURL(httpServer.URL+"/"),
	)

	// Create display name resolver using the mock Slack API.
	resolver := &slackDisplayNameResolver{api: api}

	// Create real normalizer with mock-backed resolver.
	norm := normalizer.New("slack", resolver)

	// Create fake publisher and CP client.
	pub := &fakePublisher{}
	cp := &fakeCPClient{}

	// Create BaseConnector with injected fakes.
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "slack",
		GRPCAddress:         "localhost:0",
		ControlPlaneAddress: "localhost:0",
		BackoffConfig: baseconnector.BackoffConfig{
			InitialInterval: 5 * time.Millisecond,
			MaxInterval:     20 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      2,
		},
	})
	base.Publisher = pub
	base.CPClient = cp
	// Skip NATS/CP connection by injecting directly.
	base.ConnectNATS = func(_ baseconnector.Config) (*nats.Conn, baseconnector.NATSPublisher, error) {
		return nil, pub, nil
	}
	base.DialCP = func(_ string) (*grpc.ClientConn, baseconnector.ControlPlaneClient, error) {
		return nil, cp, nil
	}

	// Create metrics with isolated registry.
	reg := prometheus.NewRegistry()
	metrics := service.NewMetrics(reg)

	// Create service with real Slack API client pointed at mock.
	svc := service.New(base, norm, api, metrics)

	// Start BaseConnector (will use fakes).
	if err := base.Start(context.Background(), svc); err != nil {
		t.Fatalf("base.Start: %v", err)
	}

	// Start gRPC server on random port.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go base.GRPCServer.Serve(lis)

	// Create gRPC client.
	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	client := connectorpb.NewConnectorServiceClient(conn)

	h := &testHarness{
		slackServer: slackAPI,
		httpServer:  httpServer,
		smServer:    smServer,
		base:        base,
		svc:         svc,
		publisher:   pub,
		cpClient:    cp,
		grpcConn:    conn,
		grpcClient:  client,
	}

	t.Cleanup(func() {
		conn.Close()
		base.GRPCServer.Stop()
		smServer.close()
		httpServer.Close()
	})

	return h
}

// slackDisplayNameResolver resolves display names via the Slack API.
type slackDisplayNameResolver struct {
	api *slack.Client
}

func (r *slackDisplayNameResolver) GetDisplayName(userID string) string {
	user, err := r.api.GetUserInfo(userID)
	if err != nil {
		return userID
	}
	if user.Profile.DisplayName != "" {
		return user.Profile.DisplayName
	}
	return user.RealName
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

func TestIntegration_InboundMessage(t *testing.T) {
	h := newTestHarness(t)

	// Simulate a Slack message event arriving via Socket Mode.
	ev := &slackevents.MessageEvent{
		User:        "U001",
		Text:        "Hello from Slack!",
		Channel:     "C100",
		ChannelType: "channel",
		TimeStamp:   "1700000000.000001",
	}

	h.svc.HandleMessageEvent(ev, "T123")

	// Verify message was published to NATS.
	msgs := h.publisher.getPublished()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}

	if msgs[0].subject != "spark.connector.inbound.slack" {
		t.Errorf("subject = %q, want %q", msgs[0].subject, "spark.connector.inbound.slack")
	}

	// Decode and verify the InboundMessage.
	var inbound connectorpb.InboundMessage
	if err := proto.Unmarshal(msgs[0].data, &inbound); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if inbound.ConnectorId != "slack" {
		t.Errorf("ConnectorId = %q, want %q", inbound.ConnectorId, "slack")
	}
	if inbound.Text != "Hello from Slack!" {
		t.Errorf("Text = %q, want %q", inbound.Text, "Hello from Slack!")
	}
	if inbound.PeerId != "C100" {
		t.Errorf("PeerId = %q, want %q", inbound.PeerId, "C100")
	}
	if inbound.PeerKind != commonpb.PeerKind_PEER_KIND_CHANNEL {
		t.Errorf("PeerKind = %v, want PEER_KIND_CHANNEL", inbound.PeerKind)
	}
	if inbound.GroupId != "T123" {
		t.Errorf("GroupId = %q, want %q", inbound.GroupId, "T123")
	}
	if inbound.Sender.SenderId != "U001" {
		t.Errorf("SenderId = %q, want %q", inbound.Sender.SenderId, "U001")
	}
	// Display name resolved from mock Slack API.
	if inbound.Sender.SenderName != "alice" {
		t.Errorf("SenderName = %q, want %q", inbound.Sender.SenderName, "alice")
	}
	if inbound.IdempotencyKey.Value != "slack:T123:1700000000.000001" {
		t.Errorf("IdempotencyKey = %q, want %q", inbound.IdempotencyKey.Value, "slack:T123:1700000000.000001")
	}
	if inbound.ThreadId != "" {
		t.Errorf("ThreadId = %q, want empty (not a threaded message)", inbound.ThreadId)
	}
}

func TestIntegration_InboundDM(t *testing.T) {
	h := newTestHarness(t)

	ev := &slackevents.MessageEvent{
		User:        "U002",
		Text:        "Private message",
		Channel:     "D200",
		ChannelType: "im",
		TimeStamp:   "1700000000.000002",
	}

	h.svc.HandleMessageEvent(ev, "T123")

	msgs := h.publisher.getPublished()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}

	var inbound connectorpb.InboundMessage
	if err := proto.Unmarshal(msgs[0].data, &inbound); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if inbound.PeerKind != commonpb.PeerKind_PEER_KIND_DM {
		t.Errorf("PeerKind = %v, want PEER_KIND_DM", inbound.PeerKind)
	}
	if inbound.PeerId != "D200" {
		t.Errorf("PeerId = %q, want %q", inbound.PeerId, "D200")
	}
	if inbound.Sender.SenderName != "bob" {
		t.Errorf("SenderName = %q, want %q", inbound.Sender.SenderName, "bob")
	}
}

func TestIntegration_ThreadedInbound(t *testing.T) {
	h := newTestHarness(t)

	ev := &slackevents.MessageEvent{
		User:             "U001",
		Text:             "Thread reply",
		Channel:          "C100",
		ChannelType:      "channel",
		TimeStamp:        "1700000000.000010",
		ThreadTimeStamp:  "1700000000.000001",
	}

	h.svc.HandleMessageEvent(ev, "T123")

	msgs := h.publisher.getPublished()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}

	var inbound connectorpb.InboundMessage
	if err := proto.Unmarshal(msgs[0].data, &inbound); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if inbound.ThreadId != "1700000000.000001" {
		t.Errorf("ThreadId = %q, want %q", inbound.ThreadId, "1700000000.000001")
	}
}

func TestIntegration_BotMessageFiltered(t *testing.T) {
	h := newTestHarness(t)

	// Bot message (has BotID).
	h.svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:  "I am a bot",
		BotID: "B999",
		User:  "U001",
		Channel: "C100",
		TimeStamp: "1700000000.000003",
	}, "T123")

	// message_changed subtype.
	h.svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:    "edited message",
		SubType: "message_changed",
		User:    "U001",
		Channel: "C100",
		TimeStamp: "1700000000.000004",
	}, "T123")

	// message_deleted subtype.
	h.svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:    "deleted",
		SubType: "message_deleted",
		User:    "U001",
		Channel: "C100",
		TimeStamp: "1700000000.000005",
	}, "T123")

	// bot_message subtype (no BotID but subtype).
	h.svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:    "bot subtype",
		SubType: "bot_message",
		User:    "U001",
		Channel: "C100",
		TimeStamp: "1700000000.000006",
	}, "T123")

	// channel_join subtype.
	h.svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:    "joined",
		SubType: "channel_join",
		User:    "U001",
		Channel: "C100",
		TimeStamp: "1700000000.000007",
	}, "T123")

	// Empty text.
	h.svc.HandleMessageEvent(&slackevents.MessageEvent{
		Text:    "",
		User:    "U001",
		Channel: "C100",
		TimeStamp: "1700000000.000008",
	}, "T123")

	// None of these should be published.
	msgs := h.publisher.getPublished()
	if len(msgs) != 0 {
		t.Errorf("expected 0 published messages for filtered events, got %d", len(msgs))
	}
}

func TestIntegration_SendMessage_MarkdownToMrkdwn(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	resp, err := h.grpcClient.SendMessage(ctx, &connectorpb.SendMessageRequest{
		PeerId: "C100",
		Text:   "Hello **world**, check [this](https://example.com)",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.MessageId == "" {
		t.Error("expected non-empty MessageId")
	}
	if resp.SentAt == nil {
		t.Error("expected SentAt to be set")
	}

	// Verify the mock Slack API received the formatted message.
	calls := h.slackServer.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 chat.postMessage call, got %d", len(calls))
	}

	call := calls[0]
	if call.channel != "C100" {
		t.Errorf("channel = %q, want %q", call.channel, "C100")
	}
	// **world** should become *world* in mrkdwn.
	if !strings.Contains(call.text, "*world*") {
		t.Errorf("expected mrkdwn bold *world*, got %q", call.text)
	}
	// [text](url) should become <url|text>.
	if !strings.Contains(call.text, "<https://example.com|this>") {
		t.Errorf("expected mrkdwn link, got %q", call.text)
	}
	// No thread_ts for non-threaded message.
	if call.threadTS != "" {
		t.Errorf("expected empty thread_ts, got %q", call.threadTS)
	}
}

func TestIntegration_SendMessage_ThreadedReply(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	_, err := h.grpcClient.SendMessage(ctx, &connectorpb.SendMessageRequest{
		PeerId:   "C100",
		Text:     "Thread reply",
		ThreadId: "1700000000.000001",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	calls := h.slackServer.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0].threadTS != "1700000000.000001" {
		t.Errorf("thread_ts = %q, want %q", calls[0].threadTS, "1700000000.000001")
	}
}

func TestIntegration_Registration(t *testing.T) {
	h := newTestHarness(t)

	// Verify connector registered with control-plane on startup.
	regCalls := h.cpClient.getRegisterCalls()
	if len(regCalls) != 1 {
		t.Fatalf("expected 1 register call, got %d", len(regCalls))
	}
	if regCalls[0].ConnectorId != "slack" {
		t.Errorf("registered connector_id = %q, want %q", regCalls[0].ConnectorId, "slack")
	}
	if !h.base.IsRegistered() {
		t.Error("expected IsRegistered=true")
	}
	if !h.base.IsHealthy() {
		t.Error("expected IsHealthy=true after start")
	}
}

func TestIntegration_Deregistration(t *testing.T) {
	// Create a separate harness for this test since Stop() is destructive.
	slackAPI := newMockSlackAPIServer()
	httpServer := httptest.NewServer(slackAPI)
	defer httpServer.Close()

	api := slack.New("xoxb-test-token",
		slack.OptionAPIURL(httpServer.URL+"/"),
	)
	resolver := &slackDisplayNameResolver{api: api}
	norm := normalizer.New("slack", resolver)
	pub := &fakePublisher{}
	cp := &fakeCPClient{}

	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "slack",
		GRPCAddress:         "localhost:0",
		ControlPlaneAddress: "localhost:0",
		BackoffConfig: baseconnector.BackoffConfig{
			InitialInterval: 5 * time.Millisecond,
			MaxInterval:     20 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      2,
		},
	})
	base.ConnectNATS = func(_ baseconnector.Config) (*nats.Conn, baseconnector.NATSPublisher, error) {
		return nil, pub, nil
	}
	base.DialCP = func(_ string) (*grpc.ClientConn, baseconnector.ControlPlaneClient, error) {
		return nil, cp, nil
	}

	reg := prometheus.NewRegistry()
	metrics := service.NewMetrics(reg)
	svc := service.New(base, norm, api, metrics)

	if err := base.Start(context.Background(), svc); err != nil {
		t.Fatalf("base.Start: %v", err)
	}

	// Verify registered.
	if !base.IsRegistered() {
		t.Fatal("expected registered=true after start")
	}

	// Stop triggers deregistration.
	base.Stop(context.Background())

	if base.IsRegistered() {
		t.Error("expected registered=false after stop")
	}
	if base.IsHealthy() {
		t.Error("expected healthy=false after stop")
	}

	deregCalls := cp.getDeregisterCalls()
	if len(deregCalls) != 1 {
		t.Fatalf("expected 1 deregister call, got %d", len(deregCalls))
	}
	if deregCalls[0].ConnectorId != "slack" {
		t.Errorf("deregistered connector_id = %q, want %q", deregCalls[0].ConnectorId, "slack")
	}
}

func TestIntegration_HealthCheckViaGRPC(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	resp, err := h.grpcClient.HealthCheck(ctx, &connectorpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if resp.Status != "SERVING" {
		t.Errorf("Status = %q, want %q", resp.Status, "SERVING")
	}
	if resp.Service != "slack" {
		t.Errorf("Service = %q, want %q", resp.Service, "slack")
	}
}

func TestIntegration_GetCapabilitiesViaGRPC(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	resp, err := h.grpcClient.GetCapabilities(ctx, &connectorpb.GetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("GetCapabilities: %v", err)
	}

	caps := resp.Capabilities
	if !caps.SupportsThreads {
		t.Error("expected SupportsThreads=true")
	}
	if !caps.SupportsReactions {
		t.Error("expected SupportsReactions=true")
	}
	if caps.MaxMessageLength != 4000 {
		t.Errorf("MaxMessageLength = %d, want 4000", caps.MaxMessageLength)
	}
}

func TestIntegration_GetStatusViaGRPC(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()
	h.svc.SetWSConnected(true)

	resp, err := h.grpcClient.GetStatus(ctx, &connectorpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.ConnectorId != "slack" {
		t.Errorf("ConnectorId = %q, want %q", resp.ConnectorId, "slack")
	}
	if !resp.Healthy {
		t.Error("expected Healthy=true")
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if !resp.Accounts[0].Connected {
		t.Error("expected account Connected=true")
	}
}

func TestIntegration_MultipleInboundMessages(t *testing.T) {
	h := newTestHarness(t)

	// Send multiple valid messages interspersed with filtered ones.
	events := []struct {
		ev      *slackevents.MessageEvent
		publish bool
	}{
		{
			ev: &slackevents.MessageEvent{
				User: "U001", Text: "msg 1", Channel: "C100",
				ChannelType: "channel", TimeStamp: "1700000000.000001",
			},
			publish: true,
		},
		{
			ev: &slackevents.MessageEvent{
				Text: "bot", BotID: "B1", Channel: "C100",
				TimeStamp: "1700000000.000002",
			},
			publish: false,
		},
		{
			ev: &slackevents.MessageEvent{
				User: "U002", Text: "msg 2", Channel: "D200",
				ChannelType: "im", TimeStamp: "1700000000.000003",
			},
			publish: true,
		},
		{
			ev: &slackevents.MessageEvent{
				User: "U001", Text: "msg 3", Channel: "C100",
				ChannelType: "channel", TimeStamp: "1700000000.000004",
			},
			publish: true,
		},
	}

	for _, e := range events {
		h.svc.HandleMessageEvent(e.ev, "T123")
	}

	msgs := h.publisher.getPublished()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 published messages, got %d", len(msgs))
	}

	// Verify ordering and content.
	expectedTexts := []string{"msg 1", "msg 2", "msg 3"}
	for i, msg := range msgs {
		var inbound connectorpb.InboundMessage
		if err := proto.Unmarshal(msg.data, &inbound); err != nil {
			t.Fatalf("unmarshal msg %d: %v", i, err)
		}
		if inbound.Text != expectedTexts[i] {
			t.Errorf("msg %d: Text = %q, want %q", i, inbound.Text, expectedTexts[i])
		}
	}
}

func TestIntegration_SocketModeServer(t *testing.T) {
	// Verify the mock Socket Mode WebSocket server accepts connections
	// and sends the hello event.
	ms := newMockSocketModeServer()
	defer ms.close()

	wsURL := "ws" + strings.TrimPrefix(ms.server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial WebSocket: %v", err)
	}
	defer conn.Close()

	// Read hello message.
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var hello map[string]interface{}
	if err := json.Unmarshal(data, &hello); err != nil {
		t.Fatalf("unmarshal hello: %v", err)
	}
	if hello["type"] != "hello" {
		t.Errorf("expected hello event, got type=%v", hello["type"])
	}

	// Send a message event through the mock Socket Mode server.
	ms.sendMessageEvent("U001", "C100", "test msg", "channel", "")

	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("read event: %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope["type"] != "events_api" {
		t.Errorf("expected events_api, got type=%v", envelope["type"])
	}
	if envelope["envelope_id"] == "" {
		t.Error("expected non-empty envelope_id")
	}
}

func TestIntegration_MockSlackAPIServer(t *testing.T) {
	// Verify the mock Slack API server responds correctly.
	slackAPI := newMockSlackAPIServer()
	srv := httptest.NewServer(slackAPI)
	defer srv.Close()

	api := slack.New("xoxb-test", slack.OptionAPIURL(srv.URL+"/"))

	// Test auth.test.
	info, err := api.AuthTest()
	if err != nil {
		t.Fatalf("AuthTest: %v", err)
	}
	if info.TeamID != "T123" {
		t.Errorf("TeamID = %q, want %q", info.TeamID, "T123")
	}

	// Test users.info.
	user, err := api.GetUserInfo("U001")
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}
	if user.Profile.DisplayName != "alice" {
		t.Errorf("DisplayName = %q, want %q", user.Profile.DisplayName, "alice")
	}

	// Test chat.postMessage.
	_, _, err = api.PostMessage("C100", slack.MsgOptionText("hello", false))
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	calls := slackAPI.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].channel != "C100" {
		t.Errorf("channel = %q, want %q", calls[0].channel, "C100")
	}
}
