// Package notification handles push notifications via Web Push (VAPID) and ntfy.sh fallback.
package notification

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

// Config holds notification configuration.
type Config struct {
	// VAPID keys for web push
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string // e.g., "mailto:admin@example.com"

	// ntfy.sh fallback
	NtfyTopic string
	NtfyURL   string // default: https://ntfy.sh

	// NtfyAllowBroadcast permits sending to the ntfy topic without a specific
	// subscription. When false (default), Send() with no subscription returns
	// an error instead of broadcasting to the ntfy topic. This prevents
	// unintended broadcast of session data to arbitrary subscribers.
	NtfyAllowBroadcast bool

	Enabled bool
}

// Subscription represents a push subscription from a client.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// NotificationPayload is the payload sent to clients.
type NotificationPayload struct {
	Title              string                 `json:"title"`
	Body               string                 `json:"body"`
	Icon               string                 `json:"icon,omitempty"`
	Badge              string                 `json:"badge,omitempty"`
	Tag                string                 `json:"tag,omitempty"`
	Data               map[string]interface{} `json:"data,omitempty"`
	RequireInteraction bool                   `json:"requireInteraction,omitempty"`
	// DedupKey is an optional stable key for idempotent delivery. If empty,
	// one is generated from session ID + notification type + entity ID.
	DedupKey string `json:"-"`
}

// NotificationType represents the type of notification.
type NotificationType string

const (
	NotificationTypeNeedsInput   NotificationType = "needs_input"
	NotificationTypePermission   NotificationType = "permission"
	NotificationTypeCompleted    NotificationType = "completed"
	NotificationTypeCrashed      NotificationType = "crashed"
	NotificationTypeError        NotificationType = "error"
	NotificationTypeQuotaWarning NotificationType = "quota_warning"
	NotificationTypeDisconnected NotificationType = "disconnected"
)

// Client handles sending notifications.
type Client struct {
	config     Config
	httpClient *http.Client
	dedupMu    sync.Mutex
	dedupSent  map[string]bool // dedup key → already sent
}

// NewClient creates a new notification client.
func NewClient(config Config) *Client {
	if config.NtfyURL == "" {
		config.NtfyURL = "https://ntfy.sh"
	}
	return &Client{
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		dedupSent:  make(map[string]bool),
	}
}

// IsEnabled returns whether notifications are enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// GenerateVAPIDKeys generates a new VAPID key pair.
func GenerateVAPIDKeys() (publicKey, privateKey string, err error) {
	// Generate 32 bytes for private key
	privateKeyBytes := make([]byte, 32)
	if _, err := rand.Read(privateKeyBytes); err != nil {
		return "", "", err
	}

	// For VAPID, we use the raw bytes as the private key
	// The public key is derived from the private key using elliptic curve crypto
	// For simplicity, we'll use a deterministic approach here
	// In production, use proper ECDSA P-256 key generation

	privateKey = base64.RawURLEncoding.EncodeToString(privateKeyBytes)
	// Note: In production, derive public key from private key using P-256 curve
	// For now, return a placeholder - real implementation would use crypto/ecdsa
	publicKey = "PLACEHOLDER_PUBLIC_KEY"

	return publicKey, privateKey, nil
}

// SendWebPush sends a web push notification to a subscription.
func (c *Client) SendWebPush(sub Subscription, payload NotificationPayload) error {
	if !c.IsEnabled() {
		return nil
	}

	if c.config.VAPIDPrivateKey == "" || c.config.VAPIDPublicKey == "" {
		return fmt.Errorf("VAPID keys not configured")
	}

	// Marshal payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Convert subscription to webpush-go format
	wpSub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.Keys.P256dh,
			Auth:   sub.Keys.Auth,
		},
	}

	// Send notification
	resp, err := webpush.SendNotification(payloadBytes, wpSub, &webpush.Options{
		VAPIDPublicKey:  c.config.VAPIDPublicKey,
		VAPIDPrivateKey: c.config.VAPIDPrivateKey,
		Subscriber:      c.config.VAPIDSubject,
		TTL:             300,
	})
	if err != nil {
		return fmt.Errorf("send web push: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("web push failed with status: %d", resp.StatusCode)
	}

	return nil
}

