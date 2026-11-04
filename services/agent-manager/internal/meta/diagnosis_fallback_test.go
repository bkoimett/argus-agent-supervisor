package meta

import (
	"testing"
)

func TestFallbackDiagnosis_TestFailure_NotFlaky(t *testing.T) {
	c := NewClient(DefaultConfig())

	req := DiagnosisRequest{
		IncidentID:   "inc-1",
		IncidentType: "test_failure",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"FAIL  TestAlways\n  main_test.go:10: Expected 1, got 2"}`),
		SessionID:    "sess-1",
	}

	resp, err := c.DiagnoseIncident(nil, req)
	if err != nil {
		t.Fatalf("DiagnoseIncident error: %v", err)
	}

	if resp.RecoveryPlan.ActionType == "RETRY" && !resp.RecoveryPlan.RequiresReview {
		t.Errorf("expected investigation not unconditional retry")
	}
	if !resp.RecoveryPlan.RequiresReview {
		t.Errorf("expected RequiresReview true")
	}
	if resp.Diagnosis == "" {
		t.Errorf("diagnosis empty")
	}
}

func TestFallbackDiagnosis_CompileError(t *testing.T) {
	c := NewClient(DefaultConfig())
	req := DiagnosisRequest{
		IncidentID:   "inc-2",
		IncidentType: "compile_error",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"main.go:5: undefined: foo"}`),
		SessionID:    "sess-2",
	}
	resp, err := c.DiagnoseIncident(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.RecoveryPlan.Command != "go" || len(resp.RecoveryPlan.Args) == 0 {
		t.Errorf("expected go build command")
	}
}

func TestFallbackDiagnosis_MissingDependency(t *testing.T) {
	c := NewClient(DefaultConfig())
	req := DiagnosisRequest{
		IncidentID:   "inc-3",
		IncidentType: "missing_dependency",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"go: finding module for package github.com/missing/pkg"}`),
		SessionID:    "sess-3",
	}
	resp, err := c.DiagnoseIncident(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.RecoveryPlan.Command != "go" || len(resp.RecoveryPlan.Args) == 0 {
		t.Errorf("expected go mod tidy")
	}
}
