package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	commonpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/common"
	connectorpb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/connector"
	controlplanepb "buf.build/gen/go/thekite/kite/protocolbuffers/go/spark/v1/controlplane"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	gmailclient "github.com/kite-production/spark/services/connector-gmail/internal/gmail"
	"github.com/kite-production/spark/services/connector-gmail/internal/normalizer"
	"github.com/kite-production/spark/services/connector-gmail/internal/service"
	baseconnector "github.com/kite-production/spark/services/cross-service/connector"
	gm "google.golang.org/api/gmail/v1"
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
// Mock Gmail API (in-memory, implements gmail.GmailAPI)
// ---------------------------------------------------------------------------

// storedMessage represents an email in the mock Gmail store.
type storedMessage struct {
	ID        string
	ThreadID  string
	HistoryID uint64
	From      string
	Subject   string
	Body      string
}

// sentEmail records a send call made through the mock API.
type sentEmail struct {
	UserID   string
	Raw      string
	ThreadID string
}

type fakeGmailAPI struct {
	mu      sync.Mutex
	msgs    []storedMessage
	sent    []sentEmail
	sentSeq int
}

func newFakeGmailAPI() *fakeGmailAPI {
	return &fakeGmailAPI{}
}

func (f *fakeGmailAPI) addMessage(msg storedMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, msg)
}

func (f *fakeGmailAPI) clearMessages() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = nil
}

func (f *fakeGmailAPI) getSent() []sentEmail {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentEmail, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeGmailAPI) ListMessages(_ context.Context, _, _ string) ([]*gm.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []*gm.Message
	for _, m := range f.msgs {
		result = append(result, &gm.Message{
			Id:       m.ID,
			ThreadId: m.ThreadID,
		})
	}
	return result, nil
}

