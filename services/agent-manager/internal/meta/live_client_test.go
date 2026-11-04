package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLiveDiagnosis_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or invalid auth header")
		}
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"diagnosis":"Test failure due to incorrect assertion in test code.","recovery_plan":{"action_type":"SKIP","description":"Investigate test failure before retrying","command":"echo","args":["investigate test failure"],"risk_level":"LOW","requires_review":true}}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		Enabled: true,
	})

	req := DiagnosisRequest{
		IncidentID:   "inc-1",
		IncidentType: "test_failure",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"FAIL  TestAlways\n  main_test.go:10: Expected 1, got 2"}`),
		SessionID:    "sess-1",
	}

	resp, err := c.DiagnoseIncident(context.Background(), req)
	if err != nil {
		t.Fatalf("DiagnoseIncident error: %v", err)
	}

	if resp.Source != "live" {
		t.Errorf("expected source=live, got %s", resp.Source)
	}
	if resp.ModelVersion != "test-model" {
		t.Errorf("expected model=test-model, got %s", resp.ModelVersion)
	}
	if resp.Diagnosis == "" {
		t.Errorf("diagnosis empty")
	}
	if resp.RecoveryPlan.Command != "echo" {
		t.Errorf("expected command=echo, got %s", resp.RecoveryPlan.Command)
	}
}

func TestLiveDiagnosis_401Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		Enabled: true,
	})

	req := DiagnosisRequest{
		IncidentID:   "inc-1",
		IncidentType: "test_failure",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"FAIL"}`),
		SessionID:    "sess-1",
	}

	resp, err := c.DiagnoseIncident(context.Background(), req)
	if err != nil {
		t.Fatalf("DiagnoseIncident error: %v", err)
	}

	if resp.Source != "fallback" {
		t.Errorf("expected source=fallback on 401, got %s", resp.Source)
	}
	if resp.FallbackReason != "http 401" {
		t.Errorf("expected fallback_reason=http 401, got %s", resp.FallbackReason)
	}
}

func TestLiveDiagnosis_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	c := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		Enabled: true,
	})

	req := DiagnosisRequest{
		IncidentID:   "inc-1",
		IncidentType: "test_failure",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"FAIL"}`),
		SessionID:    "sess-1",
	}

	resp, err := c.DiagnoseIncident(context.Background(), req)
	if err != nil {
		t.Fatalf("DiagnoseIncident error: %v", err)
	}

	if resp.Source != "fallback" {
		t.Errorf("expected source=fallback on malformed json, got %s", resp.Source)
	}
	if resp.FallbackReason != "invalid json response" {
		t.Errorf("expected fallback_reason=invalid json response, got %s", resp.FallbackReason)
	}
}

func TestLiveDiagnosis_JSONInCodeFences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "```json\n{\"diagnosis\":\"Compile error due to undefined variable.\",\"recovery_plan\":{\"action_type\":\"PATCH\",\"description\":\"Fix undefined reference\",\"command\":\"go\",\"args\":[\"build\",\"./...\"],\"risk_level\":\"LOW\",\"requires_review\":true}}\n```",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		Enabled: true,
	})

	req := DiagnosisRequest{
		IncidentID:   "inc-1",
		IncidentType: "compile_error",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"main.go:5: undefined: foo"}`),
		SessionID:    "sess-1",
	}

	resp, err := c.DiagnoseIncident(context.Background(), req)
	if err != nil {
		t.Fatalf("DiagnoseIncident error: %v", err)
	}

	if resp.Source != "live" {
		t.Errorf("expected source=live, got %s", resp.Source)
	}
	if resp.RecoveryPlan.Command != "go" || len(resp.RecoveryPlan.Args) != 2 {
		t.Errorf("expected go build ./..., got %s %v", resp.RecoveryPlan.Command, resp.RecoveryPlan.Args)
	}
}

func TestLiveDiagnosis_DisallowedCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"diagnosis":"Malicious command detected.","recovery_plan":{"action_type":"PATCH","description":"Remove all files","command":"rm","args":["-rf", "/"],"risk_level":"HIGH","requires_review":true}}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		Enabled: true,
	})

	req := DiagnosisRequest{
		IncidentID:   "inc-1",
		IncidentType: "compile_error",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"main.go:5: undefined: foo"}`),
		SessionID:    "sess-1",
	}

	resp, err := c.DiagnoseIncident(context.Background(), req)
	if err != nil {
		t.Fatalf("DiagnoseIncident error: %v", err)
	}

	if resp.Source != "fallback" {
		t.Errorf("expected source=fallback for disallowed command, got %s", resp.Source)
	}
	if resp.FallbackReason != "command not allowlisted" {
		t.Errorf("expected fallback_reason=command not allowlisted, got %s", resp.FallbackReason)
	}
}

func TestLiveDiagnosis_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{}})
	}))
	defer server.Close()

	c := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		Enabled: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req := DiagnosisRequest{
		IncidentID:   "inc-1",
		IncidentType: "test_failure",
		Severity:     "HIGH",
		Evidence:     []byte(`{"output":"FAIL"}`),
		SessionID:    "sess-1",
	}

	resp, err := c.DiagnoseIncident(ctx, req)
	if err != nil {
		t.Fatalf("DiagnoseIncident error: %v", err)
	}

	if resp.Source != "fallback" {
		t.Errorf("expected source=fallback on timeout, got %s", resp.Source)
	}
}

func TestIsAllowedCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		args     []string
		expected bool
	}{
		{"echo single arg", "echo", []string{"hello"}, true},
		{"echo many args", "echo", []string{"a", "b", "c"}, true},
		{"echo too many args", "echo", make([]string, 17), false},
		{"echo arg too long", "echo", []string{string(make([]byte, 257))}, false},
		{"go test ./...", "go", []string{"test", "./..."}, true},
		{"go build ./...", "go", []string{"build", "./..."}, true},
		{"go vet ./...", "go", []string{"vet", "./..."}, true},
		{"go mod tidy", "go", []string{"mod", "tidy"}, true},
		{"go mod verify", "go", []string{"mod", "verify"}, true},
		{"go run main.go", "go", []string{"run", "main.go"}, false},
		{"rm -rf /", "rm", []string{"-rf", "/"}, false},
		{"ls", "ls", []string{"-la"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAllowedCommand(tt.command, tt.args)
			if result != tt.expected {
				t.Errorf("IsAllowedCommand(%q, %v) = %v, want %v", tt.command, tt.args, result, tt.expected)
			}
		})
	}
}
