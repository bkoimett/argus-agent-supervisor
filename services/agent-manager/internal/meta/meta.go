// Package meta provides integration with the Meta model API for incident diagnosis and recovery proposals.
package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Config holds Meta API configuration.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Enabled bool
}

// DefaultConfig returns a default configuration (disabled).
func DefaultConfig() Config {
	return Config{
		BaseURL: "https://api.meta.example.com/v1",
		Model:   "meta-llama-3-70b",
		Enabled: false,
	}
}

// Client interacts with the Meta API.
type Client struct {
	config Config
	http   *http.Client
}

// NewClient creates a new Meta client.
func NewClient(config Config) *Client {
	if !config.Enabled {
		return &Client{config: config}
	}
	return &Client{
		config: config,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DiagnosisRequest represents a request for incident diagnosis.
type DiagnosisRequest struct {
	IncidentID   string          `json:"incident_id"`
	IncidentType string          `json:"incident_type"`
	Severity     string          `json:"severity"`
	Evidence     json.RawMessage `json:"evidence"`
	SessionID    string          `json:"session_id"`
	TaskID       *string         `json:"task_id,omitempty"`
}

// DiagnosisResponse represents a diagnosis and recovery proposal.
type DiagnosisResponse struct {
	Diagnosis    string       `json:"diagnosis"`
	RecoveryPlan RecoveryPlan `json:"recovery_plan"`
	Confidence   float64      `json:"confidence"`
	ModelVersion string       `json:"model_version"`
}

// RecoveryPlan represents a proposed recovery action.
type RecoveryPlan struct {
	ActionType     string          `json:"action_type"` // RETRY, RESTART, FALLBACK, PATCH, SKIP
	Description    string          `json:"description"`
	Command        string          `json:"command"` // allowlisted verification command
	Args           []string        `json:"args"`
	RiskLevel      string          `json:"risk_level"` // LOW, MEDIUM, HIGH
	RequiresReview bool            `json:"requires_review"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

// DiagnoseIncident sends incident evidence to Meta for diagnosis and recovery proposal.
func (c *Client) DiagnoseIncident(ctx context.Context, req DiagnosisRequest) (*DiagnosisResponse, error) {
	if !c.config.Enabled {
		return c.fallbackDiagnosis(req)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.config.BaseURL + "/diagnose"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		// Fallback on network error
		return c.fallbackDiagnosis(req)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackDiagnosis(req)
	}

	var diagResp DiagnosisResponse
	if err := json.NewDecoder(resp.Body).Decode(&diagResp); err != nil {
		return c.fallbackDiagnosis(req)
	}

	// Validate response
	if diagResp.Diagnosis == "" || diagResp.RecoveryPlan.Command == "" {
		return c.fallbackDiagnosis(req)
	}

	return &diagResp, nil
}

// fallbackDiagnosis provides deterministic diagnosis for seeded test failures.
func (c *Client) fallbackDiagnosis(req DiagnosisRequest) (*DiagnosisResponse, error) {
	var plan RecoveryPlan

	switch req.IncidentType {
	case "compile_error":
		plan = RecoveryPlan{
			ActionType:     "PATCH",
			Description:    "Fix compilation error by correcting undefined reference",
			Command:        "go",
			Args:           []string{"build", "./..."},
			RiskLevel:      "LOW",
			RequiresReview: true,
		}
	case "test_failure":
		plan = RecoveryPlan{
			ActionType:     "SKIP",
			Description:    "Test failure detected; investigate root cause before retrying",
			Command:        "echo",
			Args:           []string{"investigate test failure"},
			RiskLevel:      "LOW",
			RequiresReview: true,
		}
	case "missing_dependency":
		plan = RecoveryPlan{
			ActionType:     "FALLBACK",
			Description:    "Download missing dependencies",
			Command:        "go",
			Args:           []string{"mod", "tidy"},
			RiskLevel:      "LOW",
			RequiresReview: false,
		}
	default:
		plan = RecoveryPlan{
			ActionType:     "SKIP",
			Description:    "Skip this task and continue",
			Command:        "echo",
			Args:           []string{"skip"},
			RiskLevel:      "HIGH",
			RequiresReview: true,
		}
	}

	return &DiagnosisResponse{
		Diagnosis:    fmt.Sprintf("Fallback diagnosis for %s: %s", req.IncidentType, plan.Description),
		RecoveryPlan: plan,
		Confidence:   0.7,
		ModelVersion: "fallback-deterministic",
	}, nil
}

// RedactEvidence removes sensitive information from evidence before sending to Meta.
func RedactEvidence(evidence json.RawMessage) json.RawMessage {
	var data map[string]interface{}
	if err := json.Unmarshal(evidence, &data); err != nil {
		return evidence
	}

	// Remove potential secrets
	delete(data, "api_key")
	delete(data, "password")
	delete(data, "token")
	delete(data, "secret")

	redacted, _ := json.Marshal(data)
	return redacted
}
