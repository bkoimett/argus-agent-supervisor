// Package session manages agent sessions and tasks.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
	"github.com/argus-command-center/argus/services/agent-manager/internal/notification"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/argus-command-center/argus/services/agent-manager/internal/subprocess"
	"github.com/argus-command-center/argus/services/agent-manager/internal/supabase"
	"github.com/google/uuid"
)

// Manager coordinates sessions, tasks, incidents, approvals, and recovery.
type Manager struct {
	store       *store.Store
	procMgr     *subprocess.ProcessManager
	metaClient  *meta.Client
	supabase    *supabase.Client
	notifClient *notification.Client
	mu          sync.RWMutex
	eventSubs   map[string][]chan *state.Event
	taskQueue   chan *TaskExecution
	approvalCh  chan *ApprovalRequest
}

// TaskExecution represents a task to be executed.
type TaskExecution struct {
	Task     *state.Task
	Session  *state.Session
	Response chan error
}

// ApprovalRequest represents a request for approval.
type ApprovalRequest struct {
	Approval *state.Approval
	Response chan error
}

// NewManager creates a new session manager.
func NewManager(st *store.Store, procMgr *subprocess.ProcessManager, metaConfig meta.Config, sbClient *supabase.Client, notifClient *notification.Client) *Manager {
	m := &Manager{
		store:       st,
		procMgr:     procMgr,
		metaClient:  meta.NewClient(metaConfig),
		supabase:    sbClient,
		notifClient: notifClient,
		eventSubs:   make(map[string][]chan *state.Event),
		taskQueue:   make(chan *TaskExecution, 100),
		approvalCh:  make(chan *ApprovalRequest, 100),
	}

	go m.taskWorker()
	go m.approvalWorker()
	go m.eventDispatcher()

	return m
}

