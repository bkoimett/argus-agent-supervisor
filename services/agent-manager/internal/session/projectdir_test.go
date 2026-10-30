package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateSessionInvalidProjectDir verifies that invalid project directories are rejected.
func TestCreateSessionInvalidProjectDir(t *testing.T) {
	mgr, _, _ := setupTestManager(t)

	ctx := context.Background()

	// Test non-existent directory
	_, _, err := mgr.CreateSession(ctx, "/nonexistent/path/that/does/not/exist", "", "test prompt")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}

	// Test file instead of directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, _, err = mgr.CreateSession(ctx, filePath, "", "test prompt")
	if err == nil {
		t.Error("expected error for file path")
	}
}

// TestCreateSessionRelativePath verifies that relative paths are resolved to absolute paths.
func TestCreateSessionRelativePath(t *testing.T) {
	mgr, _, projDir := setupTestManager(t)

	ctx := context.Background()

	// Change to a known directory and use relative path
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	if err := os.Chdir(projDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	session, _, err := mgr.CreateSession(ctx, ".", "", "test prompt")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// The project directory should be resolved to absolute path
	expectedAbs, _ := filepath.Abs(".")
	if session.ProjectDir != expectedAbs {
		t.Errorf("project dir not normalized: got %q, want %q", session.ProjectDir, expectedAbs)
	}
}
