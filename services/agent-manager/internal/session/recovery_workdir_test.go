package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/argus-command-center/argus/services/agent-manager/internal/subprocess"
	"github.com/google/uuid"
)

// setupTestManager creates a manager with a temporary store and project directory.
func setupTestManager(t *testing.T) (*Manager, *store.Store, string) {
	dbPath := "/tmp/test_wd_" + time.Now().Format("20060102150405.000000000") + ".db"
	t.Cleanup(func() { os.Remove(dbPath) })

	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	procConfig := subprocess.OpenCodeConfig{BinaryPath: "opencode"}
	procMgr := subprocess.NewProcessManager(st, procConfig)
	metaConfig := meta.DefaultConfig()
	mgr := NewManager(st, procMgr, metaConfig)

	projDir := "/tmp/test-wd-" + time.Now().Format("20060102150405.000000000")
	t.Cleanup(func() { os.RemoveAll(projDir) })
	os.MkdirAll(projDir, 0755)

	return mgr, st, projDir
}

// TestRecoveryExecutionUsesProjectDir verifies that the recovery verification command
// runs in the session's project directory.
func TestRecoveryExecutionUsesProjectDir(t *testing.T) {
	mgr, _, projDir := setupTestManager(t)

	// Write a minimal Go module with a test that checks the current working directory.
	goMod := `module testproj

go 1.22
`
	if err := os.WriteFile(filepath.Join(projDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	testFile := fmt.Sprintf(`package testproj

import (
	"os"
	"testing"
)

func TestCwd(t *testing.T) {
	expected := %q
	got, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error: %%v", err)
	}
	if got != expected {
		t.Errorf("cwd = %%q, want %%q", got, expected)
	}
}
`, projDir)
	if err := os.WriteFile(filepath.Join(projDir, "cwd_test.go"), []byte(testFile), 0644); err != nil {
		t.Fatalf("write cwd_test.go: %v", err)
	}

	ctx := context.Background()
	session, task, err := mgr.CreateSession(ctx, projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create an incident
	incident := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        session.ID,
		TaskID:           &task.ID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output": "FAIL", "exit": 1}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	if err := mgr.CreateIncident(incident); err != nil {
		t.Fatalf("CreateIncident failed: %v", err)
	}

	// Diagnose to produce a recovery action (RETRY with go test ./...)
	if err := mgr.DiagnoseIncident(incident.ID); err != nil {
		t.Fatalf("DiagnoseIncident failed: %v", err)
	}

	actions, err := mgr.ListRecoveryActions(incident.ID)
	if err != nil {
		t.Fatalf("ListRecoveryActions failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 recovery action, got %d", len(actions))
	}
	recoveryAction := actions[0]

	// Request approval for the recovery action
	approval, err := mgr.RequestRecoveryApproval(recoveryAction.ID)
	if err != nil {
		t.Fatalf("RequestRecoveryApproval failed: %v", err)
	}

	// Approve it
	if err := mgr.RespondApproval(approval.ID, true, "test-user"); err != nil {
		t.Fatalf("RespondApproval failed: %v", err)
	}

	// Execute recovery – this runs the verification command (go test ./...)
	result, err := mgr.ExecuteRecovery(recoveryAction.ID)
	if err != nil {
		t.Fatalf("ExecuteRecovery failed: %v", err)
	}

	// The verification should pass because the test in the project directory asserts the cwd.
	if result.Status != state.VerificationStatusPassed {
		t.Errorf("expected verification PASSED, got %s: %s", result.Status, result.Notes)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}
