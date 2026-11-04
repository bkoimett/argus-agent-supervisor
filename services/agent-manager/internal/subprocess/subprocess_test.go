package subprocess

import (
	"os"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
)

func TestTestAgent(t *testing.T) {
	dbPath := "/tmp/test_subprocess_" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(dbPath)

	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	// Create a session first (required for FK constraint)
	session := &state.Session{
		ID:         "test-session",
		ProjectDir: "/tmp/test",
		Agent:      "opencode",
		Status:     state.SessionStatusRunning,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := st.CreateSession(session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	ta := NewTestAgent(st, "test-session")

	// Test basic event emission
	ta.EmitEvent("test_event", map[string]interface{}{"key": "value"})
	events := ta.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != "test_event" {
		t.Errorf("Event type mismatch: got %s", events[0].Type)
	}

	// Test scenario methods
	ta.ScenarioCompileError()
	events = ta.GetEvents()
	t.Logf("TestAgent events count: %d", len(events))
	for i, e := range events {
		t.Logf("Event %d: type=%s", i, e.Type)
	}
	if len(events) < 3 {
		t.Errorf("Expected at least 3 events for compile error scenario, got %d", len(events))
	}

	// Verify events were persisted
	storedEvents, err := st.GetEvents("test-session", 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	t.Logf("Stored events count: %d", len(storedEvents))
	for i, e := range storedEvents {
		t.Logf("Stored event %d: type=%s, session_id=%s", i, e.Type, e.SessionID)
	}
	if len(storedEvents) < 3 {
		t.Errorf("Expected at least 3 stored events, got %d", len(storedEvents))
	}
}

func TestOpenCodeEventConversion(t *testing.T) {
	// This would require a running store, so we'll test the conversion logic separately
	// by creating a minimal test
}
