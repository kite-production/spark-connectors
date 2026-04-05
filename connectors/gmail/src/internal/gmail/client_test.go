package gmail

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	gm "google.golang.org/api/gmail/v1"
)

// mockGmailAPI implements GmailAPI for tests.
type mockGmailAPI struct {
	messages    []*gm.Message
	fullMsgs    map[string]*gm.Message
	sentMsgs    []*gm.Message
	listErr     error
	getErr      error
	sendErr     error
	listCalls   atomic.Int32
}

func (m *mockGmailAPI) ListMessages(_ context.Context, _, _ string) ([]*gm.Message, error) {
	m.listCalls.Add(1)
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.messages, nil
}

func (m *mockGmailAPI) GetMessage(_ context.Context, _, messageID string) (*gm.Message, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if msg, ok := m.fullMsgs[messageID]; ok {
		return msg, nil
	}
	return &gm.Message{Id: messageID, HistoryId: 100}, nil
}

func (m *mockGmailAPI) SendMessage(_ context.Context, _ string, msg *gm.Message) (*gm.Message, error) {
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return &gm.Message{Id: "sent123"}, nil
}

func TestPoller_PollsOnInterval(t *testing.T) {
	api := &mockGmailAPI{
		messages: []*gm.Message{},
	}

	var callCount atomic.Int32
	poller := NewPoller(api, "me", 50*time.Millisecond, func(msg *gm.Message) {
		callCount.Add(1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = poller.Run(ctx)

	// Should have called list at least 2 times (initial + at least 1 tick).
	calls := api.listCalls.Load()
	if calls < 2 {
		t.Errorf("expected at least 2 list calls, got %d", calls)
	}
}

func TestPoller_ProcessesMessages(t *testing.T) {
	api := &mockGmailAPI{
		messages: []*gm.Message{
			{Id: "msg1"},
			{Id: "msg2"},
		},
		fullMsgs: map[string]*gm.Message{
			"msg1": {Id: "msg1", HistoryId: 100},
			"msg2": {Id: "msg2", HistoryId: 200},
		},
	}

	var received []*gm.Message
	poller := NewPoller(api, "me", 1*time.Hour, func(msg *gm.Message) {
		received = append(received, msg)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = poller.Run(ctx)

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
	if received[0].Id != "msg1" {
		t.Errorf("first message ID = %q, want %q", received[0].Id, "msg1")
	}
}

func TestPoller_TracksHistoryID(t *testing.T) {
	api := &mockGmailAPI{
		messages: []*gm.Message{{Id: "msg1"}},
		fullMsgs: map[string]*gm.Message{
			"msg1": {Id: "msg1", HistoryId: 42},
		},
	}

	poller := NewPoller(api, "me", 1*time.Hour, func(msg *gm.Message) {})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = poller.Run(ctx)

	if poller.LastHistoryID() != 42 {
		t.Errorf("LastHistoryID() = %d, want 42", poller.LastHistoryID())
	}
}

func TestPoller_HistoryIDOnlyIncreases(t *testing.T) {
	api := &mockGmailAPI{
		messages: []*gm.Message{{Id: "msg1"}, {Id: "msg2"}},
		fullMsgs: map[string]*gm.Message{
			"msg1": {Id: "msg1", HistoryId: 200},
			"msg2": {Id: "msg2", HistoryId: 100},
		},
	}

	poller := NewPoller(api, "me", 1*time.Hour, func(msg *gm.Message) {})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = poller.Run(ctx)

	if poller.LastHistoryID() != 200 {
		t.Errorf("LastHistoryID() = %d, want 200 (should keep highest)", poller.LastHistoryID())
	}
}

func TestParseHistoryID(t *testing.T) {
	tests := []struct {
		input   string
		want    uint64
		wantErr bool
	}{
		{"12345", 12345, false},
		{"0", 0, false},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseHistoryID(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseHistoryID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("ParseHistoryID(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
