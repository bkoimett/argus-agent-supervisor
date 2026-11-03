// Package supabase provides Supabase integration for the Argus agent manager.
package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/supabase-community/supabase-go"
)

// Config holds Supabase configuration.
type Config struct {
	URL        string
	AnonKey    string
	ServiceKey string
	Enabled    bool
}

// Client wraps the Supabase client with Argus-specific operations.
type Client struct {
	client *supabase.Client
	config Config
	mu     sync.RWMutex

	// Realtime subscriptions
	sessionChannels map[string]chan *state.Session
	cmdChannels     map[string]chan *state.Command
	eventChannels   map[string]chan *state.Event
}

// NewClient creates a new Supabase client.
func NewClient(config Config) (*Client, error) {
	if !config.Enabled {
		log.Println("Supabase integration disabled")
		return &Client{
			config:          config,
			sessionChannels: make(map[string]chan *state.Session),
			cmdChannels:     make(map[string]chan *state.Command),
			eventChannels:   make(map[string]chan *state.Event),
		}, nil
	}

	if config.URL == "" || config.AnonKey == "" {
		return nil, fmt.Errorf("Supabase URL and anon key required when enabled")
	}

	client, err := supabase.NewClient(config.URL, config.AnonKey, &supabase.ClientOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Supabase client: %w", err)
	}

	c := &Client{
		client:          client,
		config:          config,
		sessionChannels: make(map[string]chan *state.Session),
		cmdChannels:     make(map[string]chan *state.Command),
		eventChannels:   make(map[string]chan *state.Event),
	}

	return c, nil
}

// IsEnabled returns whether Supabase integration is enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// SyncSession syncs a session to Supabase.
func (c *Client) SyncSession(ctx context.Context, sess *state.Session) error {
	if !c.IsEnabled() {
		return nil
	}

	data := map[string]interface{}{
		"id":            sess.ID,
		"project_dir":   sess.ProjectDir,
		"status":        sess.Status,
		"started_at":    sess.StartedAt,
		"ended_at":      sess.EndedAt,
		"pid":           sess.PID,
		"agent":         sess.Agent,
		"model":         sess.Model,
		"error_message": sess.ErrorMessage,
		"updated_at":    time.Now(),
	}

	_, _, err := c.client.From("sessions").Upsert(data, "", "", "").Execute()
	if err != nil {
		return fmt.Errorf("failed to sync session: %w", err)
	}

	// Notify subscribers
	c.mu.RLock()
	for _, ch := range c.sessionChannels {
		select {
		case ch <- sess:
		default:
		}
	}
	c.mu.RUnlock()

	return nil
}

// SyncTask syncs a task to Supabase.
func (c *Client) SyncTask(ctx context.Context, task *state.Task) error {
	if !c.IsEnabled() {
		return nil
	}

	data := map[string]interface{}{
		"id":             task.ID,
		"session_id":     task.SessionID,
		"prompt":         task.Prompt,
		"scope":          task.Scope,
		"dependencies":   task.Dependencies,
		"status":         task.Status,
		"started_at":     task.StartedAt,
		"ended_at":       task.EndedAt,
		"exit_code":      task.ExitCode,
		"error_message":  task.ErrorMessage,
		"approval_id":    task.ApprovalID,
		"recovery_count": task.RecoveryCount,
		"updated_at":     time.Now(),
	}

	_, _, err := c.client.From("tasks").Upsert(data, "", "", "").Execute()
	return err
}

