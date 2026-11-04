package adapter

import (
	"fmt"
	"sync"
)

// OpenCodeAdapter is the MVP implementation of AgentAdapter for OpenCode.
//
// It owns a single detached `opencode serve` process (started lazily on the
// first Start() call) shared across all sessions. All OpenCode-specific detail
// lives in this package, per DESIGN.md "Module boundaries".
type OpenCodeAdapter struct {
	mu         sync.Mutex
	serve      *ServeClient // lazily started, shared across sessions
	sessions   map[string]*sessionHandle
	binaryPath string
}

type sessionHandle struct {
	SessionID  string
	ProjectDir string
	ServeURL   string
	LastResult *MessageResult // structured result of the most recent message
	Status     Status         // WORKING while tracked, DETACHED after Stop (cannot confirm OpenCode process termination)
}

// NewOpenCodeAdapter constructs an OpenCodeAdapter. binaryPath may be empty to
// use the default "opencode" on PATH.
func NewOpenCodeAdapter(binaryPath string) *OpenCodeAdapter {
	if binaryPath == "" {
		binaryPath = "opencode"
	}
	return &OpenCodeAdapter{
		binaryPath: binaryPath,
		sessions:   make(map[string]*sessionHandle),
	}
}

var _ AgentAdapter = (*OpenCodeAdapter)(nil)

// Start creates a persistent OpenCode session and returns its session ID.
//
// The initial prompt is NOT sent here (the AgentAdapter Start contract takes
// only a project dir). The caller sends the initial prompt via SendMessage on
// the returned session ID, which uses the verified session message endpoint
// POST /session/{sessionID}/message.
func (a *OpenCodeAdapter) Start(projectDir string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.serve == nil {
		sc, err := StartServe(a.binaryPath)
		if err != nil {
			return "", fmt.Errorf("start opencode serve: %w", err)
		}
		a.serve = sc
	}

	sessionID, err := a.serve.CreateSession(projectDir)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	a.sessions[sessionID] = &sessionHandle{
		SessionID:  sessionID,
		ProjectDir: projectDir,
		ServeURL:   a.serve.baseURL,
		Status:     StatusWorking,
	}
	return sessionID, nil
}

// SendMessage sends a follow-up message to an existing session and returns the
// structured assistant result.
//
// The serve client is captured under the lock before the HTTP call so a
// concurrent Close() cannot nil-pointer-dereference it mid-request.
// Returns an error if the session is unknown or has been detached (Stopped).
func (a *OpenCodeAdapter) SendMessage(sessionID, message string) (MessageResult, error) {
	a.mu.Lock()
	handle, ok := a.sessions[sessionID]
	sc := a.serve
	a.mu.Unlock()
	if !ok {
		return MessageResult{}, fmt.Errorf("session %s not found", sessionID)
	}
	if handle.Status != StatusWorking {
		return MessageResult{}, fmt.Errorf("session %s is not active (status: %s)", sessionID, handle.Status)
	}
	if sc == nil {
		return MessageResult{}, fmt.Errorf("serve client not started")
	}

	result, err := sc.SendMessage(sessionID, message)
	if err != nil {
		return MessageResult{}, fmt.Errorf("send message: %w", err)
	}

	a.mu.Lock()
	handle.LastResult = &result
	a.mu.Unlock()
	return result, nil
}

// Status reports the session lifecycle state.
//
// WORKING if the session is actively tracked; DETACHED once it has been
// stopped via Stop/Abort (Argus dropped tracking, cannot confirm the OpenCode
// process terminated because the shared serve process has no per-session kill
// endpoint). Returns an error if the session was never started.
func (a *OpenCodeAdapter) Status(sessionID string) (Status, error) {
	a.mu.Lock()
	handle, ok := a.sessions[sessionID]
	a.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("session %s not found", sessionID)
	}
	return handle.Status, nil
}

// Stop detaches the session from local tracking and marks it DETACHED from
// Argus's perspective. It makes a best-effort attempt to interrupt the
// current turn so it is not left running, but it does NOT kill the shared
// `opencode serve` process: doing so would terminate every session and
// disrupt unrelated sessions. OpenCode 1.19.0 exposes no per-session
// termination endpoint, so Stop() cannot confirm that the session's OpenCode
// process exited. DETACHED means "no longer managed by Argus", not "process
// confirmed dead".
//
// Stop is idempotent: calling it on an already-detached or unknown session is
// a no-op and returns nil.
func (a *OpenCodeAdapter) Stop(sessionID string) error {
	a.mu.Lock()
	handle, ok := a.sessions[sessionID]
	if ok {
		handle.Status = StatusDetached
	}
	a.mu.Unlock()

	if !ok {
		// Idempotent: already detached or never tracked.
		return nil
	}

	// Best-effort: stop the current turn. Ignore errors; the session is
	// marked detached regardless because we cannot confirm termination
	// of one session on the shared serve process.
	if a.serve != nil {
		_ = a.serve.Interrupt(sessionID)
	}
	return nil
}

// Restart detaches the existing session (marks DETACHED) and starts a fresh
// session in the same project directory. The new session is tracked as WORKING.
// Does NOT claim the original OpenCode process/session was reattached.
func (a *OpenCodeAdapter) Restart(sessionID string) error {
	a.mu.Lock()
	handle, ok := a.sessions[sessionID]
	projectDir := ""
	if ok {
		projectDir = handle.ProjectDir
		handle.Status = StatusDetached
	}
	a.mu.Unlock()

	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if a.serve == nil {
		return fmt.Errorf("serve client not started")
	}
	if err := a.serve.Interrupt(sessionID); err != nil {
		return fmt.Errorf("interrupt session %s: %w", sessionID, err)
	}

	newID, err := a.serve.CreateSession(projectDir)
	if err != nil {
		return fmt.Errorf("recreate session: %w", err)
	}
	a.mu.Lock()
	a.sessions[newID] = &sessionHandle{
		SessionID:  newID,
		ProjectDir: projectDir,
		ServeURL:   a.serve.baseURL,
		Status:     StatusWorking,
	}
	a.mu.Unlock()
	return nil
}

// Abort is an alias for Stop: detach the session from local tracking.
// OpenCode 1.19.0 exposes no distinct per-session "abort" endpoint, so Abort
// has the same semantics as Stop (marks DETACHED, best-effort interrupt,
// shared serve process is NOT killed).
func (a *OpenCodeAdapter) Abort(sessionID string) error {
	return a.Stop(sessionID)
}

// Interrupt stops the current turn in a session via the verified interrupt
// endpoint without removing the session from local tracking. The session
// remains WORKING and can receive follow-up messages. Use Stop() when the
// session should be retired entirely (dropped from tracking).
//
// Interrupt is safe to repeat: if the session is not tracked it returns an
// error; if the current turn has already ended the serve endpoint still
// accepts the request.
func (a *OpenCodeAdapter) Interrupt(sessionID string) error {
	a.mu.Lock()
	_, ok := a.sessions[sessionID]
	sc := a.serve
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	if sc == nil {
		return fmt.Errorf("serve client not started")
	}
	if err := sc.Interrupt(sessionID); err != nil {
		return fmt.Errorf("interrupt session %s: %w", sessionID, err)
	}
	return nil
}

// RespondToPermission sends an approve/deny decision for a permission request.
//
// END-TO-END UNVERIFIED: the reply endpoint contract was confirmed from the
// OpenCode frontend source, but no live permission request was triggered or
// answered during the 1.19.0 probe. Do not claim this works until a real
// request is demonstrated end-to-end.
func (a *OpenCodeAdapter) RespondToPermission(sessionID, requestID string, approved bool) error {
	a.mu.Lock()
	_, ok := a.sessions[sessionID]
	sc := a.serve
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	if sc == nil {
		return fmt.Errorf("serve client not started")
	}

	reply := "deny"
	if approved {
		reply = "approve"
	}
	return sc.ReplyPermission(sessionID, requestID, reply, "")
}

// GetEvents returns preserved message parts from the last message result as
// structured events, keeping session identifiers and event data.
func (a *OpenCodeAdapter) GetEvents(sessionID string) ([]Event, error) {
	a.mu.Lock()
	handle, ok := a.sessions[sessionID]
	var result *MessageResult
	if ok && handle.LastResult != nil {
		result = handle.LastResult
	}
	a.mu.Unlock()

	if result == nil {
		return nil, nil
	}

	events := make([]Event, 0, len(result.Parts))
	for _, p := range result.Parts {
		events = append(events, Event{
			SessionID: sessionID,
			Type:      p.Type,
			Payload: map[string]any{
				"text":      p.Text,
				"id":        p.ID,
				"messageID": p.MessageID,
			},
		})
	}
	return events, nil
}

// Close stops the owned serve process and releases all session handles.
func (a *OpenCodeAdapter) Close() error {
	a.mu.Lock()
	sc := a.serve
	a.serve = nil
	a.sessions = make(map[string]*sessionHandle)
	a.mu.Unlock()

	if sc == nil {
		return nil
	}
	return sc.Close()
}
