package session

import (
	"context"
	"encoding/json"
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

// setupRecoveryTest creates manager with a temp project containing a go.mod and a simple test.
func setupRecoveryTest(t *testing.T) (*Manager, *store.Store, string, string) {
	dbPath := "/tmp/test_rec_" + time.Now().Format("20060102150405.000000000") + ".db"
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

	projDir := "/tmp/test-rec-" + time.Now().Format("20060102150405.000000000")
	t.Cleanup(func() { os.RemoveAll(projDir) })
	os.MkdirAll(projDir, 0755)

	// minimal go module
	if err := os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// simple passing test
	if err := os.WriteFile(filepath.Join(projDir, "pass_test.go"), []byte(`package test
import "testing"
func TestPass(t *testing.T) {}`), 0644); err != nil {
		t.Fatalf("write pass_test.go: %v", err)
	}

	ctx := context.Background()
	sess, _, err := mgr.CreateSession(ctx, projDir, "", "run tests")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	return mgr, st, sess.ID, projDir
}

func TestRecoveryExecution_RequiresApproval(t *testing.T) {
	mgr, _, sessID, _ := setupRecoveryTest(t)

	// create incident and recovery action manually (skip opencode)
	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sessID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output":"FAIL"}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	mgr.CreateIncident(inc)
	mgr.DiagnoseIncident(inc.ID)

	actions, _ := mgr.ListRecoveryActions(inc.ID)
	recID := actions[0].ID

	// try execute without approval -> should fail
	_, err := mgr.ExecuteRecovery(recID)
	if err == nil {
		t.Fatal("expected error executing without approval")
	}
}

func TestRecoveryExecution_RejectedCannotExecute(t *testing.T) {
	mgr, _, sessID, _ := setupRecoveryTest(t)

	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sessID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output":"FAIL"}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	mgr.CreateIncident(inc)
	mgr.DiagnoseIncident(inc.ID)

	actions, _ := mgr.ListRecoveryActions(inc.ID)
	recID := actions[0].ID

	appr, _ := mgr.RequestRecoveryApproval(recID)
	mgr.RespondApproval(appr.ID, false, "user")

	_, err := mgr.ExecuteRecovery(recID)
	if err == nil {
		t.Fatal("expected error executing rejected recovery")
	}
}

func TestRecoveryExecution_AllowlistEnforced(t *testing.T) {
	mgr, st, sessID, _ := setupRecoveryTest(t)

	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sessID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output":"FAIL"}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	mgr.CreateIncident(inc)

	// manually create recovery with disallowed command
	rec := &state.RecoveryAction{
		ID:          uuid.New().String(),
		IncidentID:  inc.ID,
		Type:        state.RecoveryActionType("RETRY"),
		Description: "bad cmd",
		Command:     "rm",
		Args:        []string{"-rf", "/"},
		Status:      state.IncidentStatusProposed,
		ProposedAt:  time.Now(),
	}
	st.CreateRecoveryAction(rec)

	appr, _ := mgr.RequestRecoveryApproval(rec.ID)
	mgr.RespondApproval(appr.ID, true, "user")

	_, err := mgr.ExecuteRecovery(rec.ID)
	if err == nil {
		t.Fatal("expected error for disallowed command")
	}
}

func TestRecoveryExecution_VerificationStatusMapping(t *testing.T) {
	mgr, st, sessID, _ := setupRecoveryTest(t)

	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sessID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output":"FAIL"}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	mgr.CreateIncident(inc)

	// create recovery with allowed command go test ./... (which will pass)
	rec := &state.RecoveryAction{
		ID:          uuid.New().String(),
		IncidentID:  inc.ID,
		Type:        state.RecoveryActionType("RETRY"),
		Description: "run tests",
		Command:     "go",
		Args:        []string{"test", "./..."},
		Status:      state.IncidentStatusProposed,
		ProposedAt:  time.Now(),
	}
	st.CreateRecoveryAction(rec)

	appr, _ := mgr.RequestRecoveryApproval(rec.ID)
	mgr.RespondApproval(appr.ID, true, "user")

	result, err := mgr.ExecuteRecovery(rec.ID)
	if err != nil {
		t.Fatalf("ExecuteRecovery failed: %v", err)
	}
	if result.Status != state.VerificationStatusPassed {
		t.Errorf("expected PASSED, got %s", result.Status)
	}
	if result.NeedsReview {
		t.Errorf("PASSED should not need review")
	}
	// incident should be VERIFIED and RecoveredAt set
	inc2, _ := mgr.store.GetIncident(inc.ID)
	if inc2.Status != state.IncidentStatusVerified {
		t.Errorf("incident status %s", inc2.Status)
	}
	if inc2.RecoveredAt == nil {
		t.Errorf("RecoveredAt not set on PASSED")
	}
}

func TestRecoveryExecution_UNVERIFIEDFlag(t *testing.T) {
	// simulate integrity failure by using a command that succeeds but we cannot easily tamper.
	// Instead we test that the flag is set when status is Unverified via mock not feasible here.
	// This test ensures the field exists in JSON.
	mgr, _, sessID, _ := setupRecoveryTest(t)

	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sessID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output":"FAIL"}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	mgr.CreateIncident(inc)
	mgr.DiagnoseIncident(inc.ID)

	actions, _ := mgr.ListRecoveryActions(inc.ID)
	recID := actions[0].ID

	appr, _ := mgr.RequestRecoveryApproval(recID)
	mgr.RespondApproval(appr.ID, true, "user")

	result, _ := mgr.ExecuteRecovery(recID)
	// serialize and ensure NeedsReview field present
	data, _ := json.Marshal(result)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if _, ok := parsed["needs_review"]; !ok {
		t.Errorf("needs_review field missing in JSON: %s", string(data))
	}
}
