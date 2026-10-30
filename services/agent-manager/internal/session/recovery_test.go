package session

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/argus-command-center/argus/services/agent-manager/internal/subprocess"
	"github.com/google/uuid"
)

func TestRecoveryWorkflow(t *testing.T) {
	dbPath := "/tmp/test_recovery_" + time.Now().Format("20060102150405.000000000") + ".db"
	defer os.Remove(dbPath)

	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	procConfig := subprocess.OpenCodeConfig{BinaryPath: "opencode"}
	procMgr := subprocess.NewProcessManager(st, procConfig)
	metaConfig := meta.DefaultConfig() // Uses fallback
	mgr := NewManager(st, procMgr, metaConfig)

	projDir := "/tmp/test-recovery-" + time.Now().Format("20060102150405.000000000")
	defer os.RemoveAll(projDir)
	os.MkdirAll(projDir, 0755)

	// Create a session
	session, task, err := mgr.CreateSession(context.Background(), projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create an incident (simulating a test failure)
	incident := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        session.ID,
		TaskID:           &task.ID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output": "FAIL TestExample\n  main_test.go:15: Expected 2, got 3", "exit": 1}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}

	if err := mgr.CreateIncident(incident); err != nil {
		t.Fatalf("CreateIncident failed: %v", err)
	}

	// Step 1: Diagnose incident
	err = mgr.DiagnoseIncident(incident.ID)
	if err != nil {
		t.Fatalf("DiagnoseIncident failed: %v", err)
	}

	// Verify diagnosis was stored
	diagIncident, err := mgr.store.GetIncident(incident.ID)
	if err != nil {
		t.Fatalf("GetIncident failed: %v", err)
	}
	if diagIncident.MetaDiagnosis == "" {
		t.Errorf("Expected diagnosis to be stored")
	}
	if diagIncident.MetaRecovery == nil {
		t.Errorf("Expected recovery plan to be stored")
	}
	if diagIncident.Status != state.IncidentStatusProposed {
		t.Errorf("Expected incident status PROPOSED, got %s", diagIncident.Status)
	}

	// Verify recovery action was created
	actions, err := mgr.store.ListRecoveryActionsByIncident(incident.ID)
	if err != nil {
		t.Fatalf("ListRecoveryActionsByIncident failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("Expected 1 recovery action, got %d", len(actions))
	}
	recoveryAction := actions[0]
	if recoveryAction.Status != state.IncidentStatusProposed {
		t.Errorf("Expected recovery action status PROPOSED, got %s", recoveryAction.Status)
	}

	// Step 2: Request recovery approval
	approval, err := mgr.RequestRecoveryApproval(recoveryAction.ID)
	if err != nil {
		t.Fatalf("RequestRecoveryApproval failed: %v", err)
	}
	if approval.Type != state.ApprovalTypeRecovery {
		t.Errorf("Expected approval type RECOVERY, got %s", approval.Type)
	}
	if approval.Status != state.ApprovalStatusPending {
		t.Errorf("Expected approval status PENDING, got %s", approval.Status)
	}

	// Step 3: Try to execute recovery before approval (should fail)
	_, err = mgr.ExecuteRecovery(recoveryAction.ID)
	if err == nil {
		t.Errorf("Expected ExecuteRecovery to fail before approval")
	}

	// Step 4: Reject the approval
	err = mgr.RespondApproval(approval.ID, false, "test-user")
	if err != nil {
		t.Fatalf("RespondApproval (reject) failed: %v", err)
	}

	// Verify approval was rejected
	rejectedApproval, err := mgr.store.GetApproval(approval.ID)
	if err != nil {
		t.Fatalf("GetApproval failed: %v", err)
	}
	if rejectedApproval.Status != state.ApprovalStatusRejected {
		t.Errorf("Expected approval status REJECTED, got %s", rejectedApproval.Status)
	}

	// Verify incident status updated to REJECTED
	rejectedIncident, err := mgr.store.GetIncident(incident.ID)
	if err != nil {
		t.Fatalf("GetIncident failed: %v", err)
	}
	if rejectedIncident.Status != state.IncidentStatusRejected {
		t.Errorf("Expected incident status REJECTED after approval rejection, got %s", rejectedIncident.Status)
	}

	// Step 5: Create a new incident for approval test
	incident2 := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        session.ID,
		TaskID:           &task.ID,
		Type:             "compile_error",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output": "main.go:10: undefined: foo", "exit": 1}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}

	if err := mgr.CreateIncident(incident2); err != nil {
		t.Fatalf("CreateIncident2 failed: %v", err)
	}

	// Diagnose
	err = mgr.DiagnoseIncident(incident2.ID)
	if err != nil {
		t.Fatalf("DiagnoseIncident2 failed: %v", err)
	}

	// Get recovery action
	actions2, err := mgr.store.ListRecoveryActionsByIncident(incident2.ID)
	if err != nil {
		t.Fatalf("ListRecoveryActionsByIncident2 failed: %v", err)
	}
	recoveryAction2 := actions2[0]

	// Request approval
	approval2, err := mgr.RequestRecoveryApproval(recoveryAction2.ID)
	if err != nil {
		t.Fatalf("RequestRecoveryApproval2 failed: %v", err)
	}

	// Step 6: Approve the recovery
	err = mgr.RespondApproval(approval2.ID, true, "test-user")
	if err != nil {
		t.Fatalf("RespondApproval (approve) failed: %v", err)
	}

	// Verify approval approved
	approvedApproval, err := mgr.store.GetApproval(approval2.ID)
	if err != nil {
		t.Fatalf("GetApproval2 failed: %v", err)
	}
	if approvedApproval.Status != state.ApprovalStatusApproved {
		t.Errorf("Expected approval status APPROVED, got %s", approvedApproval.Status)
	}

	// Step 7: Execute recovery
	result, err := mgr.ExecuteRecovery(recoveryAction2.ID)
	if err != nil {
		t.Fatalf("ExecuteRecovery failed: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Expected verification result")
	}
	t.Logf("Verification result: %s, exit code: %d, integrity: %v", result.Status, result.ExitCode, result.IntegrityOK)

	// Verify incident status
	finalIncident, err := mgr.store.GetIncident(incident2.ID)
	if err != nil {
		t.Fatalf("GetIncident2 final failed: %v", err)
	}

	// Should be either VERIFIED or FAILED depending on command success
	if finalIncident.Status != state.IncidentStatusVerified && finalIncident.Status != state.IncidentStatusFailed {
		t.Errorf("Expected incident status VERIFIED or FAILED, got %s", finalIncident.Status)
	}

	t.Logf("Final incident status: %s", finalIncident.Status)
	t.Logf("Recovery attempts: %d", finalIncident.RecoveryAttempts)
}

// TestRecoveryApprovalTargetsExactAction ensures an approval cannot authorize
// another recovery action belonging to the same incident.
func TestRecoveryApprovalTargetsExactAction(t *testing.T) {
	mgr, st, projDir := setupManager(t)

	session, _, err := mgr.CreateSession(context.Background(), projDir, "", "approval binding test")
	if err != nil {
		t.Fatal(err)
	}

	incident := &state.Incident{
		ID:         uuid.New().String(),
		SessionID:  session.ID,
		Type:       "test_failure",
		Severity:   state.IncidentSeverityHigh,
		Status:     state.IncidentStatusDetected,
		Evidence:   json.RawMessage(`{"test":"approval binding"}`),
		DetectedAt: time.Now(),
	}
	if err := mgr.CreateIncident(incident); err != nil {
		t.Fatal(err)
	}

	actionA := &state.RecoveryAction{
		ID:          uuid.New().String(),
		IncidentID:  incident.ID,
		Type:        state.RecoveryActionType("verification"),
		Description: "Recovery A",
		Command:     "echo",
		Args:        []string{"A"},
		Status:      state.IncidentStatusProposed,
	}
	actionB := &state.RecoveryAction{
		ID:          uuid.New().String(),
		IncidentID:  incident.ID,
		Type:        state.RecoveryActionType("verification"),
		Description: "Recovery B",
		Command:     "echo",
		Args:        []string{"B"},
		Status:      state.IncidentStatusProposed,
	}

	if err := st.CreateRecoveryAction(actionA); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateRecoveryAction(actionB); err != nil {
		t.Fatal(err)
	}

	approval, err := mgr.RequestRecoveryApproval(actionA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.RespondApproval(approval.ID, true, "test-user"); err != nil {
		t.Fatal(err)
	}

	gotA, err := st.GetRecoveryAction(actionA.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := st.GetRecoveryAction(actionB.ID)
	if err != nil {
		t.Fatal(err)
	}

	if gotA.Status != state.IncidentStatusApproved {
		t.Fatalf("approved action A status = %s, want APPROVED", gotA.Status)
	}
	if gotB.Status != state.IncidentStatusProposed {
		t.Fatalf("unapproved action B status = %s, want PROPOSED", gotB.Status)
	}
}

// TestRecoveryRejectsDisallowedCommandBeforeExecution ensures a command outside
// the allowlist is rejected before it can create a side-effect.
func TestRecoveryRejectsDisallowedCommandBeforeExecution(t *testing.T) {
	mgr, st, projDir := setupManager(t)

	session, _, err := mgr.CreateSession(context.Background(), projDir, "", "unsafe command test")
	if err != nil {
		t.Fatal(err)
	}

	marker := projDir + "/command-was-executed"
	incident := &state.Incident{
		ID:         uuid.New().String(),
		SessionID:  session.ID,
		Type:       "test_failure",
		Severity:   state.IncidentSeverityHigh,
		Status:     state.IncidentStatusApproved,
		Evidence:   json.RawMessage(`{"test":"command allowlist"}`),
		DetectedAt: time.Now(),
	}
	if err := mgr.CreateIncident(incident); err != nil {
		t.Fatal(err)
	}

	action := &state.RecoveryAction{
		ID:          uuid.New().String(),
		IncidentID:  incident.ID,
		Type:        state.RecoveryActionType("verification"),
		Description: "Must never execute",
		Command:     "touch",
		Args:        []string{marker},
		Status:      state.IncidentStatusApproved,
	}
	if err := st.CreateRecoveryAction(action); err != nil {
		t.Fatal(err)
	}

	_, err = mgr.ExecuteRecovery(action.ID)
	if err == nil {
		t.Fatal("ExecuteRecovery succeeded for a disallowed command; want rejection")
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("side-effect marker exists or cannot be checked; command may have executed (stat error: %v)", statErr)
	}
}
