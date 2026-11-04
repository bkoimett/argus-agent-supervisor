// Package state defines the core domain models for Argus agent supervision.
// Session, Task, Incident, Approval, Recovery, Verification states.
package state

import (
	"encoding/json"
	"time"
)

// SessionStatus represents the lifecycle state of a session.
type SessionStatus string

const (
	SessionStatusStarting     SessionStatus = "STARTING"
	SessionStatusRunning      SessionStatus = "RUNNING"
	SessionStatusWaiting      SessionStatus = "WAITING"
	SessionStatusParked       SessionStatus = "PARKED" // blocked on approval
	SessionStatusPaused       SessionStatus = "PAUSED"
	SessionStatusCompleted    SessionStatus = "COMPLETED"
	SessionStatusError        SessionStatus = "ERROR"
	SessionStatusCrashed      SessionStatus = "CRASHED"
	SessionStatusDisconnected SessionStatus = "DISCONNECTED"
	SessionStatusStopped      SessionStatus = "STOPPED"
	SessionStatusDetached     SessionStatus = "DETACHED" // Argus stopped tracking but cannot confirm OpenCode process termination (shared serve process has no per-session kill endpoint). Conflicts with PRD §10 state list; documented in PROGRESS.md.
)

// TaskStatus represents the state of a task within a session.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "PENDING"
	TaskStatusRunning    TaskStatus = "RUNNING"
	TaskStatusParked     TaskStatus = "PARKED"     // blocked on approval
	TaskStatusRecovering TaskStatus = "RECOVERING" // recovery in progress
	TaskStatusCompleted  TaskStatus = "COMPLETED"
	TaskStatusFailed     TaskStatus = "FAILED"
	TaskStatusSkipped    TaskStatus = "SKIPPED"
)

// IncidentSeverity represents the severity of an incident.
type IncidentSeverity string

const (
	IncidentSeverityLow      IncidentSeverity = "LOW"
	IncidentSeverityMedium   IncidentSeverity = "MEDIUM"
	IncidentSeverityHigh     IncidentSeverity = "HIGH"
	IncidentSeverityCritical IncidentSeverity = "CRITICAL"
)

// IncidentStatus represents the state of an incident.
type IncidentStatus string

const (
	IncidentStatusDetected   IncidentStatus = "DETECTED"
	IncidentStatusDiagnosing IncidentStatus = "DIAGNOSING"
	IncidentStatusProposed   IncidentStatus = "PROPOSED" // recovery plan proposed
	IncidentStatusApproved   IncidentStatus = "APPROVED" // recovery approved
	IncidentStatusRecovering IncidentStatus = "RECOVERING"
	IncidentStatusVerified   IncidentStatus = "VERIFIED"
	IncidentStatusFailed     IncidentStatus = "FAILED"
	IncidentStatusRejected   IncidentStatus = "REJECTED"
)

// ApprovalType represents the type of approval request.
type ApprovalType string

const (
	ApprovalTypePermission   ApprovalType = "PERMISSION"   // agent needs permission
	ApprovalTypeRecovery     ApprovalType = "RECOVERY"     // recovery plan needs approval
	ApprovalTypeVerification ApprovalType = "VERIFICATION" // verification result needs review
)

// ApprovalStatus represents the state of an approval request.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "PENDING"
	ApprovalStatusApproved ApprovalStatus = "APPROVED"
	ApprovalStatusRejected ApprovalStatus = "REJECTED"
	ApprovalStatusExpired  ApprovalStatus = "EXPIRED"
)

// RecoveryActionType represents the type of recovery action.
type RecoveryActionType string

const (
	RecoveryActionRetry    RecoveryActionType = "RETRY"
	RecoveryActionRestart  RecoveryActionType = "RESTART"
	RecoveryActionFallback RecoveryActionType = "FALLBACK"
	RecoveryActionPatch    RecoveryActionType = "PATCH"
	RecoveryActionSkip     RecoveryActionType = "SKIP"
)

// VerificationStatus represents the result of a verification.
type VerificationStatus string

const (
	VerificationStatusPassed     VerificationStatus = "PASSED"
	VerificationStatusFailed     VerificationStatus = "FAILED"
	VerificationStatusUnverified VerificationStatus = "UNVERIFIED"
	VerificationStatusTimeout    VerificationStatus = "TIMEOUT"
	VerificationStatusError      VerificationStatus = "ERROR"
)

