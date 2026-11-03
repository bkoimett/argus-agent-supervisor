package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/adapter"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

// mockCommandStore records commands and events for testing.
type mockCommandStore struct {
	mu         sync.Mutex
	commands   []*state.Command
	events     []*state.Event
	syncCalls  int
	eventCalls int
}

func (m *mockCommandStore) ListPendingCommands(ctx context.Context) ([]*state.Command, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*state.Command, len(m.commands))
	copy(out, m.commands)
	return out, nil
}

func (m *mockCommandStore) SyncCommand(ctx context.Context, cmd *state.Command) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCalls++
	found := false
	for i, c := range m.commands {
		if c.ID == cmd.ID {
			m.commands[i] = cmd
			found = true
			break
		}
	}
	if !found {
		m.commands = append(m.commands, cmd)
	}
	return nil
}

func (m *mockCommandStore) SyncEvent(ctx context.Context, evt *state.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventCalls++
	m.events = append(m.events, evt)
	return nil
}

func (m *mockCommandStore) findCommand(sessionID string) *state.Command {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.commands {
		if c.SessionID == sessionID {
			return c
		}
	}
	return nil
}

// mockAdapter is a test double for OpenCodeAdapter.
type mockAdapter struct {
	mu          sync.Mutex
	sendCalled  bool
	stopCalled  bool
	lastSession string
	lastMessage string
	sendResult  adapter.MessageResult
	sendErr     error
	stopErr     error
}

func (m *mockAdapter) SendMessage(sessionID, message string) (adapter.MessageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalled = true
	m.lastSession = sessionID
	m.lastMessage = message
	return m.sendResult, m.sendErr
}

func (m *mockAdapter) Stop(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
	return m.stopErr
}

func (m *mockAdapter) Interrupt(sessionID string) error { return nil }

// Unused methods to satisfy AgentAdapter interface.
func (m *mockAdapter) Start(string) (string, error) { return "", nil }
func (m *mockAdapter) Restart(string) error         { return nil }
func (m *mockAdapter) Status(string) (adapter.Status, error) {
	return adapter.StatusWorking, nil
}
func (m *mockAdapter) Abort(string) error { return nil }
func (m *mockAdapter) RespondToPermission(string, string, bool) error {
	return nil
}
func (m *mockAdapter) GetEvents(string) ([]adapter.Event, error) {
	return nil, nil
}
func (m *mockAdapter) Close() error { return nil }

var _ adapter.AgentAdapter = (*mockAdapter)(nil)

func TestProcessCommandSendMessage(t *testing.T) {
	store := &mockCommandStore{
		commands: []*state.Command{
			{
				ID:        "cmd-1",
				SessionID: "ses_test_1",
				Type:      "send_message",
				Payload:   json.RawMessage(`{"message":"hello from phone"}`),
				Status:    "PENDING",
				CreatedAt: time.Now(),
			},
		},
	}
	adptr := &mockAdapter{
		sendResult: adapter.MessageResult{
			MessageID: "msg_1",
			SessionID: "ses_test_1",
			Text:      "assistant response",
			Finish:    "stop",
		},
	}

	processCommand(store, adptr, store.commands[0])

	if !adptr.sendCalled {
		t.Error("expected SendMessage to be called")
	}
	if adptr.lastSession != "ses_test_1" {
		t.Errorf("expected session ses_test_1, got %s", adptr.lastSession)
	}
	if adptr.lastMessage != "hello from phone" {
		t.Errorf("expected message 'hello from phone', got %s", adptr.lastMessage)
	}
	if store.eventCalls < 2 {
		t.Errorf("expected 2 SyncEvent calls (user + assistant), got %d", store.eventCalls)
	}
	if store.syncCalls < 1 {
		t.Errorf("expected SyncCommand to be called, got %d calls", store.syncCalls)
	}
	cmd := store.findCommand("ses_test_1")
	if cmd == nil {
		t.Fatal("expected command to be updated")
	}
	if cmd.Status != "EXECUTED" {
		t.Errorf("expected command status EXECUTED, got %s", cmd.Status)
	}
}

func TestProcessCommandStop(t *testing.T) {
	store := &mockCommandStore{
		commands: []*state.Command{
			{
				ID:        "cmd-stop-1",
				SessionID: "ses_test_2",
				Type:      "stop",
				Payload:   json.RawMessage(`{}`),
				Status:    "PENDING",
				CreatedAt: time.Now(),
			},
		},
	}
	adptr := &mockAdapter{}

	processCommand(store, adptr, store.commands[0])

	if !adptr.stopCalled {
		t.Error("expected Stop to be called")
	}
	cmd := store.findCommand("ses_test_2")
	if cmd == nil {
		t.Fatal("expected command to be updated")
	}
	if cmd.Status != "EXECUTED" {
		t.Errorf("expected command status EXECUTED, got %s", cmd.Status)
	}
}

func TestProcessCommandUnknownType(t *testing.T) {
	store := &mockCommandStore{
		commands: []*state.Command{
			{
				ID:        "cmd-unk-1",
				SessionID: "ses_test_3",
				Type:      "frobnicate",
				Payload:   json.RawMessage(`{}`),
				Status:    "PENDING",
				CreatedAt: time.Now(),
			},
		},
	}

	processCommand(store, nil, store.commands[0])

	cmd := store.findCommand("ses_test_3")
	if cmd == nil {
		t.Fatal("expected command to be updated")
	}
	if cmd.Status != "FAILED" {
		t.Errorf("expected command status FAILED, got %s", cmd.Status)
	}
}

func TestProcessCommandInvalidPayload(t *testing.T) {
	store := &mockCommandStore{
		commands: []*state.Command{
			{
				ID:        "cmd-bad-1",
				SessionID: "ses_test_4",
				Type:      "send_message",
				Payload:   json.RawMessage(`not json`),
				Status:    "PENDING",
				CreatedAt: time.Now(),
			},
		},
	}
	adptr := &mockAdapter{}

	processCommand(store, adptr, store.commands[0])

	if adptr.sendCalled {
		t.Error("expected SendMessage NOT to be called for invalid payload")
	}
	cmd := store.findCommand("ses_test_4")
	if cmd == nil {
		t.Fatal("expected command to be updated")
	}
	if cmd.Status != "FAILED" {
		t.Errorf("expected command status FAILED, got %s", cmd.Status)
	}
}

func TestProcessCommandSendMessageAdapterError(t *testing.T) {
	store := &mockCommandStore{
		commands: []*state.Command{
			{
				ID:        "cmd-err-1",
				SessionID: "ses_test_5",
				Type:      "send_message",
				Payload:   json.RawMessage(`{"message":"test"}`),
				Status:    "PENDING",
				CreatedAt: time.Now(),
			},
		},
	}
	adptr := &mockAdapter{sendErr: errTestFailure}

	processCommand(store, adptr, store.commands[0])

	if !adptr.sendCalled {
		t.Error("expected SendMessage to be called")
	}
	cmd := store.findCommand("ses_test_5")
	if cmd == nil {
		t.Fatal("expected command to be updated")
	}
	if cmd.Status != "FAILED" {
		t.Errorf("expected command status FAILED, got %s", cmd.Status)
	}
}

var errTestFailure = &testError{msg: "test failure"}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }
