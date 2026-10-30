package session

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/argus-command-center/argus/services/agent-manager/internal/subprocess"
	"github.com/google/uuid"
)

func setupManager(t *testing.T) (*Manager, *store.Store, string) {
	dbPath := "/tmp/test_mgr_" + time.Now().Format("20060102150405.000000000") + ".db"
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

	projDir := "/tmp/test-mgr-" + time.Now().Format("20060102150405.000000000")
	t.Cleanup(func() { os.RemoveAll(projDir) })
	os.MkdirAll(projDir, 0755)

	return mgr, st, projDir
}

func TestManagerApprovalWorkflow(t *testing.T) {
	mgr, _, projDir := setupManager(t)

	session, task, err := mgr.CreateSession(context.Background(), projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	approval := &state.Approval{
		ID:          "test-approval-1",
		SessionID:   session.ID,
		TaskID:      &task.ID,
		Type:        state.ApprovalTypePermission,
		Status:      state.ApprovalStatusPending,
		Title:       "Test Permission",
		Description: "Test description",
		Payload:     json.RawMessage(`{"tool": "bash"}`),
		Scope:       task.Scope,
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	err = mgr.RequestApproval(approval)
	if err != nil {
		t.Fatalf("RequestApproval failed: %v", err)
	}

	// Verify task is parked
	parkedTask, err := mgr.store.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if parkedTask.Status != state.TaskStatusParked {
		t.Errorf("Task should be parked, got %s", parkedTask.Status)
	}
	if parkedTask.ApprovalID == nil || *parkedTask.ApprovalID != approval.ID {
		t.Errorf("Task approval ID not set")
	}

	// Test duplicate approval rejection
	approval2 := &state.Approval{
		ID:          "test-approval-2",
		SessionID:   session.ID,
		TaskID:      &task.ID,
		Type:        state.ApprovalTypePermission,
		Status:      state.ApprovalStatusPending,
		Title:       "Test Permission 2",
		Description: "Test description 2",
		Payload:     json.RawMessage(`{"tool": "write"}`),
		Scope:       task.Scope,
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	err = mgr.RequestApproval(approval2)
	if err == nil {
		t.Errorf("Expected duplicate approval to be rejected")
	}

	// Approve the original approval
	err = mgr.RespondApproval(approval.ID, true, "test-user")
	if err != nil {
		t.Fatalf("RespondApproval failed: %v", err)
	}

	// Verify task is running
	approvedTask, err := mgr.store.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if approvedTask.Status != state.TaskStatusRunning {
		t.Errorf("Task should be running after approval, got %s", approvedTask.Status)
	}
	if approvedTask.ApprovalID != nil {
		t.Errorf("Task approval ID should be cleared")
	}

	// Verify approval status
	approvedApproval, err := mgr.store.GetApproval(approval.ID)
	if err != nil {
		t.Fatalf("GetApproval failed: %v", err)
	}
	if approvedApproval.Status != state.ApprovalStatusApproved {
		t.Errorf("Approval should be approved, got %s", approvedApproval.Status)
	}
	if approvedApproval.Decision == nil || !*approvedApproval.Decision {
		t.Errorf("Approval decision should be true")
	}
}

func TestManagerApprovalRejection(t *testing.T) {
	mgr, _, projDir := setupManager(t)

	session, task, err := mgr.CreateSession(context.Background(), projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	approval := &state.Approval{
		ID:          "test-approval-rej",
		SessionID:   session.ID,
		TaskID:      &task.ID,
		Type:        state.ApprovalTypePermission,
		Status:      state.ApprovalStatusPending,
		Title:       "Test Permission",
		Description: "Test",
		Payload:     json.RawMessage(`{}`),
		Scope:       task.Scope,
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	err = mgr.RequestApproval(approval)
	if err != nil {
		t.Fatalf("RequestApproval failed: %v", err)
	}

	// Reject the approval
	err = mgr.RespondApproval(approval.ID, false, "test-user")
	if err != nil {
		t.Fatalf("RespondApproval failed: %v", err)
	}

	// Verify task is failed
	rejectedTask, err := mgr.store.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if rejectedTask.Status != state.TaskStatusFailed {
		t.Errorf("Task should be failed after rejection, got %s", rejectedTask.Status)
	}
}

func TestManagerApprovalExpiry(t *testing.T) {
	mgr, _, projDir := setupManager(t)

	session, task, err := mgr.CreateSession(context.Background(), projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create expired approval
	approval := &state.Approval{
		ID:          "test-approval-exp",
		SessionID:   session.ID,
		TaskID:      &task.ID,
		Type:        state.ApprovalTypePermission,
		Status:      state.ApprovalStatusPending,
		Title:       "Test Permission",
		Description: "Test",
		Payload:     json.RawMessage(`{}`),
		Scope:       task.Scope,
		RequestedAt: time.Now().Add(-10 * time.Minute),
		ExpiresAt:   time.Now().Add(-5 * time.Minute), // Already expired
	}

	err = mgr.RequestApproval(approval)
	if err != nil {
		t.Fatalf("RequestApproval failed: %v", err)
	}

	// Try to approve expired approval
	err = mgr.RespondApproval(approval.ID, true, "test-user")
	if err == nil {
		t.Errorf("Expected expired approval to be rejected")
	}

	// Verify task is still parked (not running)
	parkedTask, err := mgr.store.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if parkedTask.Status != state.TaskStatusParked {
		t.Errorf("Task should remain parked for expired approval, got %s", parkedTask.Status)
	}
}

func TestManagerIncidentDetection(t *testing.T) {
	mgr, st, projDir := setupManager(t)

	session, _, err := mgr.CreateSession(context.Background(), projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add events that should trigger incidents
	events := []*state.Event{
		{
			SessionID: session.ID,
			Type:      "tool_use",
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"tool": "bash",
				"state": map[string]interface{}{
					"status": "error",
					"metadata": map[string]interface{}{
						"output": "main.go:10: undefined: foo\n",
						"exit":   1.0,
					},
				},
			},
		},
		{
			SessionID: session.ID,
			Type:      "tool_use",
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"tool": "bash",
				"state": map[string]interface{}{
					"status": "completed",
					"metadata": map[string]interface{}{
						"output": "FAIL  TestExample\n  main_test.go:15: Expected 2, got 3\n",
						"exit":   1.0,
					},
				},
			},
		},
		{
			SessionID: session.ID,
			Type:      "tool_use",
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"tool": "bash",
				"state": map[string]interface{}{
					"status": "error",
					"metadata": map[string]interface{}{
						"output": "go: finding module for package github.com/missing/pkg\n",
						"exit":   1.0,
					},
				},
			},
		},
	}

	for _, e := range events {
		st.AppendEvent(e)
	}

	incidents, err := mgr.DetectIncidentsFromEvents(session.ID)
	if err != nil {
		t.Fatalf("DetectIncidentsFromEvents failed: %v", err)
	}

	t.Logf("Detected %d incidents", len(incidents))
	for _, inc := range incidents {
		t.Logf("  Type: %s, Severity: %s", inc.Type, inc.Severity)
	}

	// Should detect at least 3 incidents (compile error, test failure, missing dep)
	if len(incidents) < 3 {
		t.Errorf("Expected at least 3 incidents, got %d", len(incidents))
	}

	// Check incident types
	incidentTypes := make(map[string]bool)
	for _, inc := range incidents {
		incidentTypes[inc.Type] = true
	}
	if !incidentTypes["compile_error"] {
		t.Errorf("Expected compile_error incident")
	}
	if !incidentTypes["test_failure"] {
		t.Errorf("Expected test_failure incident")
	}
	if !incidentTypes["missing_dependency"] {
		t.Errorf("Expected missing_dependency incident")
	}
}

func TestManagerIndependentContinuation(t *testing.T) {
	mgr, st, _ := setupManager(t)

	// Create two tasks with different scopes
	task1 := &state.Task{
		ID:           "task-1",
		SessionID:    "session-1",
		Prompt:       "task 1",
		Status:       state.TaskStatusParked,
		Scope:        "project:foo",
		Dependencies: []string{},
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	task2 := &state.Task{
		ID:           "task-2",
		SessionID:    "session-1",
		Prompt:       "task 2",
		Status:       state.TaskStatusPending,
		Scope:        "project:bar",
		Dependencies: []string{},
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	st.CreateTask(task1)
	st.CreateTask(task2)

	// Different scopes should be independent
	if !mgr.CanTaskRunIndependently(task2, task1) {
		t.Errorf("Tasks with different scopes should be independent")
	}

	// Same scope - not independent
	task3 := &state.Task{
		ID:           "task-3",
		SessionID:    "session-1",
		Prompt:       "task 3",
		Status:       state.TaskStatusPending,
		Scope:        "project:foo",
		Dependencies: []string{},
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	st.CreateTask(task3)

	if mgr.CanTaskRunIndependently(task3, task1) {
		t.Errorf("Tasks with same scope should not be independent")
	}

	// Shared dependency - not independent (but same scope already covers this)
	task4 := &state.Task{
		ID:           "task-4",
		SessionID:    "session-1",
		Prompt:       "task 4",
		Status:       state.TaskStatusPending,
		Scope:        "project:baz",
		Dependencies: []string{"dep-1"},
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	task5 := &state.Task{
		ID:           "task-5",
		SessionID:    "session-1",
		Prompt:       "task 5",
		Status:       state.TaskStatusParked,
		Scope:        "project:baz",
		Dependencies: []string{"dep-1"},
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	st.CreateTask(task4)
	st.CreateTask(task5)

	if mgr.CanTaskRunIndependently(task4, task5) {
		t.Errorf("Tasks with same scope should not be independent")
	}
}

// TestDuplicatePendingRecoveryApproval ensures only one pending approval per recovery action.
func TestDuplicatePendingRecoveryApproval(t *testing.T) {
	mgr, st, projDir := setupManager(t)

	ctx := context.Background()
	sess, _, err := mgr.CreateSession(ctx, projDir, "", "test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output":"FAIL"}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	if err := mgr.CreateIncident(inc); err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}
	if err := mgr.DiagnoseIncident(inc.ID); err != nil {
		t.Fatalf("DiagnoseIncident: %v", err)
	}

	actions, err := mgr.ListRecoveryActions(inc.ID)
	if err != nil || len(actions) == 0 {
		t.Fatalf("no recovery actions")
	}
	recID := actions[0].ID

	// First request should succeed
	appr1, err := mgr.RequestRecoveryApproval(recID)
	if err != nil {
		t.Fatalf("first RequestRecoveryApproval failed: %v", err)
	}
	if appr1.Status != state.ApprovalStatusPending {
		t.Errorf("first approval not pending")
	}

	// Second request for same recovery action should be rejected (duplicate incident)
	_, err = mgr.RequestRecoveryApproval(recID)
	if err == nil {
		t.Fatal("second RequestRecoveryApproval should fail")
	}
	if !strings.Contains(err.Error(), "duplicate approval request for incident") {
		t.Errorf("unexpected error: %v", err)
	}

	// Ensure only one pending approval exists
	pending, _ := st.ListPendingApprovals()
	count := 0
	for _, a := range pending {
		if a.IncidentID != nil && *a.IncidentID == inc.ID && a.Type == state.ApprovalTypeRecovery {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one pending recovery approval, got %d", count)
	}
}

// TestDuplicateDecisionRejectThenApprove verifies a rejected approval cannot later be approved.
func TestDuplicateDecisionRejectThenApprove(t *testing.T) {
	mgr, _, projDir := setupManager(t)

	ctx := context.Background()
	sess, _, err := mgr.CreateSession(ctx, projDir, "", "test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
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

	// Reject first
	if err = mgr.RespondApproval(appr.ID, false, "user"); err != nil {
		t.Fatalf("reject failed: %v", err)
	}

	// Attempt to approve same approval -> should fail with conflict
	err = mgr.RespondApproval(appr.ID, true, "user")
	if err == nil {
		t.Fatal("approve after reject should fail")
	}
	if !strings.Contains(err.Error(), "not pending") && !strings.Contains(err.Error(), "expired") {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify recovery action remains rejected
	ra, _ := mgr.store.GetRecoveryAction(recID)
	if ra.Status != state.IncidentStatusRejected {
		t.Errorf("recovery action not rejected: %s", ra.Status)
	}
}

// TestDuplicateDecisionApproveThenReject verifies an approved approval cannot later be rejected.
func TestDuplicateDecisionApproveThenReject(t *testing.T) {
	mgr, _, projDir := setupManager(t)

	ctx := context.Background()
	sess, _, err := mgr.CreateSession(ctx, projDir, "", "test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	inc := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
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

	// Approve first
	if err = mgr.RespondApproval(appr.ID, true, "user"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	// Attempt to reject same approval -> should fail
	err = mgr.RespondApproval(appr.ID, false, "user")
	if err == nil {
		t.Fatal("reject after approve should fail")
	}
	if !strings.Contains(err.Error(), "not pending") && !strings.Contains(err.Error(), "expired") {
		t.Errorf("unexpected error: %v", err)
	}

	// Recovery action should stay approved
	ra, _ := mgr.store.GetRecoveryAction(recID)
	if ra.Status != state.IncidentStatusApproved {
		t.Errorf("recovery action not approved: %s", ra.Status)
	}
}
