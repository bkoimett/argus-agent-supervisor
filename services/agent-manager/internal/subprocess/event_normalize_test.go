package subprocess

import (
	"testing"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

func TestNormalizeToolUseEvent(t *testing.T) {
	// Test case 1: bash tool with error
	part := map[string]interface{}{
		"tool":   "bash",
		"callID": "call_123",
		"state": map[string]interface{}{
			"status": "error",
			"input": map[string]interface{}{
				"command": "go test ./...",
			},
			"output": "FAIL\n",
			"metadata": map[string]interface{}{
				"exit":   1,
				"output": "FAIL  TestExample\n",
			},
		},
	}

	normalized := normalizeToolUseEvent(part)

	// Check tool name
	if normalized["tool_name"] != "bash" {
		t.Errorf("tool_name not set: %v", normalized["tool_name"])
	}

	// Check call ID
	if normalized["call_id"] != "call_123" {
		t.Errorf("call_id not set: %v", normalized["call_id"])
	}

	// Check execution status
	if normalized["execution_status"] != "error" {
		t.Errorf("execution_status not set: %v", normalized["execution_status"])
	}

	// Check tool input
	input, ok := normalized["tool_input"].(map[string]interface{})
	if !ok || input["command"] != "go test ./..." {
		t.Errorf("tool_input not normalized: %v", normalized["tool_input"])
	}

	// Check tool output
	if normalized["tool_output"] != "FAIL\n" {
		t.Errorf("tool_output not set: %v", normalized["tool_output"])
	}

	// Check exit code
	if normalized["exit_code"] != 1 {
		t.Errorf("exit_code not set: %v", normalized["exit_code"])
	}

	// Check metadata output
	if normalized["metadata_output"] != "FAIL  TestExample\n" {
		t.Errorf("metadata_output not set: %v", normalized["metadata_output"])
	}

	// Test case 2: event without state
	part2 := map[string]interface{}{
		"tool":   "read",
		"callID": "call_456",
	}

	normalized2 := normalizeToolUseEvent(part2)

	if normalized2["tool_name"] != "read" {
		t.Errorf("tool_name not set for simple event: %v", normalized2["tool_name"])
	}
	if normalized2["call_id"] != "call_456" {
		t.Errorf("call_id not set for simple event: %v", normalized2["call_id"])
	}
	if _, ok := normalized2["execution_status"]; ok {
		t.Errorf("execution_status should not be set for simple event")
	}
}

func TestConvertEventToolUse(t *testing.T) {
	mp := &ManagedProcess{
		SessionID: "test-session",
	}

	ocEvent := &state.OpenCodeEvent{
		Type:      "tool_use",
		Timestamp: 1234567890000,
		SessionID: "test-session",
		Part: map[string]interface{}{
			"tool":   "bash",
			"callID": "call_123",
			"state": map[string]interface{}{
				"status": "error",
				"input": map[string]interface{}{
					"command": "go test ./...",
				},
				"output": "FAIL\n",
				"metadata": map[string]interface{}{
					"exit": 1,
				},
			},
		},
	}

	event := mp.convertEvent(ocEvent)

	if event.Type != "tool_use" {
		t.Errorf("event type not mapped: %s", event.Type)
	}

	// Check normalized fields
	if event.Payload["tool_name"] != "bash" {
		t.Errorf("tool_name not normalized in event: %v", event.Payload["tool_name"])
	}
	if event.Payload["execution_status"] != "error" {
		t.Errorf("execution_status not normalized: %v", event.Payload["execution_status"])
	}
	if event.Payload["exit_code"] != 1 {
		t.Errorf("exit_code not normalized: %v", event.Payload["exit_code"])
	}
}
