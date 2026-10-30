package subprocess

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
)

// TestProcessManagerProjectDir verifies that the OpenCode process runs in the correct project directory.
func TestProcessManagerProjectDir(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a fake opencode binary that writes its working directory to a file
	fakeBin := filepath.Join(tmpDir, "fake-opencode")
	fakeBinContent := `#!/bin/sh
echo "WORKDIR: $(pwd)" > "` + filepath.Join(tmpDir, "workdir.txt") + `"
echo '{"type":"step_start","timestamp":1234567890,"sessionID":"test","part":{}}'
echo '{"type":"step_finish","timestamp":1234567891,"sessionID":"test","part":{"reason":"stop"}}'
`
	if err := os.WriteFile(fakeBin, []byte(fakeBinContent), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	// Create a project directory
	projDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	// Create a go.mod to make it look like a Go project
	if err := os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	// Create a session in the store (required for FK)
	session := &state.Session{
		ID:         "test-session-wd",
		ProjectDir: projDir,
		Agent:      "opencode",
		Status:     state.SessionStatusRunning,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := st.CreateSession(session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	procConfig := OpenCodeConfig{
		BinaryPath: fakeBin,
		Timeout:    10 * time.Second,
	}
	procMgr := NewProcessManager(st, procConfig)

	ctx := context.Background()
	_, err = procMgr.StartSession(ctx, "test-session-wd", projDir, "test prompt")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Wait a bit for the process to complete
	time.Sleep(500 * time.Millisecond)

	// Check the workdir file
	workdirFile := filepath.Join(tmpDir, "workdir.txt")
	content, err := os.ReadFile(workdirFile)
	if err != nil {
		t.Fatalf("read workdir file: %v", err)
	}

	expectedWorkdir := "WORKDIR: " + projDir + "\n"
	if string(content) != expectedWorkdir {
		t.Errorf("process ran in wrong directory\nexpected: %q\ngot: %q", expectedWorkdir, string(content))
	}
}
