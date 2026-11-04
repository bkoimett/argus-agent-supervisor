// Package adapter defines the AgentAdapter interface. Every coding agent
// Argus supports (OpenCode first, others later) implements this interface.
// Nothing outside this package should depend on an agent-specific type,
// see DESIGN.md "Module boundaries" at the repo root.
package adapter

import "time"

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
	StatusDetached           Status = "DETACHED" // Argus stopped tracking but cannot confirm OpenCode process termination.
)

// MessageResult is the structured result of sending a message to a coding
// agent session. It preserves the assistant text, session/message identifiers,
// finish reason, and token/cost usage observed from the agent.
type MessageResult struct {
	MessageID   string
	SessionID   string
	Role        string
	Text        string        // concatenated assistant text parts
	Parts       []MessagePart // raw message parts
	Finish      string        // e.g. "stop", "max_tokens"
	Cost        float64
	Tokens      TokenUsage
	CreatedAt   time.Time
	CompletedAt time.Time
}

// MessagePart is a single part of an agent message.
type MessagePart struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	MessageID string `json:"messageID"`
	SessionID string `json:"sessionID"`
}

// TokenUsage summarizes token and cost usage for a message.
type TokenUsage struct {
	Total     int `json:"total"`
	Input     int `json:"input"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
}

// Event is a small, structured message emitted by an adapter. Keep events
// small, see docs/prd.md section 22, do not use this as a log dump.
type Event struct {
	SessionID string
	Type      string
	Payload   map[string]any
}

// AgentAdapter is the contract every supported coding agent must satisfy.
// OpenCodeAdapter implements this against the verified `opencode serve`
// headless HTTP interface (OpenCode 1.19.0). See OPENCODE_COMPATIBILITY_PROBE.md.
type AgentAdapter interface {
	Start(projectDir string) (sessionID string, err error)
	Stop(sessionID string) error
	Restart(sessionID string) error
	Status(sessionID string) (Status, error)
	SendMessage(sessionID string, message string) (MessageResult, error)
	Interrupt(sessionID string) error
	Abort(sessionID string) error
	RespondToPermission(sessionID string, requestID string, approved bool) error
	GetEvents(sessionID string) ([]Event, error)
}
