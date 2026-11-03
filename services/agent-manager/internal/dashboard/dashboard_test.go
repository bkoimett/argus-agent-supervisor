package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/adapter"
	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
	"github.com/argus-command-center/argus/services/agent-manager/internal/session"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/argus-command-center/argus/services/agent-manager/internal/subprocess"
	"github.com/google/uuid"
)

func setupDashboard(t *testing.T) (*Server, *store.Store, string) {
	dbPath := "/tmp/test_dashboard_" + time.Now().Format("20060102150405.000000000") + ".db"
	t.Cleanup(func() { os.Remove(dbPath) })

	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	procConfig := subprocess.OpenCodeConfig{BinaryPath: "opencode"}
	procMgr := subprocess.NewProcessManager(st, nil, nil, procConfig)
	metaConfig := meta.DefaultConfig()
	mgr := session.NewManager(st, procMgr, metaConfig, nil, nil)

	projDir := "/tmp/test-dashboard-" + time.Now().Format("20060102150405.000000000")
	t.Cleanup(func() { os.RemoveAll(projDir) })
	os.MkdirAll(projDir, 0755)

	srv := NewServer(mgr, st, "0", nil, nil, time.Now())
	return srv, st, projDir
}

func createTestSessionWithIncidentAndRecovery(t *testing.T, mgr *session.Manager, projDir string) (string, string, string) {
	ctx := context.Background()
	sess, task, err := mgr.CreateSession(ctx, projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	incident := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
		TaskID:           &task.ID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output": "FAIL TestExample", "exit": 1}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	if err := mgr.CreateIncident(incident); err != nil {
		t.Fatalf("CreateIncident failed: %v", err)
	}

	err = mgr.DiagnoseIncident(incident.ID)
	if err != nil {
		t.Fatalf("DiagnoseIncident failed: %v", err)
	}

	actions, err := mgr.ListRecoveryActions(incident.ID)
	if err != nil {
		t.Fatalf("ListRecoveryActions failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("Expected 1 recovery action, got %d", len(actions))
	}

	return sess.ID, incident.ID, actions[0].ID
}

func TestHandleRecoveryList(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

	_, incID, recID := createTestSessionWithIncidentAndRecovery(t, mgr, projDir)

	// Test GET /api/recovery?incident_id=...
	req := httptest.NewRequest(http.MethodGet, "/api/recovery?incident_id="+incID, nil)
	rr := httptest.NewRecorder()
	srv.handleRecovery(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var actions []*state.RecoveryAction
	if err := json.Unmarshal(rr.Body.Bytes(), &actions); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if len(actions) != 1 {
		t.Errorf("Expected 1 recovery action, got %d", len(actions))
	}
	if actions[0].ID != recID {
		t.Errorf("Expected recovery ID %s, got %s", recID, actions[0].ID)
	}

	// Test GET /api/recovery without filter (should return all)
	req = httptest.NewRequest(http.MethodGet, "/api/recovery", nil)
	rr = httptest.NewRecorder()
	srv.handleRecovery(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &actions); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if len(actions) < 1 {
		t.Errorf("Expected at least 1 recovery action, got %d", len(actions))
	}
}

func TestHandleRecoveryGetByID(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

	_, incID, recID := createTestSessionWithIncidentAndRecovery(t, mgr, projDir)

	// Test GET /api/recovery/{id}
	req := httptest.NewRequest(http.MethodGet, "/api/recovery/"+recID, nil)
	rr := httptest.NewRecorder()
	srv.handleRecoveryByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var action state.RecoveryAction
	if err := json.Unmarshal(rr.Body.Bytes(), &action); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if action.ID != recID {
		t.Errorf("Expected recovery ID %s, got %s", recID, action.ID)
	}
	if action.IncidentID != incID {
		t.Errorf("Expected incident ID %s, got %s", incID, action.IncidentID)
	}

	// Test GET /api/recovery/{id} for non-existent ID
	nonExistentID := uuid.New().String()
	req = httptest.NewRequest(http.MethodGet, "/api/recovery/"+nonExistentID, nil)
	rr = httptest.NewRecorder()
	srv.handleRecoveryByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent ID, got %d", rr.Code)
	}

	// Test GET /api/recovery/ (empty ID)
	req = httptest.NewRequest(http.MethodGet, "/api/recovery/", nil)
	rr = httptest.NewRecorder()
	srv.handleRecoveryByID(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty ID, got %d", rr.Code)
	}
}

func TestHandleRecoveryPost(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

	ctx := context.Background()
	sess, task, err := mgr.CreateSession(ctx, projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	incident := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
		TaskID:           &task.ID,
		Type:             "test_failure",
		Severity:         state.IncidentSeverityHigh,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(`{"output": "FAIL TestExample", "exit": 1}`),
		DetectedAt:       time.Now(),
		RecoveryAttempts: 0,
	}
	if err := mgr.CreateIncident(incident); err != nil {
		t.Fatalf("CreateIncident failed: %v", err)
	}

	// Test POST /api/recovery
	reqBody := map[string]interface{}{
		"incident_id": incident.ID,
		"type":        "RETRY",
		"description": "Retry tests",
		"command":     "go",
		"args":        []string{"test", "./..."},
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/recovery", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleRecovery(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var action state.RecoveryAction
	if err := json.Unmarshal(rr.Body.Bytes(), &action); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if action.IncidentID != incident.ID {
		t.Errorf("Expected incident ID %s, got %s", incident.ID, action.IncidentID)
	}
	if action.Type != state.RecoveryActionType("RETRY") {
		t.Errorf("Expected type RETRY, got %s", action.Type)
	}
	if action.Command != "go" {
		t.Errorf("Expected command 'go', got %s", action.Command)
	}
}

func TestHandleRecoveryUnsupportedMethods(t *testing.T) {
	srv, _, _ := setupDashboard(t)

	// Test PUT /api/recovery
	req := httptest.NewRequest(http.MethodPut, "/api/recovery", nil)
	rr := httptest.NewRecorder()
	srv.handleRecovery(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for PUT /api/recovery, got %d", rr.Code)
	}

	// Test DELETE /api/recovery/{id}
	req = httptest.NewRequest(http.MethodDelete, "/api/recovery/some-id", nil)
	rr = httptest.NewRecorder()
	srv.handleRecoveryByID(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for DELETE /api/recovery/{id}, got %d", rr.Code)
	}
}

func TestHandleDashboardIncludesRecovery(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

	_, _, recID := createTestSessionWithIncidentAndRecovery(t, mgr, projDir)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rr := httptest.NewRecorder()
	srv.handleDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var dashboard []struct {
		Session         *state.Session          `json:"session"`
		Tasks           []*state.Task           `json:"tasks"`
		Incidents       []*state.Incident       `json:"incidents"`
		Approvals       []*state.Approval       `json:"approvals"`
		RecentEvents    []*state.Event          `json:"recent_events"`
		RecoveryActions []*state.RecoveryAction `json:"recovery_actions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &dashboard); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if len(dashboard) != 1 {
		t.Errorf("Expected 1 session in dashboard, got %d", len(dashboard))
	}
	if len(dashboard[0].RecoveryActions) != 1 {
		t.Errorf("Expected 1 recovery action in dashboard, got %d", len(dashboard[0].RecoveryActions))
	}
	if dashboard[0].RecoveryActions[0].ID != recID {
		t.Errorf("Expected recovery ID %s in dashboard, got %s", recID, dashboard[0].RecoveryActions[0].ID)
	}
}

// TestHandleIncidentByID tests GET /api/incidents/{id}
func TestHandleIncidentByID(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

	ctx := context.Background()
	sess, _, err := mgr.CreateSession(ctx, projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	incident := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
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

	// GET existing incident
	req := httptest.NewRequest(http.MethodGet, "/api/incidents/"+incident.ID, nil)
	rr := httptest.NewRecorder()
	srv.handleIncidentByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got state.Incident
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != incident.ID {
		t.Errorf("incident ID mismatch")
	}

	// GET non-existent incident
	req = httptest.NewRequest(http.MethodGet, "/api/incidents/"+uuid.New().String(), nil)
	rr = httptest.NewRecorder()
	srv.handleIncidentByID(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for missing incident, got %d", rr.Code)
	}

	// Unsupported method
	req = httptest.NewRequest(http.MethodPost, "/api/incidents/"+incident.ID, nil)
	rr = httptest.NewRecorder()
	srv.handleIncidentByID(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 for POST, got %d", rr.Code)
	}
}

// TestHandleIncidentByIDEmptyID tests empty ID returns 400
func TestHandleIncidentByIDEmptyID(t *testing.T) {
	srv, _, _ := setupDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/", nil)
	rr := httptest.NewRecorder()
	srv.handleIncidentByID(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty ID, got %d", rr.Code)
	}
}

// TestHandleRecoveryApprovalValidation tests validation of recovery approval endpoint
func TestHandleRecoveryApprovalValidation(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

	ctx := context.Background()
	sess, _, err := mgr.CreateSession(ctx, projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	incident := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
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

	// diagnose to create recovery action
	if err := mgr.DiagnoseIncident(incident.ID); err != nil {
		t.Fatalf("DiagnoseIncident failed: %v", err)
	}

	actions, err := mgr.ListRecoveryActions(incident.ID)
	if err != nil || len(actions) == 0 {
		t.Fatalf("no recovery actions")
	}
	recID := actions[0].ID

	// Test empty recovery_id
	body := `{"recovery_id":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/recovery/approval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleRecoveryApproval(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty recovery_id expected 400, got %d", rr.Code)
	}

	// Test missing recovery_id
	body = `{}`
	req = httptest.NewRequest(http.MethodPost, "/api/recovery/approval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleRecoveryApproval(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing recovery_id expected 400, got %d", rr.Code)
	}

	// Test invalid JSON
	body = `not json`
	req = httptest.NewRequest(http.MethodPost, "/api/recovery/approval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleRecoveryApproval(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON expected 400, got %d", rr.Code)
	}

	// Valid request should succeed (201)
	body = `{"recovery_id":"` + recID + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/recovery/approval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleRecoveryApproval(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("valid request expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleApprovalDecisionValidation tests approval decision endpoint validation
func TestHandleApprovalDecisionValidation(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

	ctx := context.Background()
	sess, _, err := mgr.CreateSession(ctx, projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	incident := &state.Incident{
		ID:               uuid.New().String(),
		SessionID:        sess.ID,
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
	if err := mgr.DiagnoseIncident(incident.ID); err != nil {
		t.Fatalf("DiagnoseIncident failed: %v", err)
	}
	actions, _ := mgr.ListRecoveryActions(incident.ID)
	recID := actions[0].ID

	// request approval
	appr, err := mgr.RequestRecoveryApproval(recID)
	if err != nil {
		t.Fatalf("RequestRecoveryApproval failed: %v", err)
	}
	apprID := appr.ID

	// missing approved field
	body := `{"decided_by":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing approved expected 400, got %d", rr.Code)
	}

	// approved null
	body = `{"approved":null,"decided_by":"user"}`
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("null approved expected 400, got %d", rr.Code)
	}

	// missing decided_by
	body = `{"approved":true}`
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing decided_by expected 400, got %d", rr.Code)
	}

	// invalid JSON
	body = `not json`
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON expected 400, got %d", rr.Code)
	}

	// unknown approval ID
	body = `{"approved":true,"decided_by":"user"}`
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/"+uuid.New().String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("unknown approval expected 404, got %d", rr.Code)
	}

	// unsupported method
	req = httptest.NewRequest(http.MethodGet, "/api/approvals/"+apprID, nil)
	rr = httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET expected 405, got %d", rr.Code)
	}
}

// TestDuplicateDecisionRejectThenApproveHTTP tests reject then approve via HTTP
func TestDuplicateDecisionRejectThenApproveHTTP(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

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

	actions, _ := mgr.ListRecoveryActions(inc.ID)
	recID := actions[0].ID

	appr, _ := mgr.RequestRecoveryApproval(recID)
	apprID := appr.ID

	// Reject via HTTP
	body := `{"approved":false,"decided_by":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("reject expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Attempt to approve same approval -> should get 409
	body = `{"approved":true,"decided_by":"user"}`
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("approve after reject expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestDuplicateDecisionApproveThenRejectHTTP tests approve then reject via HTTP
func TestDuplicateDecisionApproveThenRejectHTTP(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	mgr := srv.mgr

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

	actions, _ := mgr.ListRecoveryActions(inc.ID)
	recID := actions[0].ID

	appr, _ := mgr.RequestRecoveryApproval(recID)
	apprID := appr.ID

	// Approve via HTTP
	body := `{"approved":true,"decided_by":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("approve expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Attempt to reject same approval -> should get 409
	body = `{"approved":false,"decided_by":"user"}`
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/"+apprID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.handleApprovalByID(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("reject after approve expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePairingCodeNoSupabase(t *testing.T) {
	srv := &Server{mgr: nil, store: nil, adapter: nil, sbClient: nil, port: "0", startTime: time.Now()}

	// POST without Supabase -> 503
	req := httptest.NewRequest(http.MethodPost, "/api/pairing-code", nil)
	rr := httptest.NewRecorder()
	srv.handlePairingCode(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when Supabase disabled, got %d", rr.Code)
	}

	// GET -> 405
	req = httptest.NewRequest(http.MethodGet, "/api/pairing-code", nil)
	rr = httptest.NewRecorder()
	srv.handlePairingCode(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", rr.Code)
	}
}

// TestHandleDeleteSessionWithManager verifies DELETE /api/sessions/{id}
// works when the manager is configured (even if adapter is nil).
func TestHandleDeleteSessionWithManager(t *testing.T) {
	srv, _, projDir := setupDashboard(t)

	// Create a session via the manager.
	ctx := context.Background()
	sess, _, err := srv.mgr.CreateSession(ctx, projDir, "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// srv.adapter is nil (setupDashboard passes nil), but manager is configured.
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/"+sess.ID, nil)
	rr := httptest.NewRecorder()
	srv.handleSessionByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with manager configured, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "detached" || resp["session_id"] != sess.ID {
		t.Errorf("unexpected response: %v", resp)
	}
}

// mockDashboardAdapter records Start/SendMessage calls for dashboard tests.
type mockDashboardAdapter struct {
	mu         sync.Mutex
	startCalls int
	sendCalls  int
	sessionID  string
	startErr   error
	sendErr    error
	sendResult adapter.MessageResult
	done       chan struct{} // closed when SendMessage completes
}

func (m *mockDashboardAdapter) Start(projectDir string) (string, error) {
	m.mu.Lock()
	m.startCalls++
	if m.startErr != nil {
		m.mu.Unlock()
		return "", m.startErr
	}
	m.sessionID = "ses_mock_" + projectDir
	m.mu.Unlock()
	return m.sessionID, nil
}

func (m *mockDashboardAdapter) SendMessage(sessionID, message string) (adapter.MessageResult, error) {
	m.mu.Lock()
	m.sendCalls++
	err := m.sendErr
	result := m.sendResult
	m.mu.Unlock()
	// Signal completion so tests can wait for the async goroutine.
	if m.done != nil {
		close(m.done)
	}
	return result, err
}

func (m *mockDashboardAdapter) waitForSend(t *testing.T) {
	if m.done == nil {
		return
	}
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SendMessage to complete")
	}
}

func (m *mockDashboardAdapter) Stop(string) error                      { return nil }
func (m *mockDashboardAdapter) Restart(string) error                   { return nil }
func (m *mockDashboardAdapter) Status(string) (adapter.Status, error)  { return adapter.StatusWorking, nil }
func (m *mockDashboardAdapter) Interrupt(string) error                 { return nil }
func (m *mockDashboardAdapter) Abort(string) error                     { return nil }
func (m *mockDashboardAdapter) RespondToPermission(string, string, bool) error { return nil }
func (m *mockDashboardAdapter) GetEvents(string) ([]adapter.Event, error) { return nil, nil }
func (m *mockDashboardAdapter) Close() error                           { return nil }

var _ adapter.AgentAdapter = (*mockDashboardAdapter)(nil)

func TestHandleSessionsCreateSession(t *testing.T) {
	srv, _, projDir := setupDashboard(t)
	// No adapter needed - we use the manager now

	body := `{"project_dir":"` + projDir + `","prompt":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleSessions(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Session recorded in SQLite.
	var bodyData map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &bodyData)
	sessionID := bodyData["session_id"].(string)

	sess, err := srv.store.GetSession(sessionID)
	if err != nil {
		t.Fatalf("expected session recorded in store: %v", err)
	}
	if sess.Status != state.SessionStatusRunning {
		t.Errorf("expected session status RUNNING, got %s", sess.Status)
	}
}

func TestHandleSessionsCreateSessionPromptFailureBestEffort(t *testing.T) {
	srv, _, _ := setupDashboard(t)

	// Use a project directory that doesn't exist to cause CreateSession to fail
	badDir := "/nonexistent/project/dir"
	body := `{"project_dir":"` + badDir + `","prompt":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleSessions(rr, req)

	// Session creation fails when project directory doesn't exist
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nonexistent project dir, got %d: %s", rr.Code, rr.Body.String())
	}
}

type boomError struct{ msg string }

func (e *boomError) Error() string { return e.msg }

func TestHandleHealth(t *testing.T) {
	srv, _, _ := setupDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	if _, ok := body["started_at"].(float64); !ok {
		t.Errorf("expected started_at to be a number, got %v", body["started_at"])
	}
}

// TestHandleSessionsIdempotentCreateSession ensures retrying create-session
// for the same project dir returns the existing session instead of a duplicate.
func TestHandleSessionsIdempotentCreateSession(t *testing.T) {
	srv, st, projDir := setupDashboard(t)
	// No adapter needed - we use the manager now

	body := `{"project_dir":"` + projDir + `","prompt":"hello"}`
	makeRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		srv.handleSessions(rr, req)
		return rr
	}

	first := makeRequest()
	if first.Code != http.StatusCreated {
		t.Fatalf("expected 201 on first create, got %d: %s", first.Code, first.Body.String())
	}
	var firstBody map[string]interface{}
	json.Unmarshal(first.Body.Bytes(), &firstBody)
	firstID := firstBody["session_id"].(string)

	second := makeRequest()
	if second.Code != http.StatusOK {
		t.Fatalf("expected 200 on idempotent retry, got %d: %s", second.Code, second.Body.String())
	}
	var secondBody map[string]interface{}
	json.Unmarshal(second.Body.Bytes(), &secondBody)
	if secondBody["session_id"] != firstID {
		t.Errorf("expected idempotent retry to return session %s, got %v", firstID, secondBody["session_id"])
	}

	// Only one session persisted in SQLite.
	sessions, err := st.ListSessions()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session in store after idempotent retry, got %d", len(sessions))
	}
}

// TestHandleSessionsCreateSessionPersistsPromptEvents ensures the initial
// prompt exchange is persisted as events so the user sees useful feedback.
func TestHandleSessionsCreateSessionPersistsPromptEvents(t *testing.T) {
	srv, st, projDir := setupDashboard(t)
	adptr := &mockDashboardAdapter{
		done: make(chan struct{}),
		sendResult: adapter.MessageResult{
			MessageID: "msg_1",
			Text:      "assistant reply",
			Finish:    "stop",
		},
	}
	srv.adapter = adptr

	body := `{"project_dir":"` + projDir + `","prompt":"do the thing"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleSessions(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var bodyData map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &bodyData)
	sessionID := bodyData["session_id"].(string)

	// Poll for persisted events: they are written after the subprocess completes,
	// so allow a short window for them to land.
	var events []*state.Event
	var err error
	for i := 0; i < 50; i++ {
		events, err = st.GetEvents(sessionID, 100)
		if err == nil && len(events) >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) >= 2 {
		if events[0].Type != "user_message" || events[1].Type != "assistant_message" {
			t.Errorf("expected user_message then assistant_message, got %s then %s", events[0].Type, events[1].Type)
		}
	}
}

// mockSessionManager is a test double for session.Manager that allows
// controlling CreateSession and SendMessage behavior for testing.
type mockSessionManager struct {
	createErr error
}

func (m *mockSessionManager) CreateSession(ctx context.Context, projectDir, model, prompt string) (*state.Session, *state.Task, error) {
	if m.createErr != nil {
		return nil, nil, m.createErr
	}
	// Return a minimal valid session for testing
	return &state.Session{
		ID:         "test-session-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		ProjectDir: projectDir,
		Agent:      "opencode",
		Model:      model,
		Status:     state.SessionStatusRunning,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, &state.Task{
		ID:        "test-task-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		SessionID: "",
		Prompt:    prompt,
		Status:    state.TaskStatusRunning,
		Scope:     fmt.Sprintf("project:%s", projectDir),
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (m *mockSessionManager) SendMessage(ctx context.Context, sessionID, message string) (*state.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockSessionManager) StopSession(sessionID string) error { return nil }
func (m *mockSessionManager) ListSessions() ([]*state.Session, error) { return nil, nil }
func (m *mockSessionManager) GetSession(id string) (*state.Session, error) { return nil, nil }
func (m *mockSessionManager) GetSessionTasks(sessionID string) ([]*state.Task, error) { return nil, nil }
func (m *mockSessionManager) GetSessionIncidents(sessionID string) ([]*state.Incident, error) { return nil, nil }
func (m *mockSessionManager) GetSessionEvents(sessionID string, limit int) ([]*state.Event, error) { return nil, nil }
func (m *mockSessionManager) SubmitTask(sessionID, prompt, scope string, dependencies []string) (*state.Task, error) { return nil, nil }
func (m *mockSessionManager) RequestApproval(approval *state.Approval) error { return nil }
func (m *mockSessionManager) RespondApproval(approvalID string, approved bool, decidedBy string) error { return nil }
func (m *mockSessionManager) CreateIncident(inc *state.Incident) error { return nil }
func (m *mockSessionManager) DiagnoseIncident(incidentID string) error { return nil }
func (m *mockSessionManager) GetRecoveryAction(id string) (*state.RecoveryAction, error) { return nil, nil }
func (m *mockSessionManager) ProposeRecovery(incidentID string, action *state.RecoveryAction) error { return nil }
func (m *mockSessionManager) RequestRecoveryApproval(recoveryID string) (*state.Approval, error) { return nil, nil }
func (m *mockSessionManager) ExecuteRecovery(recoveryID string) (*state.VerificationResult, error) { return nil, nil }
func (m *mockSessionManager) DetectIncidentsFromEvents(sessionID string) ([]*state.Incident, error) { return nil, nil }
func (m *mockSessionManager) RestoreSessions() error { return nil }
func (m *mockSessionManager) RestoreSession(sessionID string) (*state.Session, *state.Task, error) { return nil, nil, nil }
