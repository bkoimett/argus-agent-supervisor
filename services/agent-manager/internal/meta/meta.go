// Package meta provides integration with the Meta model API for incident diagnosis and recovery proposals.
package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		Enabled: false,
	}
}

// IsAllowedCommand validates that a command and args are in the allowlist.
// Exported so manager.go can use the same allowlist.
func IsAllowedCommand(command string, args []string) bool {
	switch command {
	case "echo":
		if len(args) > 16 {
			return false
		}
		for _, arg := range args {
			if len(arg) > 256 {
				return false
			}
		}
		return true
	case "go":
		switch {
		case len(args) == 2 && args[0] == "test" && args[1] == "./...":
			return true
		case len(args) == 2 && args[0] == "build" && args[1] == "./...":
			return true
		case len(args) == 2 && args[0] == "vet" && args[1] == "./...":
			return true
		case len(args) == 2 && args[0] == "mod" && args[1] == "verify":
			return true
		case len(args) == 2 && args[0] == "mod" && args[1] == "tidy":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// Client interacts with the OpenAI-compatible Meta API.
type Client struct {
	config Config
	http   *http.Client
}

// NewClient creates a new Meta client.
// Live mode is enabled only when both APIKey and BaseURL are non-empty.
func NewClient(config Config) *Client {
	enabled := config.APIKey != "" && config.BaseURL != ""
	if !enabled {
		return &Client{config: Config{APIKey: config.APIKey, BaseURL: strings.TrimSuffix(config.BaseURL, "/"), Model: config.Model, Enabled: false}, http: &http.Client{Timeout: 30 * time.Second}}
	}
	return &Client{
		config: Config{
			APIKey:  config.APIKey,
			BaseURL: strings.TrimSuffix(config.BaseURL, "/"),
			Model:   config.Model,
			Enabled: true,
		},
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsEnabled returns whether the Meta client is enabled (has valid API key and base URL).
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// HasAPIKey reports whether an API key is present, independent of base URL.
func (c *Client) HasAPIKey() bool { return c.config.APIKey != "" }

// GetBaseURL returns the base URL of the Meta API.
func (c *Client) GetBaseURL() string {
	return c.config.BaseURL
}

// GetModel returns the model name.
func (c *Client) GetModel() string {
	return c.config.Model
}

// UpdateConfig updates the Meta client configuration.
func (c *Client) UpdateConfig(baseURL, apiKey, model string) {
	enabled := apiKey != "" && baseURL != ""
	c.config = Config{
		APIKey:  apiKey,
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
		Enabled: enabled,
	}
}

// WithOverrides returns a client using the given values, falling back to
// this client's stored API key when key is blank. The key is never exposed.
func (c *Client) WithOverrides(baseURL, apiKey, model string) *Client {
	if apiKey == "" {
		apiKey = c.config.APIKey
	}
	if model == "" {
		model = c.config.Model
	}
	return NewClient(Config{APIKey: apiKey, BaseURL: baseURL, Model: model})
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
	Diagnosis      string       `json:"diagnosis"`
	RecoveryPlan   RecoveryPlan `json:"recovery_plan"`
	Confidence     float64      `json:"confidence"`
	ModelVersion   string       `json:"model_version"`
	Source         string       `json:"source"`          // "live" or "fallback"
	FallbackReason string       `json:"fallback_reason"` // reason if fallback used
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
		return c.fallbackDiagnosis(req, "not configured")
	}

	// Truncate evidence to last 4000 characters
	evidenceStr := string(req.Evidence)
	if len(evidenceStr) > 4000 {
		evidenceStr = evidenceStr[len(evidenceStr)-4000:]
	}

	systemPrompt := `You diagnose failures of an AI coding agent. Reply with ONLY a JSON object with fields: diagnosis (2 to 4 plain language sentences naming the likely root cause), action_type, command, args, risk_level. Only these commands are allowed: go test ./..., go build ./..., go vet ./..., go mod tidy, go mod verify.`

	userPrompt := fmt.Sprintf(`Incident type: %s
Severity: %s
Evidence (truncated to last 4000 chars):
%s`, req.IncidentType, req.Severity, evidenceStr)

	chatReq := map[string]interface{}{
		"model": c.config.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.1,
		"max_tokens":  600,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return c.fallbackDiagnosis(req, "marshal request failed")
	}

	url := c.config.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return c.fallbackDiagnosis(req, "create request failed")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return c.fallbackDiagnosis(req, "http error: "+err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackDiagnosis(req, fmt.Sprintf("http %d", resp.StatusCode))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return c.fallbackDiagnosis(req, "invalid json response")
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		return c.fallbackDiagnosis(req, "empty response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip code fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var diagResp DiagnosisResponse
	if err := json.Unmarshal([]byte(content), &diagResp); err != nil {
		return c.fallbackDiagnosis(req, "invalid json in content")
	}

	// Validate required fields
	if diagResp.Diagnosis == "" || diagResp.RecoveryPlan.Command == "" {
		return c.fallbackDiagnosis(req, "missing required fields")
	}

	// Validate command against allowlist
	if !IsAllowedCommand(diagResp.RecoveryPlan.Command, diagResp.RecoveryPlan.Args) {
		return c.fallbackDiagnosis(req, "command not allowlisted")
	}

	diagResp.Source = "live"
	diagResp.ModelVersion = c.config.Model
	return &diagResp, nil
}

// fallbackDiagnosis provides deterministic diagnosis for seeded test failures.
func (c *Client) fallbackDiagnosis(req DiagnosisRequest, reason string) (*DiagnosisResponse, error) {
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
		Diagnosis:      fmt.Sprintf("Fallback diagnosis for %s: %s", req.IncidentType, plan.Description),
		RecoveryPlan:   plan,
		Confidence:     0.7,
		ModelVersion:   "fallback-deterministic",
		Source:         "fallback",
		FallbackReason: reason,
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

// TestConnection verifies the credential and model with an authenticated
// request. It first tries GET /models (credential check plus model list);
// providers without /models fall back to a minimal chat completion. A bare
// 200 from an unauthenticated route is never treated as success. Error
// messages never contain the API key.
func (c *Client) TestConnection(ctx context.Context) error {
	if c.config.BaseURL == "" {
		return fmt.Errorf("missing base URL: enter the provider base URL")
	}
	if c.config.Model == "" {
		return fmt.Errorf("missing model: enter a model name")
	}
	if c.config.APIKey == "" {
		return fmt.Errorf("missing API key: enter an API key, or save one first and leave the field blank to use it")
	}
	status, body, err := c.doAuthed(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return fmt.Errorf("test connection: %w", err)
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		return c.testViaChat(ctx)
	}
	if err := statusError("test connection", status, body, c.config.APIKey); err != nil {
		return err
	}
	// If the provider returned a model list, the selected model must be on it.
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &list) == nil && len(list.Data) > 0 {
		for _, m := range list.Data {
			if m.ID == c.config.Model {
				return nil
			}
		}
		return fmt.Errorf("model %q is not available from this provider (not in its model list)", c.config.Model)
	}
	return nil
}

func (c *Client) testViaChat(ctx context.Context) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"model":      c.config.Model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	status, body, err := c.doAuthed(ctx, http.MethodPost, "/chat/completions", payload)
	if err != nil {
		return fmt.Errorf("test connection: %w", err)
	}
	if err := statusError("test connection", status, body, c.config.APIKey); err != nil {
		return err
	}
	return nil
}

func (c *Client) doAuthed(ctx context.Context, method, path string, payload []byte) (int, []byte, error) {
	var rdr io.Reader
	if payload != nil {
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.config.BaseURL+path, rdr)
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request to provider failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, body, nil
}

// statusError maps a provider status to a safe, useful message. The API key
// is redacted if the provider echoes it back.
func statusError(op string, status int, body []byte, apiKey string) error {
	if status >= 200 && status < 300 {
		return nil
	}
	detail := strings.TrimSpace(string(body))
	if apiKey != "" {
		detail = strings.ReplaceAll(detail, apiKey, "[redacted]")
	}
	// Prefer the provider's structured error message, truncated.
	var parsed struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		detail = parsed.Error.Message
		if apiKey != "" {
			detail = strings.ReplaceAll(detail, apiKey, "[redacted]")
		}
	}
	if len(detail) > 200 {
		detail = detail[:200]
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%s: provider rejected the API key (HTTP %d): %s", op, status, detail)
	case status == http.StatusNotFound:
		return fmt.Errorf("%s: provider returned HTTP 404 (check the base URL, and that the model exists): %s", op, detail)
	case status == http.StatusBadRequest && strings.Contains(strings.ToLower(detail), "model"):
		return fmt.Errorf("%s: provider rejected the request, model may be unavailable (HTTP %d): %s", op, status, detail)
	default:
		return fmt.Errorf("%s: provider returned HTTP %d: %s", op, status, detail)
	}
}