// SyncIncident syncs an incident to Supabase.
func (c *Client) SyncIncident(ctx context.Context, inc *state.Incident) error {
	if !c.IsEnabled() {
		return nil
	}

	data := map[string]interface{}{
		"id":                inc.ID,
		"session_id":        inc.SessionID,
		"task_id":           inc.TaskID,
		"type":              inc.Type,
		"severity":          inc.Severity,
		"status":            inc.Status,
		"evidence":          inc.Evidence,
		"meta_diagnosis":    inc.MetaDiagnosis,
		"meta_recovery":     inc.MetaRecovery,
		"detected_at":       inc.DetectedAt,
		"diagnosed_at":      inc.DiagnosedAt,
		"proposed_at":       inc.ProposedAt,
		"approved_at":       inc.ApprovedAt,
		"recovered_at":      inc.RecoveredAt,
		"verified_at":       inc.VerifiedAt,
		"recovery_attempts": inc.RecoveryAttempts,
		"updated_at":        time.Now(),
	}

	_, _, err := c.client.From("incidents").Upsert(data, "", "", "").Execute()
	return err
}

// SyncApproval syncs an approval to Supabase.
func (c *Client) SyncApproval(ctx context.Context, appr *state.Approval) error {
	if !c.IsEnabled() {
		return nil
	}

	data := map[string]interface{}{
		"id":           appr.ID,
		"session_id":   appr.SessionID,
		"task_id":      appr.TaskID,
		"incident_id":  appr.IncidentID,
		"type":         appr.Type,
		"status":       appr.Status,
		"title":        appr.Title,
		"description":  appr.Description,
		"payload":      appr.Payload,
		"scope":        appr.Scope,
		"requested_at": appr.RequestedAt,
		"responded_at": appr.RespondedAt,
		"decision":     appr.Decision,
		"decided_by":   appr.DecidedBy,
		"expires_at":   appr.ExpiresAt,
		"updated_at":   time.Now(),
	}

	_, _, err := c.client.From("approvals").Upsert(data, "", "", "").Execute()
	return err
}

// SyncRecoveryAction syncs a recovery action to Supabase.
func (c *Client) SyncRecoveryAction(ctx context.Context, ra *state.RecoveryAction) error {
	if !c.IsEnabled() {
		return nil
	}

	data := map[string]interface{}{
		"id":          ra.ID,
		"incident_id": ra.IncidentID,
		"type":        ra.Type,
		"description": ra.Description,
		"command":     ra.Command,
		"args":        ra.Args,
		"status":      ra.Status,
		"result":      ra.Result,
		"proposed_at": ra.ProposedAt,
		"approved_at": ra.ApprovedAt,
		"executed_at": ra.ExecutedAt,
		"updated_at":  time.Now(),
	}

	_, _, err := c.client.From("recovery_actions").Upsert(data, "", "", "").Execute()
	return err
}

// SyncEvent syncs an event to Supabase.
func (c *Client) SyncEvent(ctx context.Context, evt *state.Event) error {
	if !c.IsEnabled() {
		return nil
	}

	data := map[string]interface{}{
		"session_id": evt.SessionID,
		"type":       evt.Type,
		"payload":    evt.Payload,
		"created_at": evt.Timestamp,
	}

	_, _, err := c.client.From("events").Insert(data, false, "", "", "").Execute()
	return err
}

// SyncCommand inserts or updates a command in Supabase.
func (c *Client) SyncCommand(ctx context.Context, cmd *state.Command) error {
	if !c.IsEnabled() {
		return nil
	}

	data := map[string]interface{}{
		"id":         cmd.ID,
		"session_id": cmd.SessionID,
		"type":       cmd.Type,
		"payload":    cmd.Payload,
		"status":     cmd.Status,
		"created_at": cmd.CreatedAt,
		"updated_at": cmd.UpdatedAt,
	}

	_, _, err := c.client.From("commands").Upsert(data, "", "", "").Execute()
	return err
}

// ListPendingCommands returns all commands with status PENDING.
func (c *Client) ListPendingCommands(ctx context.Context) ([]*state.Command, error) {
	if !c.IsEnabled() {
		return nil, nil
	}

	var result []map[string]interface{}
	_, err := c.client.From("commands").
		Select("*", "", false).
		Eq("status", "PENDING").
		ExecuteTo(&result)
	if err != nil {
		return nil, fmt.Errorf("list pending commands: %w", err)
	}

	cmds := make([]*state.Command, 0, len(result))
	for _, row := range result {
		cmd := &state.Command{
			ID:        stringValue(row["id"]),
			SessionID: stringValue(row["session_id"]),
			Type:      stringValue(row["type"]),
			Status:    stringValue(row["status"]),
		}
		if payload, ok := row["payload"]; ok {
			payloadBytes, _ := json.Marshal(payload)
			cmd.Payload = json.RawMessage(payloadBytes)
		}
		if createdAt, ok := row["created_at"]; ok {
			if t, err := time.Parse(time.RFC3339, stringValue(createdAt)); err == nil {
				cmd.CreatedAt = t
			}
		}
		if updatedAt, ok := row["updated_at"]; ok {
			if t, err := time.Parse(time.RFC3339, stringValue(updatedAt)); err == nil {
				cmd.UpdatedAt = t
			}
		}
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}

// stringValue converts an interface{} from Supabase to a string.
func stringValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case json.RawMessage:
		return string(val)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// GeneratePairingCode creates a new pairing code for device pairing.
func (c *Client) GeneratePairingCode(ctx context.Context, userID string) (string, error) {
	if !c.IsEnabled() {
		return "", fmt.Errorf("Supabase not enabled")
	}

	code := generateCode(6)
	expiresAt := time.Now().Add(5 * time.Minute)

	data := map[string]interface{}{
		"code":       code,
		"user_id":    userID,
		"expires_at": expiresAt,
		"created_at": time.Now(),
	}

	_, _, err := c.client.From("pairing_codes").Insert(data, false, "", "", "").Execute()
	if err != nil {
		return "", fmt.Errorf("failed to create pairing code: %w", err)
	}

	return code, nil
}

// ValidatePairingCode validates a pairing code and returns the user ID.
func (c *Client) ValidatePairingCode(ctx context.Context, code string) (string, error) {
	if !c.IsEnabled() {
		return "", fmt.Errorf("Supabase not enabled")
	}

	var result []map[string]interface{}
	_, err := c.client.From("pairing_codes").
		Select("user_id, expires_at", "", false).
		Eq("code", code).
		Gt("expires_at", time.Now().Format(time.RFC3339)).
		ExecuteTo(&result)
	if err != nil || len(result) == 0 {
		return "", fmt.Errorf("invalid or expired pairing code")
	}

	userID := result[0]["user_id"].(string)

	// Delete the used code
	_, _, _ = c.client.From("pairing_codes").Delete("", "").Eq("code", code).Execute()

	return userID, nil
}

// SubscribeSessions subscribes to session changes for a user.
func (c *Client) SubscribeSessions(userID string) (<-chan *state.Session, error) {
	if !c.IsEnabled() {
		return nil, fmt.Errorf("Supabase not enabled")
	}

	ch := make(chan *state.Session, 10)
	c.mu.Lock()
	c.sessionChannels[userID] = ch
	c.mu.Unlock()

	return ch, nil
}

// SubscribeCommands subscribes to command changes for a session.
func (c *Client) SubscribeCommands(sessionID string) (<-chan *state.Command, error) {
	if !c.IsEnabled() {
		return nil, fmt.Errorf("Supabase not enabled")
	}

	ch := make(chan *state.Command, 10)
	c.mu.Lock()
	c.cmdChannels[sessionID] = ch
	c.mu.Unlock()

	return ch, nil
}

// Close cleans up resources.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, ch := range c.sessionChannels {
		close(ch)
	}
	for _, ch := range c.cmdChannels {
		close(ch)
	}
	for _, ch := range c.eventChannels {
		close(ch)
	}
	c.sessionChannels = make(map[string]chan *state.Session)
	c.cmdChannels = make(map[string]chan *state.Command)
	c.eventChannels = make(map[string]chan *state.Event)
}

func generateCode(n int) string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}
