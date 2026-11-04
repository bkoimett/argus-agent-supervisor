package store

import (
	"database/sql"
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

func TestStoreGetEventsOrdering(t *testing.T) {
	dbPath := "/tmp/test_store_events_order_" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(dbPath)

	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	session := &state.Session{
		ID:         "test-session-events",
		ProjectDir: "/tmp/test",
		Agent:      "opencode",
		Status:     state.SessionStatusRunning,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := st.CreateSession(session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	base := time.Now()
	// Insert three events with deliberately out-of-order timestamps to verify
	// GetEvents returns them sorted oldest-first (ASC).
	events := []*state.Event{
		{SessionID: session.ID, Type: "tool_use", Timestamp: base.Add(2 * time.Second), Payload: map[string]interface{}{"seq": 3}},
		{SessionID: session.ID, Type: "user_message", Timestamp: base, Payload: map[string]interface{}{"seq": 1}},
		{SessionID: session.ID, Type: "assistant_message", Timestamp: base.Add(time.Second), Payload: map[string]interface{}{"seq": 2}},
	}
	for _, e := range events {
		if err := st.AppendEvent(e); err != nil {
			t.Fatalf("AppendEvent failed: %v", err)
		}
	}

	got, err := st.GetEvents(session.ID, 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(got))
	}
	// Timestamps must be non-decreasing (oldest first).
	for i := 1; i < len(got); i++ {
		if got[i].Timestamp.Before(got[i-1].Timestamp) {
			t.Errorf("Events not in ascending timestamp order: event %d (%v) before event %d (%v)",
				i-1, got[i-1].Timestamp, i, got[i].Timestamp)
		}
	}
	// Verify the first event is the user_message (earliest timestamp).
	if got[0].Type != "user_message" {
		t.Errorf("First event type = %q, want user_message", got[0].Type)
	}
	if got[2].Type != "tool_use" {
		t.Errorf("Last event type = %q, want tool_use", got[2].Type)
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

func TestMigrationAddsMissingColumns(t *testing.T) {
	// Create a database with old schema (v1) - no supervision_enabled on sessions, no scope on approvals
	dbPath := "/tmp/test_migration_" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(dbPath)

	// Create old schema manually
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	oldSchema := `
	CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		project_dir TEXT NOT NULL,
		agent TEXT NOT NULL DEFAULT 'opencode',
		model TEXT,
		status TEXT NOT NULL,
		pid INTEGER,
		started_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		ended_at DATETIME,
		exit_code INTEGER,
		error_message TEXT
	);

	CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		prompt TEXT NOT NULL,
		status TEXT NOT NULL,
		dependencies TEXT NOT NULL DEFAULT '[]',
		scope TEXT NOT NULL DEFAULT '',
		started_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		ended_at DATETIME,
		exit_code INTEGER,
		error_message TEXT,
		approval_id TEXT REFERENCES approvals(id),
		recovery_count INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE incidents (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
		type TEXT NOT NULL,
		severity TEXT NOT NULL,
		status TEXT NOT NULL,
		evidence TEXT NOT NULL,
		meta_diagnosis TEXT,
		meta_recovery TEXT,
		meta_source TEXT,
		meta_model TEXT,
		detected_at DATETIME NOT NULL,
		diagnosed_at DATETIME,
		proposed_at DATETIME,
		approved_at DATETIME,
		recovered_at DATETIME,
		verified_at DATETIME,
		recovery_attempts INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE approvals (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
		incident_id TEXT REFERENCES incidents(id) ON DELETE SET NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		payload TEXT NOT NULL,
		requested_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		responded_at DATETIME,
		decision INTEGER,
		decided_by TEXT
	);

	CREATE TABLE recovery_actions (
		id TEXT PRIMARY KEY,
		incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		description TEXT NOT NULL,
		command TEXT NOT NULL,
		args TEXT NOT NULL DEFAULT '[]',
		status TEXT NOT NULL,
		proposed_at DATETIME NOT NULL,
		approved_at DATETIME,
		executed_at DATETIME,
		result TEXT
	);

	CREATE TABLE events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		payload TEXT NOT NULL
	);
	`
	if _, err := db.Exec(oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	// Insert test data
	if _, err := db.Exec(`
		INSERT INTO sessions (id, project_dir, agent, model, status, pid, started_at, updated_at)
		VALUES ('sess-1', '/tmp/test', 'opencode', 'test', 'RUNNING', 123, datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO approvals (id, session_id, type, status, title, description, payload, requested_at, expires_at)
		VALUES ('appr-1', 'sess-1', 'PERMISSION', 'PENDING', 'Test', 'Desc', '{}', datetime('now'), datetime('now', '+5 minutes'))
	`); err != nil {
		t.Fatalf("insert approval: %v", err)
	}
	db.Close()

	// Now open with NewStore which should run migrations
	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore after migration failed: %v", err)
	}
	defer st.Close()

	// Verify supervision_enabled column exists and has default value
	session, err := st.GetSession("sess-1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if !session.SupervisionEnabled {
		t.Errorf("Expected SupervisionEnabled=true (default), got false")
	}

	// Verify scope column exists on approvals
	approval, err := st.GetApproval("appr-1")
	if err != nil {
		t.Fatalf("GetApproval failed: %v", err)
	}
	if approval.Scope != "" {
		t.Errorf("Expected Scope='' (default), got %q", approval.Scope)
	}

	// Verify schema_version table was created and version is current
	var version int
	err = st.db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != currentSchemaVersion {
		t.Errorf("Expected schema version %d, got %d", currentSchemaVersion, version)
	}
}
