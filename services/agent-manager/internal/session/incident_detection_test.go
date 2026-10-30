package session

import (
	"context"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

// TestIncidentDetectionRegression tests various incident classification scenarios.
func TestIncidentDetectionRegression(t *testing.T) {
	tests := []struct {
		name               string
		event              *state.Event
		expectIncidentType string // empty means no incident expected
	}{
		{
			name: "pwd success exit 0 with output",
			event: &state.Event{
				SessionID: "sess-1",
				Type:      "tool_use",
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"tool_name":        "bash",
					"execution_status": "completed",
					"exit_code":        float64(0),
					"tool_output":      "/tmp/tmp.YwbocaAsgv",
					"tool_input":       map[string]interface{}{"command": "pwd"},
				},
			},
			expectIncidentType: "",
		},
		{
			name: "go test failure exit 1",
			event: &state.Event{
				SessionID: "sess-2",
				Type:      "tool_use",
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"tool_name":        "bash",
					"execution_status": "completed",
					"exit_code":        float64(1),
					"tool_output":      "FAIL  ExampleTest\n  main_test.go:15: Expected 2, got 3",
					"tool_input":       map[string]interface{}{"command": "go test ./..."},
				},
			},
			expectIncidentType: "test_failure",
		},
		{
			name: "generic command exit 1",
			event: &state.Event{
				SessionID: "sess-3",
				Type:      "tool_use",
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"tool_name":        "bash",
					"execution_status": "completed",
					"exit_code":        float64(1),
					"tool_output":      "some error",
					"tool_input":       map[string]interface{}{"command": "false"},
				},
			},
			expectIncidentType: "command_failure",
		},
		{
			name: "missing exit code (nil) -> no confirmed failure",
			event: &state.Event{
				SessionID: "sess-4",
				Type:      "tool_use",
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"tool_name":        "bash",
					"execution_status": "completed",
					"tool_output":      "output without exit",
					"tool_input":       map[string]interface{}{"command": "echo hello"},
				},
			},
			expectIncidentType: "",
		},
		{
			name: "harmless output containing word error but exit 0",
			event: &state.Event{
				SessionID: "sess-5",
				Type:      "tool_use",
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"tool_name":        "bash",
					"execution_status": "completed",
					"exit_code":        float64(0),
					"tool_output":      "this is an error message but command succeeded",
					"tool_input":       map[string]interface{}{"command": "echo 'error'"},
				},
			},
			expectIncidentType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, st, _ := setupTestManager(t)
			ctx := context.Background()
			sess, _, err := mgr.CreateSession(ctx, "/tmp", "", "test")
			if err != nil {
				t.Fatalf("CreateSession failed: %v", err)
			}
			// Update event session ID to match created session
			tt.event.SessionID = sess.ID

			// Persist event
			if err := st.AppendEvent(tt.event); err != nil {
				t.Fatalf("AppendEvent failed: %v", err)
			}
			// Run detection
			incidents, err := mgr.DetectIncidentsFromEvents(sess.ID)
			if err != nil {
				t.Fatalf("DetectIncidentsFromEvents failed: %v", err)
			}

			// Find incident for this event (last added)
			var found *state.Incident
			for _, inc := range incidents {
				if inc.Type == tt.expectIncidentType || (tt.expectIncidentType == "" && inc.Type != "") {
					found = inc
					break
				}
			}

			if tt.expectIncidentType == "" {
				if found != nil {
					t.Errorf("expected no incident, got %s: %s", found.Type, string(found.Evidence))
				}
			} else {
				if found == nil {
					t.Errorf("expected incident type %s, got none", tt.expectIncidentType)
				} else if found.Type != tt.expectIncidentType {
					t.Errorf("expected incident type %s, got %s", tt.expectIncidentType, found.Type)
				}
			}
		})
	}
}
