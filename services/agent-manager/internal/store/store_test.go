package store

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

func TestStoreCRUD(t *testing.T) {
	// Create temp database
	dbPath := "/tmp/test_store_" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(dbPath)

	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	// Test session CRUD
	session := &state.Session{
		ID:         "test-session-1",
		ProjectDir: "/tmp/test",
		Agent:      "opencode",
		Model:      "test-model",
		Status:     state.SessionStatusRunning,
		PID:        12345,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := st.CreateSession(session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	retrieved, err := st.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.ID != session.ID {
		t.Errorf("Session ID mismatch: got %s, want %s", retrieved.ID, session.ID)
	}
	if retrieved.Status != session.Status {
		t.Errorf("Session status mismatch: got %s, want %s", retrieved.Status, session.Status)
	}

	// Update session
	session.Status = state.SessionStatusCompleted
	now := time.Now()
	session.EndedAt = &now
	exitCode := 0
	session.ExitCode = &exitCode
	if err := st.UpdateSession(session); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	retrieved, err = st.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession after update failed: %v", err)
	}
	if retrieved.Status != state.SessionStatusCompleted {
		t.Errorf("Session status not updated: got %s, want %s", retrieved.Status, state.SessionStatusCompleted)
	}

	// Test task CRUD
	task := &state.Task{
		ID:           "test-task-1",
		SessionID:    session.ID,
		Prompt:       "test prompt",
		Status:       state.TaskStatusRunning,
		Dependencies: []string{},
		Scope:        "test",
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := st.CreateTask(task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	retrievedTask, err := st.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if retrievedTask.ID != task.ID {
		t.Errorf("Task ID mismatch")
	}

	// Test incident CRUD
	incident := &state.Incident{
		ID:               "test-incident-1",
		SessionID:        session.ID,
		TaskID:           &task.ID,
		Type:             "test_error",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"error": "test error"}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}

	if err := st.CreateIncident(incident); err != nil {
		t.Fatalf("CreateIncident failed: %v", err)
	}

	retrievedIncident, err := st.GetIncident(incident.ID)
	if err != nil {
		t.Fatalf("GetIncident failed: %v", err)
	}
	if retrievedIncident.ID != incident.ID {
		t.Errorf("Incident ID mismatch")
	}

	// Test approval CRUD
	approval := &state.Approval{
		ID:          "test-approval-1",
		SessionID:   session.ID,
		TaskID:      &task.ID,
		Type:        state.ApprovalTypePermission,
		Status:      state.ApprovalStatusPending,
		Title:       "Test Approval",
		Description: "Test description",
		Payload:     json.RawMessage(`{"tool": "bash"}`),
		Scope:       "test",
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	if err := st.CreateApproval(approval); err != nil {
		t.Fatalf("CreateApproval failed: %v", err)
	}

	retrievedApproval, err := st.GetApproval(approval.ID)
	if err != nil {
		t.Fatalf("GetApproval failed: %v", err)
	}
	if retrievedApproval.ID != approval.ID {
		t.Errorf("Approval ID mismatch")
	}

	// Test event append and retrieval
	event := &state.Event{
		SessionID: session.ID,
		Type:      "tool_use",
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"tool": "bash"},
	}

	if err := st.AppendEvent(event); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}

	events, err := st.GetEvents(session.ID, 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if events[0].SessionID != session.ID {
		t.Errorf("Event session ID mismatch")
	}
}

func TestStoreListSessions(t *testing.T) {
	dbPath := "/tmp/test_store_list_" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(dbPath)

	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	// Create multiple sessions
	for i := 0; i < 3; i++ {
		session := &state.Session{
			ID:         "session-" + string(rune('a'+i)),
			ProjectDir: "/tmp/test",
			Agent:      "opencode",
			Status:     state.SessionStatusRunning,
			StartedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := st.CreateSession(session); err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	sessions, err := st.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(sessions))
	}
}
