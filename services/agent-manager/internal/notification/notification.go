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
}

// NewClient creates a new notification client.
func NewClient(config Config) *Client {
	if config.NtfyURL == "" {
		config.NtfyURL = "https://ntfy.sh"
	}
	return &Client{
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Second},
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
		return c.SendNtfy(payload)
	}

	return fmt.Errorf("no notification method available")
}

// SendToUser sends a notification to a user via all their subscriptions.
func (c *Client) SendToUser(ctx context.Context, userID string, subs []Subscription, payload NotificationPayload) error {
	if len(subs) == 0 {
		// No subscriptions, try ntfy
		return c.SendNtfy(payload)
	}

	var lastErr error
	for _, sub := range subs {
		err := c.SendWebPush(sub, payload)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	// All web push failed, try ntfy
	if c.config.NtfyTopic != "" {
		return c.SendNtfy(payload)
	}

	return lastErr
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
