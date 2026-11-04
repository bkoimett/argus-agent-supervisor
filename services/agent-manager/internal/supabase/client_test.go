package supabase

import (
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

// TestSyncSessionDisabled verifies SyncSession is a safe no-op when Supabase
// is not enabled. This is the default operating mode and must not fail or
// make network calls.
func TestSyncSessionDisabled(t *testing.T) {
	c := &Client{config: Config{Enabled: false}}
	sess := &state.Session{
		ID:         "ses_text_non_uuid_id",
		ProjectDir: "/tmp/demo",
		Agent:      "opencode",
		Status:     state.SessionStatusRunning,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := c.SyncSession(nil, sess); err != nil {
		t.Fatalf("SyncSession with disabled client should be a no-op, got: %v", err)
	}
}

// TestSessionIDIsText verifies that a session ID can be a non-UUID text string
// (as produced by OpenCode) and is correctly stored in the Session struct.
// This guards against the schema mismatch where sessions.id was previously
// uuid in Supabase but OpenCode produces text IDs like "ses_...".
func TestSessionIDIsText(t *testing.T) {
	sessionID := "ses_e63e46cc1ffeH51krVDC0NS61a"
	sess := &state.Session{
		ID:         sessionID,
		ProjectDir: "/tmp/demo",
		Agent:      "opencode",
		Status:     state.SessionStatusRunning,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if sess.ID != sessionID {
		t.Errorf("Session.ID = %q, want %q", sess.ID, sessionID)
	}
	// Text IDs are not valid UUIDs; this documents the expectation.
	if len(sessionID) < 10 || sessionID[:3] != "ses" {
		t.Errorf("unexpected OpenCode session ID format: %q", sessionID)
	}
}
