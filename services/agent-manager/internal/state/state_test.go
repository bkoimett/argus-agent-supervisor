package state

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionStatusConstants(t *testing.T) {
	statuses := []SessionStatus{
		SessionStatusStarting,
		SessionStatusRunning,
		SessionStatusWaiting,
		SessionStatusParked,
		SessionStatusPaused,
		SessionStatusCompleted,
		SessionStatusError,
		SessionStatusCrashed,
		SessionStatusDisconnected,
		SessionStatusStopped,
	}

	for _, s := range statuses {
		if s == "" {
			t.Errorf("SessionStatus should not be empty")
		}
	}
}

func TestTaskStatusConstants(t *testing.T) {
	statuses := []TaskStatus{
		TaskStatusPending,
		TaskStatusRunning,
		TaskStatusParked,
		TaskStatusRecovering,
		TaskStatusCompleted,
		TaskStatusFailed,
		TaskStatusSkipped,
	}

	for _, s := range statuses {
		if s == "" {
			t.Errorf("TaskStatus should not be empty")
		}
	}
}

func TestIncidentStatusConstants(t *testing.T) {
	statuses := []IncidentStatus{
		IncidentStatusDetected,
		IncidentStatusDiagnosing,
		IncidentStatusProposed,
		IncidentStatusApproved,
		IncidentStatusRecovering,
		IncidentStatusVerified,
		IncidentStatusFailed,
		IncidentStatusRejected,
	}

	for _, s := range statuses {
		if s == "" {
			t.Errorf("IncidentStatus should not be empty")
		}
	}
}

func TestApprovalStatusConstants(t *testing.T) {
	statuses := []ApprovalStatus{
		ApprovalStatusPending,
		ApprovalStatusApproved,
		ApprovalStatusRejected,
		ApprovalStatusExpired,
	}

	for _, s := range statuses {
		if s == "" {
			t.Errorf("ApprovalStatus should not be empty")
		}
	}
}

func TestSessionSerialization(t *testing.T) {
	session := &Session{
		ID:         "test-session",
		ProjectDir: "/tmp/test",
		Agent:      "opencode",
		Model:      "anthropic/claude-3",
		Status:     SessionStatusRunning,
		PID:        12345,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Failed to marshal session: %v", err)
	}

	var decoded Session
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal session: %v", err)
	}

	if decoded.ID != session.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, session.ID)
	}
	if decoded.Status != session.Status {
		t.Errorf("Status mismatch: got %s, want %s", decoded.Status, session.Status)
	}
}

func TestEventSerialization(t *testing.T) {
	event := &Event{
		SessionID: "test-session",
		Type:      "tool_use",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"tool":   "bash",
			"callID": "call-123",
			"input":  map[string]string{"command": "ls"},
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	if decoded.SessionID != event.SessionID {
		t.Errorf("SessionID mismatch")
	}
	if decoded.Type != event.Type {
		t.Errorf("Type mismatch")
	}
}