func (f *fakeGmailAPI) GetMessage(_ context.Context, _, messageID string) (*gm.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, m := range f.msgs {
		if m.ID == messageID {
			bodyData := base64.URLEncoding.EncodeToString([]byte(m.Body))
			return &gm.Message{
				Id:        m.ID,
				ThreadId:  m.ThreadID,
				HistoryId: m.HistoryID,
				Payload: &gm.MessagePart{
					MimeType: "text/plain",
					Headers: []*gm.MessagePartHeader{
						{Name: "From", Value: m.From},
						{Name: "Subject", Value: m.Subject},
					},
					Body: &gm.MessagePartBody{Data: bodyData},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("message %s not found", messageID)
}

func (f *fakeGmailAPI) SendMessage(_ context.Context, userID string, msg *gm.Message) (*gm.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sentSeq++
	f.sent = append(f.sent, sentEmail{
		UserID:   userID,
		Raw:      msg.Raw,
		ThreadID: msg.ThreadId,
	})

	return &gm.Message{
		Id:       fmt.Sprintf("sent_%d", f.sentSeq),
		ThreadId: msg.ThreadId,
	}, nil
}

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type testHarness struct {
	gmailAPI   *fakeGmailAPI
	base       *baseconnector.BaseConnector
	svc        *service.Service
	publisher  *fakePublisher
	cpClient   *fakeCPClient
	grpcConn   *grpc.ClientConn
	grpcClient connectorpb.ConnectorServiceClient
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	// Create fake publisher and CP client.
	pub := &fakePublisher{}
	cp := &fakeCPClient{}

	// Create BaseConnector with injected fakes.
	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "gmail",
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
	base.ConnectNATS = func(_ baseconnector.Config) (*nats.Conn, baseconnector.NATSPublisher, error) {
		return nil, pub, nil
	}
	base.DialCP = func(_ string) (*grpc.ClientConn, baseconnector.ControlPlaneClient, error) {
		return nil, cp, nil
	}

	// Create metrics with isolated registry.
	reg := prometheus.NewRegistry()
	metrics := service.NewMetrics(reg)

	// Create normalizer and mock Gmail API.
	norm := normalizer.New("gmail", "test@example.com")
	gmailAPI := newFakeGmailAPI()

	// Create service.
	svc := service.New(base, norm, gmailAPI, "me", metrics)

	// Start BaseConnector (connects fakes, registers).
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
		gmailAPI:   gmailAPI,
		base:       base,
		svc:        svc,
		publisher:  pub,
		cpClient:   cp,
		grpcConn:   conn,
		grpcClient: client,
	}

	t.Cleanup(func() {
		conn.Close()
		base.GRPCServer.Stop()
	})

	return h
}

// makeGmailMessage creates a *gm.Message with common test fields.
func makeGmailMessage(id, threadID string, historyID uint64, from, subject, body string) *gm.Message {
	bodyData := base64.URLEncoding.EncodeToString([]byte(body))
	return &gm.Message{
		Id:        id,
		ThreadId:  threadID,
		HistoryId: historyID,
		Payload: &gm.MessagePart{
			MimeType: "text/plain",
			Headers: []*gm.MessagePartHeader{
				{Name: "From", Value: from},
				{Name: "Subject", Value: subject},
			},
			Body: &gm.MessagePartBody{Data: bodyData},
		},
	}
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

func TestIntegration_Registration(t *testing.T) {
	h := newTestHarness(t)

	regCalls := h.cpClient.getRegisterCalls()
	if len(regCalls) != 1 {
		t.Fatalf("expected 1 register call, got %d", len(regCalls))
	}
	if regCalls[0].ConnectorId != "gmail" {
		t.Errorf("registered connector_id = %q, want %q", regCalls[0].ConnectorId, "gmail")
	}
	if !h.base.IsRegistered() {
		t.Error("expected IsRegistered=true")
	}
	if !h.base.IsHealthy() {
		t.Error("expected IsHealthy=true after start")
	}
}

func TestIntegration_Deregistration(t *testing.T) {
	pub := &fakePublisher{}
	cp := &fakeCPClient{}

	base := baseconnector.New(baseconnector.Config{
		ConnectorID:         "gmail",
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
	norm := normalizer.New("gmail", "test@example.com")
	api := newFakeGmailAPI()
	svc := service.New(base, norm, api, "me", metrics)

	if err := base.Start(context.Background(), svc); err != nil {
		t.Fatalf("base.Start: %v", err)
	}

	if !base.IsRegistered() {
		t.Fatal("expected registered=true after start")
	}

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
	if deregCalls[0].ConnectorId != "gmail" {
		t.Errorf("deregistered connector_id = %q, want %q", deregCalls[0].ConnectorId, "gmail")
	}
}

func TestIntegration_InboundMessage(t *testing.T) {
	h := newTestHarness(t)

	// Simulate the poller calling HandleMessage with a Gmail message.
	h.svc.HandleMessage(makeGmailMessage("msg1", "thread1", 1000,
		"Alice <alice@example.com>", "Hello from Gmail", "Hi there!"))

	msgs := h.publisher.getPublished()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}

	if msgs[0].subject != "spark.connector.inbound.gmail" {
		t.Errorf("subject = %q, want %q", msgs[0].subject, "spark.connector.inbound.gmail")
	}

	var inbound connectorpb.InboundMessage
	if err := proto.Unmarshal(msgs[0].data, &inbound); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if inbound.ConnectorId != "gmail" {
		t.Errorf("ConnectorId = %q, want %q", inbound.ConnectorId, "gmail")
	}
	if inbound.Text != "Hi there!" {
		t.Errorf("Text = %q, want %q", inbound.Text, "Hi there!")
	}
	if inbound.PeerKind != commonpb.PeerKind_PEER_KIND_DM {
		t.Errorf("PeerKind = %v, want PEER_KIND_DM", inbound.PeerKind)
	}
	if inbound.PeerId != "alice@example.com" {
		t.Errorf("PeerId = %q, want %q", inbound.PeerId, "alice@example.com")
	}
	if inbound.ThreadId != "thread1" {
		t.Errorf("ThreadId = %q, want %q", inbound.ThreadId, "thread1")
	}
	if inbound.Sender.SenderId != "alice@example.com" {
		t.Errorf("SenderId = %q, want %q", inbound.Sender.SenderId, "alice@example.com")
	}
	if inbound.Sender.SenderName != "Alice" {
		t.Errorf("SenderName = %q, want %q", inbound.Sender.SenderName, "Alice")
	}
	if inbound.IdempotencyKey.Value != "gmail:test@example.com:msg1" {
		t.Errorf("IdempotencyKey = %q, want %q", inbound.IdempotencyKey.Value, "gmail:test@example.com:msg1")
	}
}

func TestIntegration_PollerDetectsNewMessages(t *testing.T) {
	h := newTestHarness(t)

	// Add messages to the mock API.
	h.gmailAPI.addMessage(storedMessage{
		ID:        "msg_poll_1",
		ThreadID:  "thread_poll_1",
		HistoryID: 2000,
		From:      "Bob <bob@example.com>",
		Subject:   "Polling Test",
		Body:      "First email",
	})
	h.gmailAPI.addMessage(storedMessage{
		ID:        "msg_poll_2",
		ThreadID:  "thread_poll_2",
		HistoryID: 2001,
		From:      "Carol <carol@example.com>",
		Subject:   "Another Email",
		Body:      "Second email",
	})

	// Create a poller that feeds messages to the service's HandleMessage.
	poller := gmailclient.NewPoller(h.gmailAPI, "me", 50*time.Millisecond, h.svc.HandleMessage)

	// Run the poller briefly — it does an immediate poll on start.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go poller.Run(ctx)
	<-ctx.Done()

	msgs := h.publisher.getPublished()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 published messages, got %d", len(msgs))
	}

	// Verify first two messages contain the expected content.
	var texts []string
	for _, msg := range msgs[:2] {
		var inbound connectorpb.InboundMessage
		if err := proto.Unmarshal(msg.data, &inbound); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		texts = append(texts, inbound.Text)
	}

	if texts[0] != "First email" {
		t.Errorf("msg 0: Text = %q, want %q", texts[0], "First email")
	}
	if texts[1] != "Second email" {
		t.Errorf("msg 1: Text = %q, want %q", texts[1], "Second email")
	}
}

func TestIntegration_HistoryIDTracking(t *testing.T) {
	api := newFakeGmailAPI()
	api.addMessage(storedMessage{
		ID:        "msg_hist_1",
		ThreadID:  "thread_hist_1",
		HistoryID: 5000,
		From:      "alice@example.com",
		Subject:   "History Test",
		Body:      "First poll body",
	})

	var handled []*gm.Message
	var handledMu sync.Mutex

	poller := gmailclient.NewPoller(api, "me", time.Hour, func(msg *gm.Message) {
		handledMu.Lock()
		handled = append(handled, msg)
		handledMu.Unlock()
	})

	// Run the poller briefly (it does an immediate poll on start).
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go poller.Run(ctx)
	<-ctx.Done()

	handledMu.Lock()
	firstPollCount := len(handled)
	handledMu.Unlock()

	if firstPollCount != 1 {
		t.Fatalf("first poll: expected 1 handled message, got %d", firstPollCount)
	}

	if hid := poller.LastHistoryID(); hid != 5000 {
		t.Errorf("LastHistoryID = %d, want 5000", hid)
	}

	// Add a second message with higher historyId.
	api.addMessage(storedMessage{
		ID:        "msg_hist_2",
		ThreadID:  "thread_hist_2",
		HistoryID: 6000,
		From:      "bob@example.com",
		Subject:   "Second History",
		Body:      "Second poll body",
	})

	// Create a fresh poller to simulate second poll cycle.
	handled = nil
	poller2 := gmailclient.NewPoller(api, "me", time.Hour, func(msg *gm.Message) {
		handledMu.Lock()
		handled = append(handled, msg)
		handledMu.Unlock()
	})

	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	go poller2.Run(ctx2)
	<-ctx2.Done()

	if hid := poller2.LastHistoryID(); hid < 6000 {
		t.Errorf("LastHistoryID = %d, want >= 6000", hid)
	}
}

func TestIntegration_HistoryIDNoReprocessing(t *testing.T) {
	// Verify that the poller's historyId only increases (never decreases),
	// which prevents reprocessing of already-seen messages.
	api := newFakeGmailAPI()
	api.addMessage(storedMessage{
		ID: "m1", ThreadID: "t1", HistoryID: 100,
		From: "a@example.com", Subject: "S1", Body: "B1",
	})
	api.addMessage(storedMessage{
		ID: "m2", ThreadID: "t2", HistoryID: 200,
		From: "b@example.com", Subject: "S2", Body: "B2",
	})
	api.addMessage(storedMessage{
		ID: "m3", ThreadID: "t3", HistoryID: 50, // Lower historyId
		From: "c@example.com", Subject: "S3", Body: "B3",
	})

	poller := gmailclient.NewPoller(api, "me", time.Hour, func(_ *gm.Message) {})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go poller.Run(ctx)
	<-ctx.Done()

	// HistoryId should be the max seen (200), not the last processed (50).
	if hid := poller.LastHistoryID(); hid != 200 {
		t.Errorf("LastHistoryID = %d, want 200 (max seen)", hid)
	}
}

func TestIntegration_SendMessage_MIME(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	resp, err := h.grpcClient.SendMessage(ctx, &connectorpb.SendMessageRequest{
		PeerId: "bob@example.com",
		Text:   "Hello Bob, this is a test email!",
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

	// Verify the mock Gmail API received the send call.
	sent := h.gmailAPI.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 send call, got %d", len(sent))
	}

	// Decode the raw MIME email.
	rawBytes, err := base64.URLEncoding.DecodeString(sent[0].Raw)
	if err != nil {
		t.Fatalf("decode raw email: %v", err)
	}
	rawEmail := string(rawBytes)

	if !strings.Contains(rawEmail, "To: bob@example.com") {
		t.Errorf("MIME email missing To header, got:\n%s", rawEmail)
	}
	if !strings.Contains(rawEmail, "Hello Bob, this is a test email!") {
		t.Errorf("MIME email missing body, got:\n%s", rawEmail)
	}
	if !strings.Contains(rawEmail, "MIME-Version: 1.0") {
		t.Errorf("MIME email missing MIME-Version header, got:\n%s", rawEmail)
	}
	if !strings.Contains(rawEmail, "Content-Type: text/plain") {
		t.Errorf("MIME email missing Content-Type header, got:\n%s", rawEmail)
	}
}

func TestIntegration_SendMessage_ReplyWithSubjectPrefix(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Handle an inbound message to cache the thread subject.
	h.svc.HandleMessage(makeGmailMessage("msg_reply_1", "thread_reply",
		3000, "alice@example.com", "Original Topic", "initial email body"))

	// Send a reply in the same thread.
	resp, err := h.grpcClient.SendMessage(ctx, &connectorpb.SendMessageRequest{
		PeerId:   "alice@example.com",
		Text:     "This is my reply",
		ThreadId: "thread_reply",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.MessageId == "" {
		t.Error("expected non-empty MessageId")
	}

	// Verify the sent email has "Re: " prefix.
	sent := h.gmailAPI.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 send call, got %d", len(sent))
	}

	rawBytes, err := base64.URLEncoding.DecodeString(sent[0].Raw)
	if err != nil {
		t.Fatalf("decode raw email: %v", err)
	}
	rawEmail := string(rawBytes)

	if !strings.Contains(rawEmail, "Subject: Re: Original Topic") {
		t.Errorf("expected 'Re: Original Topic' subject, got:\n%s", rawEmail)
	}
	if !strings.Contains(rawEmail, "This is my reply") {
		t.Errorf("expected reply body, got:\n%s", rawEmail)
	}
}

func TestIntegration_SendMessage_ThreadID(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	_, err := h.grpcClient.SendMessage(ctx, &connectorpb.SendMessageRequest{
		PeerId:   "alice@example.com",
		Text:     "Thread reply",
		ThreadId: "thread_xyz",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	sent := h.gmailAPI.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(sent))
	}
	if sent[0].ThreadID != "thread_xyz" {
		t.Errorf("ThreadID = %q, want %q", sent[0].ThreadID, "thread_xyz")
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
	if resp.Service != "gmail" {
		t.Errorf("Service = %q, want %q", resp.Service, "gmail")
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
	if !caps.SupportsReply {
		t.Error("expected SupportsReply=true")
	}
	if !caps.SupportsAttachments {
		t.Error("expected SupportsAttachments=true")
	}
	if !caps.SupportsImages {
		t.Error("expected SupportsImages=true")
	}
	if caps.MaxMessageLength != 0 {
		t.Errorf("MaxMessageLength = %d, want 0", caps.MaxMessageLength)
	}
}

func TestIntegration_GetStatusViaGRPC(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()
	h.svc.SetPolling(true)

	resp, err := h.grpcClient.GetStatus(ctx, &connectorpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.ConnectorId != "gmail" {
		t.Errorf("ConnectorId = %q, want %q", resp.ConnectorId, "gmail")
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

	messages := []struct {
		id       string
		threadID string
		from     string
		subject  string
		body     string
	}{
		{"m1", "t1", "Alice <alice@example.com>", "First", "body one"},
		{"m2", "t2", "Bob <bob@example.com>", "Second", "body two"},
		{"m3", "t3", "Carol <carol@example.com>", "Third", "body three"},
	}

	for _, m := range messages {
		h.svc.HandleMessage(makeGmailMessage(m.id, m.threadID, 1000, m.from, m.subject, m.body))
	}

	msgs := h.publisher.getPublished()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 published messages, got %d", len(msgs))
	}

	expectedTexts := []string{"body one", "body two", "body three"}
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

func TestIntegration_MockGmailAPI(t *testing.T) {
	api := newFakeGmailAPI()
	api.addMessage(storedMessage{
		ID:        "test_msg_1",
		ThreadID:  "test_thread_1",
		HistoryID: 100,
		From:      "Test <test@example.com>",
		Subject:   "Test Subject",
		Body:      "Test body content",
	})

	ctx := context.Background()

	// Test ListMessages.
	listed, err := api.ListMessages(ctx, "me", "is:unread")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 message, got %d", len(listed))
	}
	if listed[0].Id != "test_msg_1" {
		t.Errorf("Id = %q, want %q", listed[0].Id, "test_msg_1")
	}

	// Test GetMessage.
	full, err := api.GetMessage(ctx, "me", "test_msg_1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if full.Id != "test_msg_1" {
		t.Errorf("Id = %q, want %q", full.Id, "test_msg_1")
	}
	if full.ThreadId != "test_thread_1" {
		t.Errorf("ThreadId = %q, want %q", full.ThreadId, "test_thread_1")
	}
	if full.HistoryId != 100 {
		t.Errorf("HistoryId = %d, want 100", full.HistoryId)
	}

	// Test SendMessage.
	sentMsg, err := api.SendMessage(ctx, "me", &gm.Message{
		Raw:      base64.URLEncoding.EncodeToString([]byte("raw email")),
		ThreadId: "test_thread_1",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sentMsg.Id == "" {
		t.Error("expected non-empty sent message ID")
	}

	sentEmails := api.getSent()
	if len(sentEmails) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(sentEmails))
	}
	if sentEmails[0].UserID != "me" {
		t.Errorf("UserID = %q, want %q", sentEmails[0].UserID, "me")
	}
}
