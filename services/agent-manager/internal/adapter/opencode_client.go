// Package adapter provides the OpenCode implementation of AgentAdapter.
//
// Lifecycle decision (documented here, per requirement 2):
//
//   - The Go agent-manager owns a single, long-lived, detached `opencode serve`
//     process. It is started lazily on the first Start() call and is never a
//     child of the GUI (PRD section 14). The GUI only talks to the Go service
//     over its local channel, never to OpenCode directly.
//   - One serve process is reused across all sessions (requirement 2:
//     "Reuse a managed server where practical"). Sessions are created within it
//     via POST /api/session and addressed by their OpenCode session ID.
//   - Auto-approval is off by default (requirement 9). Permission approve/deny
//     plumbing is implemented to the extent supported by the probe
//     (requirement 7) but end-to-end permission handling is marked UNVERIFIED
//     because no live permission request was triggered during the probe.
//
// Verified against OpenCode 1.19.0 (see OPENCODE_COMPATIBILITY_PROBE.md):
//
//   - POST /api/session  {"directory":...}  -> creates persistent session
//   - POST /session/{sessionID}/message {"parts":[...]}  -> assistant text
//   - GET  /api/event              -> SSE stream of message.part.updated etc.
//   - POST /session/{sessionID}/interrupt  -> stop a session
//   - POST /api/session/{sessionID}/permission/{requestID}/reply  -> approve/deny
package adapter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ServeClient is an HTTP client for a headless `opencode serve` instance.
//
// It owns the underlying serve process when created via StartServe, keeping the
// process detached and long-lived (one per adapter, shared across sessions).
type ServeClient struct {
	baseURL    string
	httpClient *http.Client
	proc       *exec.Cmd // non-nil only when we spawned the serve process
}

// NewServeClient wraps an existing serve base URL (e.g. a test server).
// The caller owns the process lifecycle.
func NewServeClient(baseURL string) *ServeClient {
	return &ServeClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// StartServe spawns `opencode serve --port 0 --hostname 127.0.0.1` as a detached
// process, reads the announced listening port from stdout, and returns a client
// bound to it. The process is kept running until Close() is closed.
//
// opencode (1.19.0) forks when serving: the tracked parent exits shortly after
// start, leaving a child as the actual server. Killing only the tracked process
// is therefore a no-op and orphans the server. We place the process in its own
// process group (Setpgid) so Close() can kill the whole group, ensuring no
// orphan opencode serve processes survive adapter shutdown.
func StartServe(binaryPath string) (*ServeClient, error) {
	cmd := exec.Command(binaryPath, "serve", "--port", "0", "--hostname", "127.0.0.1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start opencode serve: %w", err)
	}

	port, err := readServePort(stdout)
	if err != nil {
		killProcessGroup(cmd)
		return nil, err
	}

	// Drain the rest of stdout so the pipe cannot block the serve process.
	go io.Copy(io.Discard, stdout)

	baseURL := fmt.Sprintf("http://127.0.0.1:%s", port)
	sc := &ServeClient{baseURL: baseURL, httpClient: &http.Client{Timeout: 30 * time.Second}, proc: cmd}

	// Wait briefly for the server to accept connections.
	if err := sc.waitForReady(5 * time.Second); err != nil {
		sc.Close()
		return nil, fmt.Errorf("serve not ready: %w", err)
	}
	return sc, nil
}

// readServePort reads the first stdout line and extracts the port from a line
// like "opencode server listening on http://127.0.0.1:4097".
func readServePort(stdout io.Reader) (string, error) {
	reader := bufio.NewReader(stdout)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("read serve stdout: %w", err)
		}
		if strings.Contains(line, "opencode server listening on") {
			return extractPort(line), nil
		}
		if line == "" && err == io.EOF {
			break
		}
	}
	return "", fmt.Errorf("serve did not announce a listening port")
}

func extractPort(line string) string {
	idx := strings.LastIndex(line, ":")
	if idx < 0 || idx >= len(line)-1 {
		return ""
	}
	port := strings.TrimSpace(line[idx+1:])
	// strip any non-digit trailing characters
	digits := strings.TrimRight(port, "\r\n ")
	for _, r := range digits {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return digits
}

// waitForReady polls /api/health until the server accepts connections.
func (sc *ServeClient) waitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, sc.baseURL+"/api/health", nil)
		if err != nil {
			return err
		}
		resp, err := sc.httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s/api/health", sc.baseURL)
}

// Close stops the serve process if we own it. It kills the entire process
// group so no orphan server survives shutdown, then waits for the tracked
// process to be reaped before returning.
func (sc *ServeClient) Close() error {
	if sc.proc != nil && sc.proc.Process != nil {
		killProcessGroup(sc.proc)
		time.Sleep(100 * time.Millisecond)
		if alive, err := processAlive(sc.proc.Process); err == nil && alive {
			return fmt.Errorf("serve process %d still alive after Close", sc.proc.Process.Pid)
		}
	}
	return nil
}

// processAlive reports whether p is still running via signal 0.
func processAlive(p *os.Process) (bool, error) {
	if err := syscall.Kill(p.Pid, 0); err != nil {
		return false, nil // ESRCH: no such process
	}
	return true, nil
}

// killProcessGroup kills the tracked process and its process group, then waits
// for it to be reaped. It is best-effort: errors are ignored because the
// process may have already exited (opencode's tracked process exits quickly).
func killProcessGroup(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGKILL) // kill whole process group
	_ = syscall.Kill(pid, syscall.SIGKILL)  // also kill tracked process directly
	_, _ = cmd.Process.Wait()               // ignore: process may already have exited
}

// ---------------------------------------------------------------------------
// Session API
// ---------------------------------------------------------------------------

type createSessionRequest struct {
	Directory string `json:"directory"`
}

type createSessionResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

type getSessionResponse struct {
	Data sessionInfo `json:"data"`
}

type sessionInfo struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
	ProjectID string `json:"projectID"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updated"`
	Version   string `json:"version"`
}

// CreateSession creates a persistent session in the serve instance.
func (sc *ServeClient) CreateSession(projectDir string) (string, error) {
	body, err := json.Marshal(createSessionRequest{Directory: projectDir})
	if err != nil {
		return "", fmt.Errorf("marshal create session: %w", err)
	}
	resp, err := sc.httpClient.Post(sc.baseURL+"/api/session", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("create session request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create session: status %d", resp.StatusCode)
	}
	var parsed createSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode create session: %w", err)
	}
	if parsed.Data.ID == "" {
		return "", fmt.Errorf("create session: empty session id in response")
	}
	return parsed.Data.ID, nil
}

// GetSession returns info for an existing session.
func (sc *ServeClient) GetSession(sessionID string) (*sessionInfo, error) {
	req, err := http.NewRequest(http.MethodGet, sc.baseURL+"/api/session/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get session request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get session: status %d", resp.StatusCode)
	}
	var parsed getSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode get session: %w", err)
	}
	return &parsed.Data, nil
}

// ---------------------------------------------------------------------------
// Message API
// ---------------------------------------------------------------------------

type messageRequest struct {
	Parts []messagePartRequest `json:"parts"`
}

type messagePartRequest struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type messageResponse struct {
	Info  messageInfoResponse   `json:"info"`
	Parts []messagePartResponse `json:"parts"`
}

type messageInfoResponse struct {
	ID        string             `json:"id"`
	SessionID string             `json:"sessionID"`
	Role      string             `json:"role"`
	Finish    string             `json:"finish"`
	Created   int64              `json:"created"`
	Completed int64              `json:"completed"`
	Cost      float64            `json:"cost"`
	Tokens    tokenUsageResponse `json:"tokens"`
}

type messagePartResponse struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	MessageID string `json:"messageID"`
	SessionID string `json:"sessionID"`
}

type tokenUsageResponse struct {
	Total     int `json:"total"`
	Input     int `json:"input"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
}

// SendMessage sends a user message to an existing session and returns the
// structured assistant result (text, parts, identifiers, usage).
func (sc *ServeClient) SendMessage(sessionID, message string) (MessageResult, error) {
	body, err := json.Marshal(messageRequest{
		Parts: []messagePartRequest{{Type: "text", Text: message}},
	})
	if err != nil {
		return MessageResult{}, fmt.Errorf("marshal message: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, sc.baseURL+"/session/"+sessionID+"/message", strings.NewReader(string(body)))
	if err != nil {
		return MessageResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return MessageResult{}, fmt.Errorf("send message request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MessageResult{}, fmt.Errorf("send message: status %d", resp.StatusCode)
	}
	var parsed messageResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return MessageResult{}, fmt.Errorf("decode message: %w", err)
	}

	var texts []string
	for _, p := range parsed.Parts {
		if p.Type == "text" {
			texts = append(texts, p.Text)
		}
	}
	parts := make([]MessagePart, len(parsed.Parts))
	for i, p := range parsed.Parts {
		parts[i] = MessagePart{
			Type:      p.Type,
			Text:      p.Text,
			ID:        p.ID,
			MessageID: p.MessageID,
			SessionID: p.SessionID,
		}
	}

	return MessageResult{
		MessageID: parsed.Info.ID,
		SessionID: parsed.Info.SessionID,
		Role:      parsed.Info.Role,
		Text:      strings.Join(texts, "\n"),
		Parts:     parts,
		Finish:    parsed.Info.Finish,
		Cost:      parsed.Info.Cost,
		Tokens: TokenUsage{
			Total:     parsed.Info.Tokens.Total,
			Input:     parsed.Info.Tokens.Input,
			Output:    parsed.Info.Tokens.Output,
			Reasoning: parsed.Info.Tokens.Reasoning,
		},
		CreatedAt:   unixMilli(parsed.Info.Created),
		CompletedAt: unixMilli(parsed.Info.Completed),
	}, nil
}

// ---------------------------------------------------------------------------
// Interrupt / Stop
// ---------------------------------------------------------------------------

// Interrupt stops the current turn in a session via the verified interrupt
// endpoint.
func (sc *ServeClient) Interrupt(sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, sc.baseURL+"/session/"+sessionID+"/interrupt", nil)
	if err != nil {
		return err
	}
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("interrupt request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("interrupt: status %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Permission
// ---------------------------------------------------------------------------

type permissionReplyRequest struct {
	Reply   string `json:"reply"`
	Message string `json:"message"`
}

// ReplyPermission sends an approve/deny decision for a permission request.
//
// END-TO-END UNVERIFIED: the reply endpoint contract was confirmed from the
// OpenCode frontend source, but no live permission request was triggered or
// answered during the 1.19.0 probe. Treat this path as unverified until a real
// request is demonstrated end-to-end.
func (sc *ServeClient) ReplyPermission(sessionID, requestID, reply, message string) error {
	body, err := json.Marshal(permissionReplyRequest{Reply: reply, Message: message})
	if err != nil {
		return fmt.Errorf("marshal permission reply: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost,
		sc.baseURL+"/api/session/"+sessionID+"/permission/"+requestID+"/reply",
		strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("permission reply request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("permission reply: status %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func unixMilli(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// ServePID returns the PID of the tracked serve process, or 0 if none.
func (sc *ServeClient) ServePID() int {
	if sc.proc == nil || sc.proc.Process == nil {
		return 0
	}
	return sc.proc.Process.Pid
}

func (sc *ServeClient) String() string {
	return sc.baseURL
}
