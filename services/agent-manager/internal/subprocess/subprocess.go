// Package subprocess manages OpenCode subprocess execution and event parsing.
package subprocess

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
)

// OpenCodeConfig holds configuration for OpenCode subprocess.
type OpenCodeConfig struct {
	BinaryPath  string
	ProjectDir  string
	Model       string
	AutoApprove bool
	Env         []string
	Timeout     time.Duration
}

// ProcessManager manages OpenCode subprocesses.
type ProcessManager struct {
	store     *store.Store
	mu        sync.Mutex
	processes map[string]*ManagedProcess
	config    OpenCodeConfig
}

// ManagedProcess tracks a running OpenCode process.
type ManagedProcess struct {
	SessionID string
	Cmd       *exec.Cmd
	Stdout    io.ReadCloser
	Stderr    io.ReadCloser
	Cancel    context.CancelFunc
	PID       int
	StartTime time.Time
	EventChan chan *state.Event
	ErrorChan chan error
	Done      chan struct{}
	store     *store.Store
}

// NewProcessManager creates a new ProcessManager.
func NewProcessManager(st *store.Store, config OpenCodeConfig) *ProcessManager {
	if config.BinaryPath == "" {
		config.BinaryPath = "opencode"
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}
	return &ProcessManager{
		store:     st,
		processes: make(map[string]*ManagedProcess),
		config:    config,
	}
}

// StartSession starts a new OpenCode session and returns the session ID.
func (pm *ProcessManager) StartSession(ctx context.Context, sessionID, projectDir, prompt string) (*ManagedProcess, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.processes[sessionID]; exists {
		return nil, fmt.Errorf("session %s already running", sessionID)
	}

	// Validate and normalize project directory
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project directory: %w", err)
	}
	info, err := os.Stat(absProjectDir)
	if err != nil {
		return nil, fmt.Errorf("project directory not accessible: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path is not a directory: %s", absProjectDir)
	}

	args := []string{"run", "--format", "json", "--dir", absProjectDir}
	if pm.config.AutoApprove {
		args = append(args, "--auto")
	}
	if pm.config.Model != "" {
		args = append(args, "--model", pm.config.Model)
	}
	args = append(args, prompt)

	// Use a background context that we control, not the request context
	procCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, pm.config.BinaryPath, args...)
	cmd.Dir = absProjectDir
	cmd.Env = append(os.Environ(), pm.config.Env...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start process: %w", err)
	}

	mp := &ManagedProcess{
		SessionID: sessionID,
		Cmd:       cmd,
		Stdout:    stdout,
		Stderr:    stderr,
		Cancel:    cancel,
		PID:       cmd.Process.Pid,
		StartTime: time.Now(),
		EventChan: make(chan *state.Event, 100),
		ErrorChan: make(chan error, 10),
		Done:      make(chan struct{}),
		store:     pm.store,
	}

	pm.processes[sessionID] = mp

	go mp.readStdout()
	go mp.readStderr()
	go mp.wait()

	return mp, nil
}

// StopSession stops a running session.
func (pm *ProcessManager) StopSession(sessionID string) error {
	pm.mu.Lock()
	mp, exists := pm.processes[sessionID]
	pm.mu.Unlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	mp.Cancel()
	<-mp.Done
	return nil
}

// GetProcess returns the managed process for a session.
func (pm *ProcessManager) GetProcess(sessionID string) (*ManagedProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	mp, ok := pm.processes[sessionID]
	return mp, ok
}

// ListSessions returns all active session IDs.
func (pm *ProcessManager) ListSessions() []string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	ids := make([]string, 0, len(pm.processes))
	for id := range pm.processes {
		ids = append(ids, id)
	}
	return ids
}

// readStdout parses JSON lines from stdout.
func (mp *ManagedProcess) readStdout() {
	defer close(mp.EventChan)
	scanner := bufio.NewScanner(mp.Stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var ocEvent state.OpenCodeEvent
		if err := json.Unmarshal([]byte(line), &ocEvent); err != nil {
			mp.ErrorChan <- fmt.Errorf("parse event: %w", err)
			continue
		}

		event := mp.convertEvent(&ocEvent)
		if event != nil {
			select {
			case mp.EventChan <- event:
			default:
				// channel full, drop event
			}
			// Also persist to store
			if err := mp.store.AppendEvent(event); err != nil {
				mp.ErrorChan <- fmt.Errorf("persist event: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		mp.ErrorChan <- fmt.Errorf("read stdout: %w", err)
	}
}

// readStderr captures stderr for debugging.
func (mp *ManagedProcess) readStderr() {
	scanner := bufio.NewScanner(mp.Stderr)
	for scanner.Scan() {
		line := scanner.Text()
		// Log stderr but don't treat as error
		mp.ErrorChan <- fmt.Errorf("stderr: %s", line)
	}
}

// wait waits for process to exit.
func (mp *ManagedProcess) wait() {
	defer close(mp.Done)

	err := mp.Cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		mp.ErrorChan <- fmt.Errorf("process exited: %w", err)
	}

	// Update session in store
	session, err := mp.store.GetSession(mp.SessionID)
	if err == nil {
		session.Status = state.SessionStatusCompleted
		if exitCode != 0 {
			session.Status = state.SessionStatusError
		}
		now := time.Now()
		session.EndedAt = &now
		session.ExitCode = &exitCode
		mp.store.UpdateSession(session)
	}

	// Update tasks for this session
	taskStatus := state.TaskStatusCompleted
	if exitCode != 0 {
		taskStatus = state.TaskStatusFailed
	}
	_ = mp.store.UpdateTasksBySession(mp.SessionID, taskStatus, &exitCode)

	// Note: ProcessManager cleanup is handled elsewhere
}

// convertEvent converts OpenCode event to internal event.
func (mp *ManagedProcess) convertEvent(ocEvent *state.OpenCodeEvent) *state.Event {
	event := &state.Event{
		SessionID: mp.SessionID,
		Type:      ocEvent.Type,
		Timestamp: time.UnixMilli(ocEvent.Timestamp),
		Payload:   ocEvent.Part,
	}

	// Map OpenCode event types to internal types
	switch ocEvent.Type {
	case "step_start":
		event.Type = "step_start"
	case "tool_use":
		event.Type = "tool_use"
		// Normalize tool_use events to have consistent fields
		event.Payload = normalizeToolUseEvent(ocEvent.Part)
	case "step_finish":
		event.Type = "step_finish"
	case "text":
		event.Type = "text"
	default:
		event.Type = "unknown"
	}

	return event
}

// normalizeToolUseEvent extracts and normalizes tool execution data from OpenCode tool_use events.
func normalizeToolUseEvent(part map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{})

	// Copy all original fields
	for k, v := range part {
		normalized[k] = v
	}

	// Ensure tool name is accessible
	if tool, ok := part["tool"].(string); ok {
		normalized["tool_name"] = tool
	}

	// Extract state information
	if stateData, ok := part["state"].(map[string]interface{}); ok {
		// Normalize status
		if status, ok := stateData["status"].(string); ok {
			normalized["execution_status"] = status
		}

		// Extract input
		if input, ok := stateData["input"]; ok {
			normalized["tool_input"] = input
		}

		// Extract output
		if output, ok := stateData["output"]; ok {
			normalized["tool_output"] = output
		}

		// Extract metadata
		if metadata, ok := stateData["metadata"].(map[string]interface{}); ok {
			normalized["metadata"] = metadata
			if exitCode, ok := metadata["exit"]; ok {
				normalized["exit_code"] = exitCode
			}
			if metaOutput, ok := metadata["output"]; ok {
				normalized["metadata_output"] = metaOutput
			}
		}
	}

	// Ensure call ID is accessible
	if callID, ok := part["callID"].(string); ok {
		normalized["call_id"] = callID
	}

	return normalized
}

// Events returns the event channel for a session.
func (mp *ManagedProcess) Events() <-chan *state.Event {
	return mp.EventChan
}

// Errors returns the error channel for a session.
func (mp *ManagedProcess) Errors() <-chan error {
	return mp.ErrorChan
}

// TestAgent provides a controlled test agent for reproducible events.
type TestAgent struct {
	store     *store.Store
	sessionID string
	events    []*state.Event
	mu        sync.Mutex
}

// NewTestAgent creates a new controlled test agent.
func NewTestAgent(st *store.Store, sessionID string) *TestAgent {
	return &TestAgent{
		store:     st,
		sessionID: sessionID,
		events:    make([]*state.Event, 0),
	}
}

// EmitEvent adds a test event.
func (ta *TestAgent) EmitEvent(eventType string, payload map[string]interface{}) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	event := &state.Event{
		SessionID: ta.sessionID,
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	}
	ta.events = append(ta.events, event)
	ta.store.AppendEvent(event)
}

// EmitStepStart emits a step_start event.
func (ta *TestAgent) EmitStepStart(messageID string) {
	ta.EmitEvent("step_start", map[string]interface{}{
		"messageID": messageID,
		"sessionID": ta.sessionID,
		"type":      "step-start",
	})
}

// EmitToolUse emits a tool_use event.
func (ta *TestAgent) EmitToolUse(tool, callID string, input, output map[string]interface{}) {
	ta.EmitEvent("tool_use", map[string]interface{}{
		"tool":      tool,
		"callID":    callID,
		"input":     input,
		"output":    output,
		"sessionID": ta.sessionID,
	})
}

// EmitStepFinish emits a step_finish event.
func (ta *TestAgent) EmitStepFinish(reason string, tokens map[string]interface{}) {
	ta.EmitEvent("step_finish", map[string]interface{}{
		"reason":    reason,
		"tokens":    tokens,
		"sessionID": ta.sessionID,
	})
}

// EmitText emits a text event.
func (ta *TestAgent) EmitText(text string) {
	ta.EmitEvent("text", map[string]interface{}{
		"text":      text,
		"sessionID": ta.sessionID,
	})
}

// EmitError emits an error event.
func (ta *TestAgent) EmitError(errMsg string) {
	ta.EmitEvent("error", map[string]interface{}{
		"error":     errMsg,
		"sessionID": ta.sessionID,
	})
}

// GetEvents returns all emitted events.
func (ta *TestAgent) GetEvents() []*state.Event {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	events := make([]*state.Event, len(ta.events))
	copy(events, ta.events)
	return events
}

// ScenarioCompileError emits a compile error scenario.
func (ta *TestAgent) ScenarioCompileError() {
	ta.EmitStepStart("msg_compile_1")
	ta.EmitToolUse("bash", "call_1", map[string]interface{}{
		"command": "go build ./...",
	}, map[string]interface{}{
		"output": "main.go:10: undefined: foo\n",
		"exit":   1,
	})
	ta.EmitStepFinish("tool-calls", map[string]interface{}{"total": 100})
	ta.EmitError("compile error: undefined: foo")
}

// ScenarioTestFailure emits a test failure scenario.
func (ta *TestAgent) ScenarioTestFailure() {
	ta.EmitStepStart("msg_test_1")
	ta.EmitToolUse("bash", "call_2", map[string]interface{}{
		"command": "go test ./...",
	}, map[string]interface{}{
		"output": "FAIL  ExampleTest\n  main_test.go:15: Expected 2, got 3\n",
		"exit":   1,
	})
	ta.EmitStepFinish("tool-calls", map[string]interface{}{"total": 200})
	ta.EmitError("test failed")
}

// ScenarioMissingDependency emits a missing dependency scenario.
func (ta *TestAgent) ScenarioMissingDependency() {
	ta.EmitStepStart("msg_dep_1")
	ta.EmitToolUse("bash", "call_3", map[string]interface{}{
		"command": "go mod tidy",
	}, map[string]interface{}{
		"output": "go: finding module for package github.com/missing/pkg\n",
		"exit":   1,
	})
	ta.EmitStepFinish("tool-calls", map[string]interface{}{"total": 150})
	ta.EmitError("missing dependency")
}

// ScenarioSuccess emits a success scenario.
func (ta *TestAgent) ScenarioSuccess() {
	ta.EmitStepStart("msg_success_1")
	ta.EmitToolUse("bash", "call_4", map[string]interface{}{
		"command": "echo success",
	}, map[string]interface{}{
		"output": "success\n",
		"exit":   0,
	})
	ta.EmitStepFinish("stop", map[string]interface{}{"total": 50})
	ta.EmitText("Task completed successfully")
}