// SendNtfy sends a notification via ntfy.sh fallback.
func (c *Client) SendNtfy(payload NotificationPayload) error {
	if !c.IsEnabled() {
		return nil
	}

	if c.config.NtfyTopic == "" {
		return fmt.Errorf("ntfy topic not configured")
	}

	// Build ntfy message
	msg := fmt.Sprintf("%s\n\n%s", payload.Title, payload.Body)
	if len(payload.Data) > 0 {
		dataBytes, _ := json.Marshal(payload.Data)
		msg += "\n\nData: " + string(dataBytes)
	}

	req, err := http.NewRequest("POST", c.config.NtfyURL+"/"+c.config.NtfyTopic, strings.NewReader(msg))
	if err != nil {
		return fmt.Errorf("create ntfy request: %w", err)
	}

	req.Header.Set("Title", payload.Title)
	req.Header.Set("Priority", "high")
	req.Header.Set("Tags", "warning")

	if payload.Icon != "" {
		req.Header.Set("Icon", payload.Icon)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send ntfy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Send sends a notification via web push (with ntfy fallback).
func (c *Client) Send(sub *Subscription, payload NotificationPayload) error {
	if !c.IsEnabled() {
		return nil
	}

	// Dedup check: skip if this notification was already sent
	dedupKey := payload.DedupKey
	if dedupKey == "" {
		dedupKey = generateDedupKey(payload)
	}
	c.dedupMu.Lock()
	if c.dedupSent[dedupKey] {
		return nil // already sent
	}
	c.dedupSent[dedupKey] = true
	c.dedupMu.Unlock()

	// Try web push first if subscription exists
	if sub != nil && c.config.VAPIDPrivateKey != "" && c.config.VAPIDPublicKey != "" {
		err := c.SendWebPush(*sub, payload)
		if err == nil {
			return nil
		}
		log.Printf("Web push failed, falling back to ntfy: %v", err)
	}

	// Fallback to ntfy
	if c.config.NtfyTopic != "" {
		if sub == nil && !c.config.NtfyAllowBroadcast {
			return fmt.Errorf("no subscription and ntfy broadcast disabled")
		}
		// Strip sensitive fields when broadcasting without a specific target
		if sub == nil {
			payload = stripSensitiveFields(payload)
		}
		return c.SendNtfy(payload)
	}

	return fmt.Errorf("no notification method available")
}

// stripSensitiveFields returns a copy of the payload with potentially
// sensitive fields removed, for safe broadcast over ntfy without a
// specific subscription target.
func stripSensitiveFields(payload NotificationPayload) NotificationPayload {
	stripped := payload
	stripped.Data = make(map[string]interface{})
	for k, v := range payload.Data {
		if k == "session_id" || k == "project" {
			continue // strip session identifiers and project paths
		}
		stripped.Data[k] = v
	}
	return stripped
}

// SendToUser sends a notification to a user via all their subscriptions.
func (c *Client) SendToUser(ctx context.Context, userID string, subs []Subscription, payload NotificationPayload) error {
	if len(subs) == 0 {
		// No subscriptions, try ntfy broadcast (may be disabled)
		return c.Send(nil, payload)
	}

	var lastErr error
	for _, sub := range subs {
		err := c.Send(&sub, payload)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	// All web push failed, try ntfy
	if c.config.NtfyTopic != "" {
		return c.Send(nil, payload)
	}

	return lastErr
}

// generateDedupKey builds a stable dedup key from the notification payload.
// Format: sessionID:notifType[:entityID] where entityID is incident_id or
// approval_id when available. This ensures the same logical notification
// (same session, same type, same target entity) is only sent once.
func generateDedupKey(payload NotificationPayload) string {
	sessionID, _ := payload.Data["session_id"].(string)
	notifType, _ := payload.Data["type"].(string)
	key := sessionID + ":" + notifType
	if entityID, ok := payload.Data["incident_id"].(string); ok && entityID != "" {
		key += ":" + entityID
	} else if entityID, ok := payload.Data["approval_id"].(string); ok && entityID != "" {
		key += ":" + entityID
	}
	return key
}

// CreateNotificationPayload creates a notification payload from an event.
func CreateNotificationPayload(notifType NotificationType, session *state.Session, details map[string]interface{}) NotificationPayload {
	base := NotificationPayload{
		Tag: string(notifType),
		Data: map[string]interface{}{
			"type":       string(notifType),
			"session_id": session.ID,
			"project":    session.ProjectDir,
		},
	}

	switch notifType {
	case NotificationTypeNeedsInput:
		base.Title = "Agent needs input"
		base.Body = fmt.Sprintf("Session in %s is waiting for your input", session.ProjectDir)
		base.RequireInteraction = true
	case NotificationTypePermission:
		base.Title = "Permission required"
		base.Body = fmt.Sprintf("Session in %s needs permission to continue", session.ProjectDir)
		base.RequireInteraction = true
	case NotificationTypeCompleted:
		base.Title = "Task completed"
		base.Body = fmt.Sprintf("Session in %s has completed", session.ProjectDir)
	case NotificationTypeCrashed:
		base.Title = "Agent crashed"
		base.Body = fmt.Sprintf("Session in %s has crashed", session.ProjectDir)
		base.RequireInteraction = true
	case NotificationTypeError:
		base.Title = "Error detected"
		base.Body = fmt.Sprintf("Session in %s encountered an error", session.ProjectDir)
	case NotificationTypeQuotaWarning:
		base.Title = "Quota warning"
		base.Body = "Your API quota is running low"
	case NotificationTypeDisconnected:
		base.Title = "Disconnected"
		base.Body = fmt.Sprintf("Lost connection to session in %s", session.ProjectDir)
	}

	// Merge additional details
	for k, v := range details {
		base.Data[k] = v
	}

	// Set dedup key for idempotent delivery
	base.DedupKey = generateDedupKey(base)

	return base
}

// Preferences represents user notification preferences.
type Preferences struct {
	UserID       string    `json:"user_id"`
	NeedsInput   bool      `json:"needs_input"`
	Permission   bool      `json:"permission"`
	Completed    bool      `json:"completed"`
	Crashed      bool      `json:"crashed"`
	Error        bool      `json:"error"`
	QuotaWarning bool      `json:"quota_warning"`
	Disconnected bool      `json:"disconnected"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ShouldNotify checks if a notification should be sent based on preferences.
func (p *Preferences) ShouldNotify(notifType NotificationType) bool {
	switch notifType {
	case NotificationTypeNeedsInput:
		return p.NeedsInput
	case NotificationTypePermission:
		return p.Permission
	case NotificationTypeCompleted:
		return p.Completed
	case NotificationTypeCrashed:
		return p.Crashed
	case NotificationTypeError:
		return p.Error
	case NotificationTypeQuotaWarning:
		return p.QuotaWarning
	case NotificationTypeDisconnected:
		return p.Disconnected
	}
	return false
}

// DefaultPreferences returns default notification preferences.
func DefaultPreferences(userID string) *Preferences {
	return &Preferences{
		UserID:       userID,
		NeedsInput:   true,
		Permission:   true,
		Completed:    true,
		Crashed:      true,
		Error:        true,
		QuotaWarning: true,
		Disconnected: true,
		UpdatedAt:    time.Now(),
	}
}