// CreateSession creates a new session and starts the OpenCode process.
func (m *Manager) CreateSession(ctx context.Context, projectDir, model, prompt string) (*state.Session, *state.Task, error) {
	// Validate and normalize project directory
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve project directory: %w", err)
	}
	info, err := os.Stat(absProjectDir)
	if err != nil {
		return nil, nil, fmt.Errorf("project directory not accessible: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("project path is not a directory: %s", absProjectDir)
	}

	sessionID := uuid.New().String()
	taskID := uuid.New().String()

	session := &state.Session{
		ID:         sessionID,
		ProjectDir: absProjectDir,
		Agent:      "opencode",
		Model:      model,
		Status:     state.SessionStatusStarting,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	task := &state.Task{
		ID:        taskID,
		SessionID: sessionID,
		Prompt:    prompt,
		Status:    state.TaskStatusPending,
		Scope:     fmt.Sprintf("project:%s", absProjectDir),
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.store.CreateSession(session); err != nil {
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	if err := m.store.CreateTask(task); err != nil {
		return nil, nil, fmt.Errorf("create task: %w", err)
	}

	// Sync to Supabase
	m.syncSession(session)
	m.syncTask(task)

	// Start the OpenCode process
	mp, err := m.procMgr.StartSession(ctx, sessionID, absProjectDir, prompt)
	if err != nil {
		session.Status = state.SessionStatusError
		session.ErrorMessage = err.Error()
		m.store.UpdateSession(session)
		m.syncSession(session)
		return nil, nil, fmt.Errorf("start opencode: %w", err)
	}

	session.Status = state.SessionStatusRunning
	session.PID = mp.PID
	m.store.UpdateSession(session)
	m.syncSession(session)

	task.Status = state.TaskStatusRunning
	m.store.UpdateTask(task)
	m.syncTask(task)

	return session, task, nil
}

// GetSession returns a session by ID.
func (m *Manager) GetSession(id string) (*state.Session, error) {
	return m.store.GetSession(id)
}

// ListSessions returns all sessions.
func (m *Manager) ListSessions() ([]*state.Session, error) {
	return m.store.ListSessions()
}

// StopSession stops a session.
func (m *Manager) StopSession(sessionID string) error {
	return m.procMgr.StopSession(sessionID)
}

// GetSessionTasks returns all tasks for a session.
func (m *Manager) GetSessionTasks(sessionID string) ([]*state.Task, error) {
	return m.store.ListTasksBySession(sessionID)
}

// GetSessionIncidents returns all incidents for a session.
func (m *Manager) GetSessionIncidents(sessionID string) ([]*state.Incident, error) {
	return m.store.ListIncidentsBySession(sessionID)
}

// GetIncident returns a single incident by ID.
func (m *Manager) GetIncident(id string) (*state.Incident, error) {
	return m.store.GetIncident(id)
}

// GetSessionEvents returns recent events for a session.
func (m *Manager) GetSessionEvents(sessionID string, limit int) ([]*state.Event, error) {
	return m.store.GetEvents(sessionID, limit)
}

// SubmitTask submits a new task for an existing session.
func (m *Manager) SubmitTask(sessionID, prompt, scope string, dependencies []string) (*state.Task, error) {
	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	taskID := uuid.New().String()
	task := &state.Task{
		ID:           taskID,
		SessionID:    sessionID,
		Prompt:       prompt,
		Status:       state.TaskStatusPending,
		Dependencies: dependencies,
		Scope:        scope,
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := m.store.CreateTask(task); err != nil {
		return nil, err
	}

	// Queue for execution
	resp := make(chan error, 1)
	m.taskQueue <- &TaskExecution{Task: task, Session: session, Response: resp}
	return task, <-resp
}

// RequestApproval creates an approval request with duplicate/stale detection.
func (m *Manager) RequestApproval(approval *state.Approval) error {
	// Check for duplicate pending approval for same task/incident
	pending, err := m.store.ListPendingApprovals()
	if err != nil {
		return err
	}
	for _, p := range pending {
		if approval.TaskID != nil && p.TaskID != nil && *p.TaskID == *approval.TaskID {
			return fmt.Errorf("duplicate approval request for task %s", *approval.TaskID)
		}
		if approval.IncidentID != nil && p.IncidentID != nil && *p.IncidentID == *approval.IncidentID {
			return fmt.Errorf("duplicate approval request for incident %s", *approval.IncidentID)
		}
	}

	if err := m.store.CreateApproval(approval); err != nil {
		return err
	}

	// Update task/session status to PARKED
	if approval.TaskID != nil {
		task, err := m.store.GetTask(*approval.TaskID)
		if err == nil {
			task.Status = state.TaskStatusParked
			task.ApprovalID = &approval.ID
			m.store.UpdateTask(task)
			m.syncTask(task)
		}
	}

	// Send notification for permission request
	m.sendApprovalNotification(approval)

	// Notify approval worker
	resp := make(chan error, 1)
	m.approvalCh <- &ApprovalRequest{Approval: approval, Response: resp}
	return <-resp
}

// RespondApproval handles an approval response.
func (m *Manager) RespondApproval(approvalID string, approved bool, decidedBy string) error {
	approval, err := m.store.GetApproval(approvalID)
	if err != nil {
		return err
	}

	if approval.Status != state.ApprovalStatusPending {
		return fmt.Errorf("approval not pending: %s", approval.Status)
	}

	if time.Now().After(approval.ExpiresAt) {
		return fmt.Errorf("approval expired")
	}

	// Validate recovery approval payload before any state changes
	var recoveryID string
	if approval.Type == state.ApprovalTypeRecovery {
		var payload struct {
			RecoveryID string `json:"recovery_id"`
		}
		if err := json.Unmarshal(approval.Payload, &payload); err != nil {
			return fmt.Errorf("invalid recovery approval payload: %w", err)
		}
		if payload.RecoveryID == "" {
			return fmt.Errorf("recovery approval missing recovery_id")
		}
		ra, err := m.store.GetRecoveryAction(payload.RecoveryID)
		if err != nil {
			return fmt.Errorf("get approved recovery action: %w", err)
		}
		if ra.IncidentID != *approval.IncidentID {
			return fmt.Errorf("recovery action does not match approval incident")
		}
		if ra.Status != state.IncidentStatusProposed {
			return fmt.Errorf("recovery action is no longer proposed: %s", ra.Status)
		}
		recoveryID = payload.RecoveryID
	}

	now := time.Now()

	// For recovery approvals, perform atomic updates via transaction.
	if approval.Type == state.ApprovalTypeRecovery && recoveryID != "" {
		if err := m.store.RespondRecoveryApprovalTx(approvalID, recoveryID, approved, decidedBy, now); err != nil {
			return err
		}
	} else {
		// Non-recovery approvals: sequential updates.
		approval.Status = state.ApprovalStatusApproved
		if !approved {
			approval.Status = state.ApprovalStatusRejected
		}
		approval.RespondedAt = &now
		approval.Decision = &approved
		approval.DecidedBy = decidedBy

		if err := m.store.UpdateApproval(approval); err != nil {
			return err
		}
		m.syncApproval(approval)

		// Update incident status
		if approval.IncidentID != nil {
			inc, err := m.store.GetIncident(*approval.IncidentID)
			if err == nil {
				if approved {
					inc.Status = state.IncidentStatusApproved
					inc.ApprovedAt = &now
				} else {
					inc.Status = state.IncidentStatusRejected
				}
				m.store.UpdateIncident(inc)
				m.syncIncident(inc)
			}
		}
	}

	// Update task status (applies to both recovery and non-recovery if TaskID present)
	if approval.TaskID != nil {
		task, err := m.store.GetTask(*approval.TaskID)
		if err == nil {
			if approved {
				task.Status = state.TaskStatusRunning
			} else {
				task.Status = state.TaskStatusFailed
			}
			task.ApprovalID = nil
			m.store.UpdateTask(task)
			m.syncTask(task)
		}
	}

	return nil
}

// GetPendingApprovals returns all pending approvals.
func (m *Manager) GetPendingApprovals() ([]*state.Approval, error) {
	return m.store.ListPendingApprovals()
}

// CreateIncident records a new incident.
func (m *Manager) CreateIncident(inc *state.Incident) error {
	err := m.store.CreateIncident(inc)
	if err == nil {
		m.syncIncident(inc)
	}
	return err
}

// DiagnoseIncident uses Meta to diagnose an incident and propose recovery.
func (m *Manager) DiagnoseIncident(incidentID string) error {
	inc, err := m.store.GetIncident(incidentID)
	if err != nil {
		return err
	}

	// Update status to diagnosing
	inc.Status = state.IncidentStatusDiagnosing
	now := time.Now()
	inc.DiagnosedAt = &now
	if err := m.store.UpdateIncident(inc); err != nil {
		return err
	}
	m.syncIncident(inc)

	// Prepare diagnosis request
	req := meta.DiagnosisRequest{
		IncidentID:   inc.ID,
		IncidentType: inc.Type,
		Severity:     string(inc.Severity),
		Evidence:     meta.RedactEvidence(inc.Evidence),
		SessionID:    inc.SessionID,
		TaskID:       inc.TaskID,
	}

	// Get diagnosis from Meta (or fallback)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := m.metaClient.DiagnoseIncident(ctx, req)
	if err != nil {
		// Diagnosis failed: mark incident as failed and store error
		inc.Status = state.IncidentStatusFailed
		inc.MetaDiagnosis = fmt.Sprintf("diagnosis error: %v", err)
		m.store.UpdateIncident(inc)
		m.syncIncident(inc)
		return fmt.Errorf("diagnosis failed: %w", err)
	}

	// Store diagnosis
	inc.MetaDiagnosis = resp.Diagnosis
	recoveryPlanBytes, _ := json.Marshal(resp.RecoveryPlan)
	inc.MetaRecovery = recoveryPlanBytes
	inc.Status = state.IncidentStatusProposed
	inc.ProposedAt = &now
	inc.RecoveryAttempts++
	if err := m.store.UpdateIncident(inc); err != nil {
		return err
	}
	m.syncIncident(inc)

	// Create recovery action from plan
	action := &state.RecoveryAction{
		ID:          uuid.New().String(),
		IncidentID:  incidentID,
		Type:        state.RecoveryActionType(resp.RecoveryPlan.ActionType),
		Description: resp.RecoveryPlan.Description,
		Command:     resp.RecoveryPlan.Command,
		Args:        resp.RecoveryPlan.Args,
		Status:      state.IncidentStatusProposed,
		ProposedAt:  now,
	}

	return m.store.CreateRecoveryAction(action)
}

// ProposeRecovery proposes a recovery action for an incident (manual proposal).
func (m *Manager) ProposeRecovery(incidentID string, action *state.RecoveryAction) error {
	inc, err := m.store.GetIncident(incidentID)
	if err != nil {
		return err
	}

	inc.Status = state.IncidentStatusProposed
	now := time.Now()
	inc.ProposedAt = &now
	inc.RecoveryAttempts++
	if err := m.store.UpdateIncident(inc); err != nil {
		return err
	}
	m.syncIncident(inc)

	action.IncidentID = incidentID
	action.Status = state.IncidentStatusProposed
	action.ProposedAt = now
	err = m.store.CreateRecoveryAction(action)
	if err == nil {
		m.syncRecoveryAction(action)
	}
	return err
}

// ListRecoveryActions returns all recovery actions, optionally filtered by incident.
func (m *Manager) ListRecoveryActions(incidentID string) ([]*state.RecoveryAction, error) {
	return m.store.ListRecoveryActions(incidentID)
}

// GetRecoveryAction returns a recovery action by ID.
func (m *Manager) GetRecoveryAction(id string) (*state.RecoveryAction, error) {
	return m.store.GetRecoveryAction(id)
}

// RequestRecoveryApproval creates an approval request for a recovery action.
func (m *Manager) RequestRecoveryApproval(recoveryID string) (*state.Approval, error) {
	ra, err := m.store.GetRecoveryAction(recoveryID)
	if err != nil {
		return nil, err
	}

	if ra.Status != state.IncidentStatusProposed {
		return nil, fmt.Errorf("recovery not in proposed state: %s", ra.Status)
	}

	inc, err := m.store.GetIncident(ra.IncidentID)
	if err != nil {
		return nil, err
	}

	approval := &state.Approval{
		ID:          uuid.New().String(),
		SessionID:   inc.SessionID,
		TaskID:      inc.TaskID,
		IncidentID:  &ra.IncidentID,
		Type:        state.ApprovalTypeRecovery,
		Status:      state.ApprovalStatusPending,
		Title:       "Recovery Approval: " + ra.Description,
		Description: fmt.Sprintf("Approve recovery action: %s (type: %s)", ra.Description, ra.Type),
		Payload:     json.RawMessage(fmt.Sprintf(`{"recovery_id":"%s","action_type":"%s","command":"%s %s"}`, ra.ID, ra.Type, ra.Command, strings.Join(ra.Args, " "))),
		Scope:       inc.SessionID, // Use session as scope for recovery
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}

	if err := m.RequestApproval(approval); err != nil {
		return nil, err
	}

	return approval, nil
}

// ExecuteRecovery executes an approved recovery action with verification.
func (m *Manager) ExecuteRecovery(recoveryID string) (*state.VerificationResult, error) {
	ra, err := m.store.GetRecoveryAction(recoveryID)
	if err != nil {
		return nil, err
	}

	if ra.Status != state.IncidentStatusApproved {
		return nil, fmt.Errorf("recovery not approved: %s", ra.Status)
	}

	// Enforce the command allowlist BEFORE any command is executed.
	if !m.verifyCommandIntegrity(ra.Command, ra.Args) {
		return nil, fmt.Errorf("recovery command is not allowlisted: %q %q", ra.Command, ra.Args)
	}

	// Update recovery status to recovering
	ra.Status = state.IncidentStatusRecovering
	executedAt := time.Now()
	ra.ExecutedAt = &executedAt
	if err := m.store.UpdateRecoveryAction(ra); err != nil {
		return nil, err
	}
	m.syncRecoveryAction(ra)

	// Update incident status
	inc, err := m.store.GetIncident(ra.IncidentID)
	if err != nil {
		return nil, err
	}
	inc.Status = state.IncidentStatusRecovering
	m.store.UpdateIncident(inc)
	m.syncIncident(inc)

	// Run the verification command (allowlisted) in the project directory
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sess, err := m.store.GetSession(inc.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get session for recovery: %w", err)
	}
	cmd := exec.CommandContext(ctx, ra.Command, ra.Args...)
	cmd.Dir = sess.ProjectDir
	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// Integrity check: verify the command hasn't been tampered with
	integrityOK := m.verifyCommandIntegrity(ra.Command, ra.Args)

	result := &state.VerificationResult{
		Command:     ra.Command + " " + strings.Join(ra.Args, " "),
		ExitCode:    exitCode,
		Stdout:      string(output),
		Stderr:      "",
		DurationMs:  duration.Milliseconds(),
		VerifiedAt:  time.Now(),
		IntegrityOK: integrityOK,
		Notes:       "",
	}

	// Determine verification status
	switch {
	case exitCode == 0 && integrityOK:
		result.Status = state.VerificationStatusPassed
		result.Notes = "Verification passed"
		result.NeedsReview = false
	case exitCode == 0 && !integrityOK:
		result.Status = state.VerificationStatusUnverified
		result.Notes = "Command succeeded but integrity check failed - needs review"
		result.NeedsReview = true
	case exitCode != 0 && integrityOK:
		result.Status = state.VerificationStatusFailed
		result.Notes = fmt.Sprintf("Verification failed with exit code %d", exitCode)
		result.NeedsReview = false
	case exitCode != 0 && !integrityOK:
		result.Status = state.VerificationStatusError
		result.Notes = "Verification failed and integrity check failed"
		result.NeedsReview = false
	default:
		result.Status = state.VerificationStatusError
		result.Notes = "Unknown verification state"
		result.NeedsReview = false
	}

	// Handle timeout
	if ctx.Err() == context.DeadlineExceeded {
		result.Status = state.VerificationStatusTimeout
		result.Notes = "Verification timed out"
		result.NeedsReview = false
	}

	ra.Result = result
	switch result.Status {
	case state.VerificationStatusPassed:
		ra.Status = state.IncidentStatusVerified
	case state.VerificationStatusUnverified:
		ra.Status = state.IncidentStatusVerified // marked verified but needs review
	case state.VerificationStatusFailed, state.VerificationStatusError, state.VerificationStatusTimeout:
		ra.Status = state.IncidentStatusFailed
	default:
		ra.Status = state.IncidentStatusFailed
	}
	if err := m.store.UpdateRecoveryAction(ra); err != nil {
		return nil, err
	}
	m.syncRecoveryAction(ra)

	// Update incident based on verification result
	inc, err = m.store.GetIncident(ra.IncidentID)
	if err != nil {
		return nil, err
	}

	verifiedAt := time.Now()
	switch result.Status {
	case state.VerificationStatusPassed:
		inc.Status = state.IncidentStatusVerified
		inc.VerifiedAt = &verifiedAt
		inc.RecoveredAt = &verifiedAt
	case state.VerificationStatusUnverified:
		inc.Status = state.IncidentStatusVerified // Mark as verified but needs review
		inc.VerifiedAt = &verifiedAt
		result.Notes += " - incident marked verified but requires manual review"
	case state.VerificationStatusFailed, state.VerificationStatusError, state.VerificationStatusTimeout:
		inc.Status = state.IncidentStatusFailed
		inc.RecoveryAttempts++
	}

	if err := m.store.UpdateIncident(inc); err != nil {
		return nil, err
	}
	m.syncIncident(inc)

	return result, nil
}

// verifyCommandIntegrity validates a recovery command before execution.
// Commands are invoked directly, never through a shell.
func (m *Manager) verifyCommandIntegrity(command string, args []string) bool {
	switch command {
	case "echo":
		// Echo is used for harmless demo/fallback actions. Bound its input.
		if len(args) > 16 {
			return false
		}
		for _, arg := range args {
			if len(arg) > 256 {
				return false
			}
		}
		return true

	case "go":
		// Permit only fixed, non-interactive verification commands.
		switch {
		case len(args) == 2 && args[0] == "test" && args[1] == "./...":
			return true
		case len(args) == 2 && args[0] == "build" && args[1] == "./...":
			return true
		case len(args) == 2 && args[0] == "vet" && args[1] == "./...":
			return true
		case len(args) == 2 && args[0] == "mod" && args[1] == "verify":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// SubscribeEvents subscribes to session events.
func (m *Manager) SubscribeEvents(sessionID string) (<-chan *state.Event, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan *state.Event, 100)
	m.eventSubs[sessionID] = append(m.eventSubs[sessionID], ch)

	unsub := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		subs := m.eventSubs[sessionID]
		for i, c := range subs {
			if c == ch {
				m.eventSubs[sessionID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}

	return ch, unsub
}

// eventDispatcher routes events from subprocess to subscribers.
func (m *Manager) eventDispatcher() {
	// This would be called for each managed process
	// For now, we'll poll or use a different mechanism
}

// taskWorker processes queued tasks.
func (m *Manager) taskWorker() {
	for exec := range m.taskQueue {
		_, ok := m.procMgr.GetProcess(exec.Session.ID)
		if !ok {
			exec.Response <- fmt.Errorf("session process not found")
			continue
		}

		// Check dependencies
		if !m.checkDependencies(exec.Task) {
			exec.Response <- fmt.Errorf("dependencies not met")
			continue
		}

		// For now, just mark as running
		exec.Task.Status = state.TaskStatusRunning
		m.store.UpdateTask(exec.Task)
		m.syncTask(exec.Task)
		exec.Response <- nil
	}
}

// approvalWorker processes approval requests.
func (m *Manager) approvalWorker() {
	for req := range m.approvalCh {
		// Just persist; actual response comes via RespondApproval
		req.Response <- nil
	}
}

// checkDependencies checks if all task dependencies are completed.
func (m *Manager) checkDependencies(task *state.Task) bool {
	for _, depID := range task.Dependencies {
		dep, err := m.store.GetTask(depID)
		if err != nil || dep.Status != state.TaskStatusCompleted {
			return false
		}
	}
	return true
}

// CanTaskRunIndependently checks if a task can run while another is parked.
func (m *Manager) CanTaskRunIndependently(newTask, parkedTask *state.Task) bool {
	// Different scopes = independent
	if newTask.Scope != parkedTask.Scope {
		return true
	}
	// Same scope = not independent, regardless of dependencies
	return false
}

// DetectIncidentsFromEvents analyzes recent events and creates incidents for failures.
func (m *Manager) DetectIncidentsFromEvents(sessionID string) ([]*state.Incident, error) {
	events, err := m.store.GetEvents(sessionID, 100)
	if err != nil {
		return nil, err
	}

	incidents := make([]*state.Incident, 0)
	for _, event := range events {
		incident := m.analyzeEventForIncident(event)
		if incident != nil {
			incident.SessionID = sessionID

			// Check for similar existing incident to correlate
			key := m.IncidentKey(incident.Type, string(incident.Evidence))
			existing, err := m.FindSimilarIncident(sessionID, key)
			if err != nil {
				return nil, err
			}

			if existing != nil {
				// Update existing incident with new occurrence
				if err := m.UpdateIncidentOccurrence(existing, event); err != nil {
					return nil, err
				}
				incidents = append(incidents, existing)
			} else {
				// Create new incident
				if err := m.store.CreateIncident(incident); err != nil {
					continue // Skip duplicate
				}
				// Send notification for new incident
				m.sendIncidentNotification(incident, notification.NotificationTypeError)
				incidents = append(incidents, incident)
			}
		}
	}
	return incidents, nil
}

// analyzeEventForIncident checks a single event for failure patterns.
func (m *Manager) analyzeEventForIncident(event *state.Event) *state.Incident {
	if event.Type != "tool_use" && event.Type != "error" {
		return nil
	}

	payload := event.Payload
	if payload == nil {
		return nil
	}

	// Support both old format (tool, state) and new normalized format (tool_name, execution_status, etc.)
	toolName := getToolName(payload)
	isBash := toolName == "bash"

	// Check execution status (both formats)
	execStatus := getExecutionStatus(payload)
	isError := execStatus == "error"

	// Check exit code (both formats)
	exitCode := getExitCode(payload)
	hasNonZeroExit := exitCode != 0

	// Get tool output (both formats)
	toolOutput := getToolOutput(payload)

	if isBash {
		// First, analyze tool output for specific failure patterns (highest priority)
		if toolOutput != "" {
			if inc := m.classifyBashFailure(event, payload, toolOutput); inc != nil {
				// Only accept generic command_failure if we have a non-zero exit code as evidence
				if inc.Type == "command_failure" && !hasNonZeroExit {
					// ignore generic failure without exit code evidence
				} else {
					return inc
				}
			}
		}

		// Check for non-zero exit code as reliable evidence of failure
		if hasNonZeroExit {
			return m.createIncidentFromError(event, payload, "command_failure")
		}
		// Do not create a command_failure incident based solely on error status or missing exit code
	}

	// Check for other tool failures
	if toolName != "" && isError {
		return m.createIncidentFromError(event, payload, "tool_failure")
	}

	// Check for explicit error events
	if event.Type == "error" {
		if errMsg, ok := payload["error"].(string); ok {
			evidenceBytes, _ := json.Marshal(map[string]string{"error": errMsg})
			return &state.Incident{
				ID:               uuid.New().String(),
				Type:             "agent_error",
				Severity:         state.IncidentSeverityHigh,
				Status:           state.IncidentStatusDetected,
				Evidence:         json.RawMessage(evidenceBytes),
				DetectedAt:       event.Timestamp,
				RecoveryAttempts: 0,
			}
		}
	}

	return nil
}

func getToolName(payload map[string]interface{}) string {
	// New normalized format
	if toolName, ok := payload["tool_name"].(string); ok {
		return toolName
	}
	// Old format
	if tool, ok := payload["tool"].(string); ok {
		return tool
	}
	return ""
}

func getExecutionStatus(payload map[string]interface{}) string {
	// New normalized format
	if status, ok := payload["execution_status"].(string); ok {
		return status
	}
	// Old format: check state.status
	if stateData, ok := payload["state"].(map[string]interface{}); ok {
		if status, ok := stateData["status"].(string); ok {
			return status
		}
	}
	return ""
}

func getExitCode(payload map[string]interface{}) float64 {
	// New normalized format
	if exitCode, ok := payload["exit_code"]; ok {
		if f, ok := exitCode.(float64); ok {
			return f
		}
	}
	// Old format: check state.metadata.exit
	if stateData, ok := payload["state"].(map[string]interface{}); ok {
		if metadata, ok := stateData["metadata"].(map[string]interface{}); ok {
			if exit, ok := metadata["exit"]; ok {
				if f, ok := exit.(float64); ok {
					return f
				}
			}
		}
	}
	return 0
}

func getToolOutput(payload map[string]interface{}) string {
	// New normalized format
	if output, ok := payload["tool_output"].(string); ok {
		return output
	}
	if output, ok := payload["metadata_output"].(string); ok {
		return output
	}
	// Old format: check state.output and state.metadata.output
	if stateData, ok := payload["state"].(map[string]interface{}); ok {
		if output, ok := stateData["output"].(string); ok {
			return output
		}
		if metadata, ok := stateData["metadata"].(map[string]interface{}); ok {
			if output, ok := metadata["output"].(string); ok {
				return output
			}
		}
	}
	return ""
}

// classifyBashFailure analyzes bash tool output to determine the specific failure category.
func (m *Manager) classifyBashFailure(event *state.Event, payload map[string]interface{}, output string) *state.Incident {
	// Network errors
	if containsAny(output, []string{
		"connection reset", "ECONNRESET", "connection refused", "timeout",
		"no route to host", "network unreachable", "DNS", "dial tcp",
		"i/o timeout", "connect: connection",
	}) {
		return m.createIncidentFromError(event, payload, "network_error")
	}

	// Authentication errors
	if containsAny(output, []string{
		"unauthorized", "authentication failed", "invalid credentials",
		"permission denied", "401", "403", "forbidden",
		"invalid token", "token expired", "expired token",
	}) {
		return m.createIncidentFromError(event, payload, "authentication_error")
	}

	// Rate limiting
	if containsAny(output, []string{
		"rate limit", "429", "too many requests", "quota exceeded",
		"rate exceeded", "throttle",
	}) {
		return m.createIncidentFromError(event, payload, "rate_limit")
	}

	// Quota exhaustion - require explicit evidence
	if containsAny(output, []string{
		"insufficient credits", "quota exhausted", "billing",
		"out of credits", "credit limit", "payment required",
		"subscription", "upgrade required",
	}) {
		return m.createIncidentFromError(event, payload, "quota_exhausted")
	}

	// Missing dependencies
	if containsAny(output, []string{
		"missing dependency", "cannot find module", "finding module", "no matching version",
		"module not found", "package not found", "could not import",
		"import error", "missing package",
	}) {
		return m.createIncidentFromError(event, payload, "missing_dependency")
	}

	// Compilation errors
	if containsAny(output, []string{
		"undefined:", "cannot find package", "import cycle",
		"syntax error", "type error", "compilation failed",
		"build failed", "compile error",
	}) {
		return m.createIncidentFromError(event, payload, "compile_error")
	}

	// Test failures
	if containsAny(output, []string{
		"FAIL", "Test failed", "assertion failed", "test failed",
		"panic:", "FAIL\t", "--- FAIL:",
	}) {
		return m.createIncidentFromError(event, payload, "test_failure")
	}

	// Generic command failure
	return m.createIncidentFromError(event, payload, "command_failure")
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(sub) > 0 && containsIgnoreCase(s, sub) {
			return true
		}
	}
	return false
}

func containsIgnoreCase(s, substr string) bool {
	if len(substr) == 0 {
		return false
	}
	// Simple case-insensitive check
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 = c1 + 'a' - 'A'
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 = c2 + 'a' - 'A'
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (m *Manager) createIncidentFromError(event *state.Event, stateData map[string]interface{}, incType string) *state.Incident {
	// Extract error details - support both old and new formats
	output := getToolOutput(stateData)

	// Classify the error
	severity := state.IncidentSeverityMedium
	incidentType := incType

	if strings.Contains(output, "undefined:") || strings.Contains(output, "cannot find package") || strings.Contains(output, "import cycle") {
		incidentType = "compile_error"
		severity = state.IncidentSeverityHigh
	} else if strings.Contains(output, "FAIL") || strings.Contains(output, "Test failed") || strings.Contains(output, "assertion failed") {
		incidentType = "test_failure"
		severity = state.IncidentSeverityHigh
	} else if strings.Contains(output, "missing dependency") || strings.Contains(output, "cannot find module") || strings.Contains(output, "finding module") || strings.Contains(output, "no matching version") {
		incidentType = "missing_dependency"
		severity = state.IncidentSeverityHigh
	}

	evidence := map[string]interface{}{
		"output":   output,
		"tool":     "bash",
		"event_id": payloadGetString(event.Payload, "id"),
	}

	evidenceBytes, _ := json.Marshal(evidence)

	return &state.Incident{
		ID:               uuid.New().String(),
		Type:             incidentType,
		Severity:         severity,
		Status:           state.IncidentStatusDetected,
		Evidence:         json.RawMessage(evidenceBytes),
		DetectedAt:       event.Timestamp,
		RecoveryAttempts: 0,
	}
}

// IncidentKey generates a key for correlating similar incidents.
func (m *Manager) IncidentKey(incidentType string, output string) string {
	// Create a normalized key based on type and error signature
	// This helps correlate repeated failures
	keyParts := []string{incidentType}

	// Extract key error signatures
	if strings.Contains(output, "undefined:") {
		keyParts = append(keyParts, "undefined")
	}
	if strings.Contains(output, "FAIL") || strings.Contains(output, "Test failed") {
		keyParts = append(keyParts, "test_failure")
	}
	if strings.Contains(output, "missing dependency") || strings.Contains(output, "cannot find module") || strings.Contains(output, "finding module") {
		keyParts = append(keyParts, "missing_dep")
	}
	if strings.Contains(output, "connection reset") || strings.Contains(output, "ECONNRESET") {
		keyParts = append(keyParts, "conn_reset")
	}
	if strings.Contains(output, "unauthorized") || strings.Contains(output, "authentication failed") {
		keyParts = append(keyParts, "auth_failure")
	}
	if strings.Contains(output, "rate limit") || strings.Contains(output, "429") {
		keyParts = append(keyParts, "rate_limit")
	}

	return strings.Join(keyParts, ":")
}

// FindSimilarIncident looks for existing incidents with the same key.
func (m *Manager) FindSimilarIncident(sessionID, incidentKey string) (*state.Incident, error) {
	incidents, err := m.store.ListIncidentsBySession(sessionID)
	if err != nil {
		return nil, err
	}

	for _, inc := range incidents {
		if m.IncidentKey(inc.Type, string(inc.Evidence)) == incidentKey {
			return inc, nil
		}
	}
	return nil, nil
}

// UpdateIncidentOccurrence updates an existing incident with new occurrence data.
func (m *Manager) UpdateIncidentOccurrence(incident *state.Incident, event *state.Event) error {
	now := time.Now()
	incident.DetectedAt = now // Update last seen time
	// Could add occurrence count field to Incident struct if needed
	return m.store.UpdateIncident(incident)
}

func payloadGetString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

// ContinueParkedTask resumes a task after approval.
// syncSession syncs session to Supabase.
func (m *Manager) syncSession(sess *state.Session) {
	if m.supabase == nil || !m.supabase.IsEnabled() {
		return
	}
	ctx := context.Background()
	if err := m.supabase.SyncSession(ctx, sess); err != nil {
		// Log but don't fail
		fmt.Printf("Supabase sync session error: %v\n", err)
	}
}

// syncTask syncs task to Supabase.
func (m *Manager) syncTask(task *state.Task) {
	if m.supabase == nil || !m.supabase.IsEnabled() {
		return
	}
	ctx := context.Background()
	if err := m.supabase.SyncTask(ctx, task); err != nil {
		fmt.Printf("Supabase sync task error: %v\n", err)
	}
}

// syncIncident syncs incident to Supabase.
func (m *Manager) syncIncident(inc *state.Incident) {
	if m.supabase == nil || !m.supabase.IsEnabled() {
		return
	}
	ctx := context.Background()
	if err := m.supabase.SyncIncident(ctx, inc); err != nil {
		fmt.Printf("Supabase sync incident error: %v\n", err)
	}
}

// syncApproval syncs approval to Supabase.
func (m *Manager) syncApproval(appr *state.Approval) {
	if m.supabase == nil || !m.supabase.IsEnabled() {
		return
	}
	ctx := context.Background()
	if err := m.supabase.SyncApproval(ctx, appr); err != nil {
		fmt.Printf("Supabase sync approval error: %v\n", err)
	}
}

// syncRecoveryAction syncs recovery action to Supabase.
func (m *Manager) syncRecoveryAction(ra *state.RecoveryAction) {
	if m.supabase == nil || !m.supabase.IsEnabled() {
		return
	}
	ctx := context.Background()
	if err := m.supabase.SyncRecoveryAction(ctx, ra); err != nil {
		fmt.Printf("Supabase sync recovery action error: %v\n", err)
	}
}

// syncEvent syncs event to Supabase.
func (m *Manager) syncEvent(evt *state.Event) {
	if m.supabase == nil || !m.supabase.IsEnabled() {
		return
	}
	ctx := context.Background()
	if err := m.supabase.SyncEvent(ctx, evt); err != nil {
		fmt.Printf("Supabase sync event error: %v\n", err)
	}
}

// sendApprovalNotification sends a push notification for an approval request.
func (m *Manager) sendApprovalNotification(approval *state.Approval) {
	if m.notifClient == nil || !m.notifClient.IsEnabled() {
		return
	}

	session, err := m.store.GetSession(approval.SessionID)
	if err != nil {
		return
	}

	notifType := notification.NotificationTypePermission
	if approval.Type == state.ApprovalTypeRecovery {
		notifType = notification.NotificationTypePermission // Recovery approvals also need attention
	}

	payload := notification.CreateNotificationPayload(notifType, session, map[string]interface{}{
		"approval_id":   approval.ID,
		"approval_type": string(approval.Type),
		"title":         approval.Title,
		"description":   approval.Description,
	})

	// In a real implementation, we'd fetch user subscriptions from Supabase
	// For now, we'll use ntfy fallback if configured
	_ = m.notifClient.Send(nil, payload)
}

// sendIncidentNotification sends a push notification for an incident.
func (m *Manager) sendIncidentNotification(inc *state.Incident, notifType notification.NotificationType) {
	if m.notifClient == nil || !m.notifClient.IsEnabled() {
		return
	}

	session, err := m.store.GetSession(inc.SessionID)
	if err != nil {
		return
	}

	payload := notification.CreateNotificationPayload(notifType, session, map[string]interface{}{
		"incident_id":   inc.ID,
		"incident_type": inc.Type,
		"severity":      string(inc.Severity),
	})

	_ = m.notifClient.Send(nil, payload)
}

func (m *Manager) ContinueParkedTask(taskID string) error {
	task, err := m.store.GetTask(taskID)
	if err != nil {
		return err
	}

	if task.Status != state.TaskStatusParked {
		return fmt.Errorf("task not parked: %s", task.Status)
	}

	if task.ApprovalID == nil {
		return fmt.Errorf("no approval ID on parked task")
	}

	approval, err := m.store.GetApproval(*task.ApprovalID)
	if err != nil {
		return err
	}

	if approval.Status != state.ApprovalStatusApproved {
		return fmt.Errorf("approval not approved: %s", approval.Status)
	}

	// Check for staleness - if the approval was for a different task state
	if approval.RespondedAt != nil && time.Since(*approval.RespondedAt) > 5*time.Minute {
		return fmt.Errorf("approval response too old")
	}

	task.Status = state.TaskStatusRunning
	task.ApprovalID = nil
	task.UpdatedAt = time.Now()
	return m.store.UpdateTask(task)
}
