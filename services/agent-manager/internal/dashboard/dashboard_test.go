package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

	srv := NewServer(mgr, st, "0")
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