// Session represents a single agent session.
type Session struct {
	ID                 string        `json:"id"`
	ProjectDir         string        `json:"project_dir"`
	Agent              string        `json:"agent"` // e.g., "opencode"
	Model              string        `json:"model"` // model identifier
	Status             SessionStatus `json:"status"`
	PID                int           `json:"pid,omitempty"`
	StartedAt          time.Time     `json:"started_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	EndedAt            *time.Time    `json:"ended_at,omitempty"`
	ExitCode           *int          `json:"exit_code,omitempty"`
	ErrorMessage       string        `json:"error_message,omitempty"`
	SupervisionEnabled bool          `json:"supervision_enabled"` // whether Argus assistant supervision is enabled for this session
}

// Task represents a unit of work within a session.
type Task struct {
	ID            string     `json:"id"`
	SessionID     string     `json:"session_id"`
	Prompt        string     `json:"prompt"`
	Status        TaskStatus `json:"status"`
	Dependencies  []string   `json:"dependencies"` // task IDs this depends on
	Scope         string     `json:"scope"`        // e.g., "project:foo", "test:bar"
	StartedAt     time.Time  `json:"started_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	ApprovalID    *string    `json:"approval_id,omitempty"`
	RecoveryCount int        `json:"recovery_count"`
}

// Incident represents a detected failure or anomaly.
type Incident struct {
	ID               string           `json:"id"`
	SessionID        string           `json:"session_id"`
	TaskID           *string          `json:"task_id,omitempty"`
	Type             string           `json:"type"` // e.g., "compile_error", "test_failure", "timeout"
	Severity         IncidentSeverity `json:"severity"`
	Status           IncidentStatus   `json:"status"`
	Evidence         json.RawMessage  `json:"evidence"` // redacted error output, stack trace
	MetaDiagnosis    string           `json:"meta_diagnosis,omitempty"`
	MetaRecovery     json.RawMessage  `json:"meta_recovery,omitempty"` // recovery plan from Meta
	MetaSource       string           `json:"meta_source,omitempty"`   // "live" or "fallback"
	MetaModel        string           `json:"meta_model,omitempty"`    // model name when live
	DetectedAt       time.Time        `json:"detected_at"`
	DiagnosedAt      *time.Time       `json:"diagnosed_at,omitempty"`
	ProposedAt       *time.Time       `json:"proposed_at,omitempty"`
	ApprovedAt       *time.Time       `json:"approved_at,omitempty"`
	RecoveredAt      *time.Time       `json:"recovered_at,omitempty"`
	VerifiedAt       *time.Time       `json:"verified_at,omitempty"`
	RecoveryAttempts int              `json:"recovery_attempts"`
}

// Approval represents a pending approval request.
type Approval struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	TaskID      *string         `json:"task_id,omitempty"`
	IncidentID  *string         `json:"incident_id,omitempty"`
	Type        ApprovalType    `json:"type"`
	Status      ApprovalStatus  `json:"status"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Payload     json.RawMessage `json:"payload"` // original permission request or recovery plan
	Scope       string          `json:"scope"`   // scope for dependency checking
	RequestedAt time.Time       `json:"requested_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	RespondedAt *time.Time      `json:"responded_at,omitempty"`
	Decision    *bool           `json:"decision,omitempty"` // true=approved, false=rejected
	DecidedBy   string          `json:"decided_by,omitempty"`
}

// RecoveryAction represents a proposed recovery action.
type RecoveryAction struct {
	ID          string              `json:"id"`
	IncidentID  string              `json:"incident_id"`
	Type        RecoveryActionType  `json:"type"`
	Description string              `json:"description"`
	Command     string              `json:"command"` // fixed allowlisted verification command
	Args        []string            `json:"args"`
	Status      IncidentStatus      `json:"status"` // reuse incident status
	ProposedAt  time.Time           `json:"proposed_at"`
	ApprovedAt  *time.Time          `json:"approved_at,omitempty"`
	ExecutedAt  *time.Time          `json:"executed_at,omitempty"`
	Result      *VerificationResult `json:"result,omitempty"`
}

// VerificationResult represents the outcome of a verification command.
type VerificationResult struct {
	Status      VerificationStatus `json:"status"`
	Command     string             `json:"command"`
	ExitCode    int                `json:"exit_code"`
	Stdout      string             `json:"stdout"`
	Stderr      string             `json:"stderr"`
	DurationMs  int64              `json:"duration_ms"`
	VerifiedAt  time.Time          `json:"verified_at"`
	IntegrityOK bool               `json:"integrity_ok"`
	NeedsReview bool               `json:"needs_review"`
	Notes       string             `json:"notes,omitempty"`
}

// Event represents a structured event from the agent subprocess.
type Event struct {
	SessionID string                 `json:"session_id"`
	Type      string                 `json:"type"` // step_start, tool_use, step_finish, text, error
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

// Command represents a command sent from the PWA to the agent.
type Command struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	Type      string          `json:"type"` // respond_permission, abort, pause, resume, stop
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"` // PENDING, EXECUTED, FAILED
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// OpenCodeEvent represents the raw event from opencode run --format=json
type OpenCodeEvent struct {
	Type      string                 `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	SessionID string                 `json:"sessionID"`
	Part      map[string]interface{} `json:"part"`
}
