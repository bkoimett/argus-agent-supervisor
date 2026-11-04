package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

func TestVerificationResult_NeedsReviewField(t *testing.T) {
	vr := state.VerificationResult{
		Status:      state.VerificationStatusUnverified,
		Command:     "go test ./...",
		ExitCode:    0,
		Stdout:      "ok",
		Stderr:      "",
		DurationMs:  10,
		VerifiedAt:  time.Now(),
		IntegrityOK: false,
		NeedsReview: true,
		Notes:       "needs review",
	}
	data, _ := json.Marshal(vr)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if v, ok := m["needs_review"]; !ok || v != true {
		t.Errorf("needs_review not true in JSON: %s", string(data))
	}
	if vr.NeedsReview != true {
		t.Errorf("NeedsReview field not set")
	}
}

func TestVerificationResult_PassedNoReview(t *testing.T) {
	vr := state.VerificationResult{
		Status:      state.VerificationStatusPassed,
		Command:     "go test ./...",
		ExitCode:    0,
		Stdout:      "ok",
		Stderr:      "",
		DurationMs:  10,
		VerifiedAt:  time.Now(),
		IntegrityOK: true,
		NeedsReview: false,
	}
	data, _ := json.Marshal(vr)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if v, ok := m["needs_review"]; !ok || v != false {
		t.Errorf("needs_review should be false: %s", string(data))
	}
}
