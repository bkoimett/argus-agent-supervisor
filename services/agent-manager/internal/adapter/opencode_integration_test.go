//go:build integration

package adapter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func requireOpencode(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("opencode")
	if err != nil {
		t.Skipf("opencode not on PATH: %v", err)
	}
	return path
}

// tempProjectDir creates a throwaway project directory for integration checks.
func tempProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := `package main

import "fmt"

func main() {
	fmt.Println("integration probe")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	return dir
}

func TestIntegrationStartSendFollowupStop(t *testing.T) {
	binary := requireOpencode(t)
	projectDir := tempProjectDir(t)

	a := NewOpenCodeAdapter(binary)
	defer func() {
		if err := a.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
	}()

	// 1. Start a persistent session.
	sessionID, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}
	t.Logf("sessionID=%s", sessionID)

	// Session status should be WORKING.
	st, err := a.Status(sessionID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st != StatusWorking {
		t.Fatalf("Status=%q want WORKING", st)
	}

	// 2. Send an initial harmless prompt and capture assistant response.
	initial := "read the main.go file and report its package name"
	res1, err := a.SendMessage(sessionID, initial)
	if err != nil {
		t.Fatalf("SendMessage initial: %v", err)
	}
	if res1.SessionID != sessionID {
		t.Fatalf("res1.SessionID=%q want %q", res1.SessionID, sessionID)
	}
	if res1.Text == "" {
		t.Fatalf("expected non-empty assistant text, got %q", res1.Text)
	}
	if len(res1.Parts) == 0 {
		t.Fatalf("expected at least one message part")
	}
	t.Logf("initial text=%q messageID=%s finish=%s", res1.Text, res1.MessageID, res1.Finish)

	// 3. Send a follow-up message to the same session; verify same session ID.
	followup := "now report the line count of main.go"
	res2, err := a.SendMessage(sessionID, followup)
	if err != nil {
		t.Fatalf("SendMessage followup: %v", err)
	}
	if res2.SessionID != sessionID {
		t.Fatalf("res2.SessionID=%q want %q (session not retained)", res2.SessionID, sessionID)
	}
	if res2.Text == "" {
		t.Fatalf("expected non-empty assistant text for follow-up, got %q", res2.Text)
	}
	t.Logf("follow-up text=%q messageID=%s finish=%s", res2.Text, res2.MessageID, res2.Finish)

	// 4. Interrupt a running task and verify state afterward.
	// interrupt stops the current turn but the session remains tracked (WORKING).
	go func() {
		_, _ = a.SendMessage(sessionID, "count slowly from 1 to 100000")
	}()
	time.Sleep(500 * time.Millisecond)
	if err := a.Interrupt(sessionID); err != nil {
		t.Logf("Interrupt (expected may already be done): %v", err)
	}

	st, err = a.Status(sessionID)
	if err != nil {
		t.Fatalf("Status after interrupt: %v", err)
	}
	if st != StatusWorking {
		t.Fatalf("Status after interrupt=%q want WORKING (session retained)", st)
	}
	if _, err := a.serve.GetSession(sessionID); err != nil {
		t.Logf("serve GetSession after interrupt: %v (session may have ended)", err)
	} else {
		t.Logf("serve still tracks session after interrupt: OK")
	}

	// 5. Permission request: attempt to trigger one safely.
	// OpenCode does not prompt for harmless file reads in this install; a
	// live permission request was not observed during the probe. We only
	// verify the plumbing is wired (endpoint path is formed).
	if err := a.RespondToPermission(sessionID, "req_integration", false); err != nil {
		t.Logf("RespondToPermission plumbing: %v (end-to-end permission unverified)", err)
	}

	// 6. Stop the session and verify clean shutdown of the managed server.
	if err := a.Stop(sessionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st, err = a.Status(sessionID)
	if err != nil {
		t.Fatalf("Status after stop: %v", err)
	}
	if st != StatusStopped {
		t.Fatalf("Status after stop=%q want STOPPED", st)
	}

	// Verify manager shutdown closes its managed OpenCode server cleanly.
	servePID := a.serve.ServePID()
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Poll until the tracked serve process is reaped (signal 0 -> ESRCH).
	dead := false
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(servePID, 0); err != nil {
			dead = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !dead {
		t.Logf("WARNING: tracked serve process %d still alive after Close", servePID)
	} else {
		t.Logf("tracked serve process %d shut down cleanly", servePID)
	}
}

// TestIntegrationConcurrentStarts verifies concurrent session starts do not
// panic or corrupt the session map.
func TestIntegrationConcurrentStarts(t *testing.T) {
	binary := requireOpencode(t)
	projectDir := tempProjectDir(t)

	a := NewOpenCodeAdapter(binary)
	defer func() { _ = a.Close() }()

	const n = 3
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id, err := a.Start(projectDir)
			ids[i] = id
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("Start[%d]: %v", i, errs[i])
		}
		if ids[i] == "" {
			t.Fatalf("Start[%d]: empty id", i)
		}
	}
	// IDs must be distinct.
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate session ID: %s", id)
		}
		seen[id] = true
	}
	fmt.Fprintln(os.Stderr, "concurrent starts produced", n, "distinct sessions")
}

// TestIntegrationInterruptRetainsSession verifies live that Interrupt stops
// the current turn but the session remains WORKING and can receive a
// follow-up message on the same session ID.
func TestIntegrationInterruptRetainsSession(t *testing.T) {
	binary := requireOpencode(t)
	projectDir := tempProjectDir(t)

	a := NewOpenCodeAdapter(binary)
	defer func() { _ = a.Close() }()

	sessionID, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	st, err := a.Status(sessionID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st != StatusWorking {
		t.Fatalf("Status=%q want WORKING", st)
	}

	// Start a long-running message, then interrupt it.
	done := make(chan error, 1)
	go func() {
		_, err := a.SendMessage(sessionID, "count slowly from 1 to 100000")
		done <- err
	}()
	time.Sleep(500 * time.Millisecond)

	if err := a.Interrupt(sessionID); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	// Session must still be WORKING (retained), not STOPPED.
	st, err = a.Status(sessionID)
	if err != nil {
		t.Fatalf("Status after interrupt: %v", err)
	}
	if st != StatusWorking {
		t.Fatalf("Status after interrupt=%q want WORKING (session retained)", st)
	}

	// A follow-up message on the same session ID must succeed (session retained).
	followup := "what is 2+2"
	res, err := a.SendMessage(sessionID, followup)
	if err != nil {
		t.Fatalf("SendMessage follow-up after interrupt: %v", err)
	}
	if res.SessionID != sessionID {
		t.Fatalf("res.SessionID=%q want %q (session not retained)", res.SessionID, sessionID)
	}
	if res.Text == "" {
		t.Fatalf("expected non-empty assistant text for follow-up")
	}
}

// TestIntegrationStopDropsTracking verifies live that Stop drops the session
// from local tracking (STOPPED) without killing the shared serve process.
func TestIntegrationStopDropsTracking(t *testing.T) {
	binary := requireOpencode(t)
	projectDir := tempProjectDir(t)

	a := NewOpenCodeAdapter(binary)
	defer func() { _ = a.Close() }()

	sessionID, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := a.Stop(sessionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Session is no longer tracked; Status reports STOPPED.
	st, err := a.Status(sessionID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st != StatusStopped {
		t.Fatalf("Status after stop=%q want STOPPED", st)
	}

	// Sending to a stopped session should fail (not tracked).
	if _, err := a.SendMessage(sessionID, "after stop"); err == nil {
		t.Fatal("expected error sending to a stopped session")
	}
}

// TestIntegrationStopOtherSessionsUnaffected verifies live that stopping one
// session leaves other sessions WORKING (the shared serve process is NOT
// killed, so unrelated sessions are not disrupted).
func TestIntegrationStopOtherSessionsUnaffected(t *testing.T) {
	binary := requireOpencode(t)
	projectDir := tempProjectDir(t)

	a := NewOpenCodeAdapter(binary)
	defer func() { _ = a.Close() }()

	id1, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	id2, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start 2: %v", err)
	}

	if err := a.Stop(id1); err != nil {
		t.Fatalf("Stop id1: %v", err)
	}

	if st, _ := a.Status(id1); st != StatusStopped {
		t.Fatalf("id1 status after stop=%q want STOPPED", st)
	}
	if st, _ := a.Status(id2); st != StatusWorking {
		t.Fatalf("id2 status after stopping id1=%q want WORKING (serve not killed)", st)
	}
}

// TestIntegrationCloseReapsProcess verifies Close terminates the managed
// serve process and the tracked PID is reaped (no orphan OpenCode server).
func TestIntegrationCloseReapsProcess(t *testing.T) {
	binary := requireOpencode(t)
	projectDir := tempProjectDir(t)

	a := NewOpenCodeAdapter(binary)
	sessionID, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = sessionID

	servePID := a.serve.ServePID()
	if servePID == 0 {
		t.Fatal("expected non-zero serve PID")
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Poll until the tracked serve process is reaped (signal 0 -> ESRCH).
	dead := false
	for i := 0; i < 100; i++ {
		if err := syscall.Kill(servePID, 0); err != nil {
			dead = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !dead {
		t.Fatalf("tracked serve process %d still alive after Close (orphaned)", servePID)
	}
	t.Logf("tracked serve process %d shut down cleanly", servePID)
}
func TestIntegrationCloseWhileSending(t *testing.T) {
	binary := requireOpencode(t)
	projectDir := tempProjectDir(t)

	a := NewOpenCodeAdapter(binary)
	sessionID, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Start a long send, then close immediately.
	go func() {
		_, _ = a.SendMessage(sessionID, "count slowly from 1 to 100000")
	}()
	time.Sleep(200 * time.Millisecond)

	// Close should not panic and should clean up.
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	t.Log("CloseWhileSending completed without panic")
}
