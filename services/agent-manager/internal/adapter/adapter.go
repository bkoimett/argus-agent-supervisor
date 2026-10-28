// Package adapter defines the AgentAdapter interface. Every coding agent
// Argus supports (OpenCode first, others later) implements this interface.
// Nothing outside this package should depend on an agent-specific type,
// see DESIGN.md "Module boundaries" at the repo root.
package adapter

// Status represents the lifecycle state of a session. Keep this in sync
// with docs/prd.md section 10.
type Status string

const (
	StatusStarting           Status = "STARTING"
	StatusWorking            Status = "WORKING"
	StatusWaiting            Status = "WAITING"
	StatusPermissionRequired Status = "PERMISSION_REQUIRED"
	StatusPaused             Status = "PAUSED"
	StatusCompleted          Status = "COMPLETED"
	StatusError              Status = "ERROR"
	StatusCrashed            Status = "CRASHED"
	StatusDisconnected       Status = "DISCONNECTED"
	StatusStopped            Status = "STOPPED"
)

// Event is a small, structured message emitted by an adapter. Keep events
// small, see docs/prd.md section 22, do not use this as a log dump.
type Event struct {
	SessionID string
	Type      string
	Payload   map[string]any
}

// AgentAdapter is the contract every supported coding agent must satisfy.
// TODO (M2): implement OpenCodeAdapter against this interface.
type AgentAdapter interface {
	Start(projectDir string) (sessionID string, err error)
	Stop(sessionID string) error
	Restart(sessionID string) error
	Status(sessionID string) (Status, error)
	SendMessage(sessionID string, message string) error
	Abort(sessionID string) error
	RespondToPermission(sessionID string, requestID string, approved bool) error
	GetEvents(sessionID string) ([]Event, error)
}
