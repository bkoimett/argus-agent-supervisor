package notification

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
)

func TestDedupSameKeySentOnce(t *testing.T) {
	var callCount int
	ntfyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ntfyServer.Close()

	c := &Client{
		config: Config{
			Enabled:            true,
			NtfyTopic:          "test-topic",
			NtfyURL:            ntfyServer.URL,
			NtfyAllowBroadcast: true,
		},
		httpClient: ntfyServer.Client(),
		dedupSent:  make(map[string]bool),
	}

	payload := NotificationPayload{
		Title:    "Test",
		Body:     "Test body",
		DedupKey: "ses_1:completed",
		Data: map[string]interface{}{
			"session_id": "ses_1",
			"type":       "completed",
		},
	}

	if err := c.Send(nil, payload); err != nil {
		t.Fatalf("first send failed: %v", err)
	}

	if err := c.Send(nil, payload); err != nil {
		t.Fatalf("second send failed: %v", err)
	}

	if callCount != 1 {
		t.Fatalf("expected 1 HTTP call, got %d", callCount)
	}
}

func TestDedupDifferentKeysSentSeparately(t *testing.T) {
	var callCount int
	ntfyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ntfyServer.Close()

	c := &Client{
		config: Config{
			Enabled:            true,
			NtfyTopic:          "test-topic",
			NtfyURL:            ntfyServer.URL,
			NtfyAllowBroadcast: true,
		},
		httpClient: ntfyServer.Client(),
		dedupSent:  make(map[string]bool),
	}

	payload1 := NotificationPayload{
		Title:    "Test 1",
		Body:     "Body 1",
		DedupKey: "ses_1:completed",
		Data: map[string]interface{}{
			"session_id": "ses_1",
			"type":       "completed",
		},
	}
	payload2 := NotificationPayload{
		Title:    "Test 2",
		Body:     "Body 2",
		DedupKey: "ses_1:crashed",
		Data: map[string]interface{}{
			"session_id": "ses_1",
			"type":       "crashed",
		},
	}

	if err := c.Send(nil, payload1); err != nil {
		t.Fatalf("first send failed: %v", err)
	}
	if err := c.Send(nil, payload2); err != nil {
		t.Fatalf("second send failed: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestDisabledClientNoSend(t *testing.T) {
	var callCount int
	ntfyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ntfyServer.Close()

	c := &Client{
		config: Config{
			Enabled:   false,
			NtfyTopic: "test-topic",
			NtfyURL:   ntfyServer.URL,
		},
		httpClient: ntfyServer.Client(),
		dedupSent:  make(map[string]bool),
	}

	payload := NotificationPayload{
		Title:    "Test",
		Body:     "Body",
		DedupKey: "ses_1:completed",
	}

	if err := c.Send(nil, payload); err != nil {
		t.Fatalf("send should not error when disabled: %v", err)
	}

	if callCount != 0 {
		t.Fatalf("expected 0 HTTP calls when disabled, got %d", callCount)
	}
}

func TestSendNtfyMissingTopic(t *testing.T) {
	c := &Client{
		config: Config{
			Enabled:   true,
			NtfyTopic: "",
			NtfyURL:   "https://ntfy.sh",
		},
		httpClient: &http.Client{},
		dedupSent:  make(map[string]bool),
	}

	payload := NotificationPayload{
		Title:    "Test",
		Body:     "Body",
		DedupKey: "ses_1:completed",
	}

	if err := c.Send(nil, payload); err == nil {
		t.Fatal("expected error when ntfy topic not configured")
	}
}

func TestGenerateDedupKey(t *testing.T) {
	payload := NotificationPayload{
		Data: map[string]interface{}{
			"session_id":  "ses_1",
			"type":        "error",
			"incident_id": "inc_1",
		},
	}
	key := generateDedupKey(payload)
	expected := "ses_1:error:inc_1"
	if key != expected {
		t.Fatalf("dedup key = %q, want %q", key, expected)
	}

	payload2 := NotificationPayload{
		Data: map[string]interface{}{
			"session_id":  "ses_2",
			"type":        "permission",
			"approval_id": "appr_1",
		},
	}
	key2 := generateDedupKey(payload2)
	expected2 := "ses_2:permission:appr_1"
	if key2 != expected2 {
		t.Fatalf("dedup key = %q, want %q", key2, expected2)
	}

	payload3 := NotificationPayload{
		Data: map[string]interface{}{
			"session_id": "ses_3",
			"type":       "completed",
		},
	}
	key3 := generateDedupKey(payload3)
	expected3 := "ses_3:completed"
	if key3 != expected3 {
		t.Fatalf("dedup key = %q, want %q", key3, expected3)
	}
}

func TestCreateNotificationPayloadSetsDedupKey(t *testing.T) {
	session := &state.Session{
		ID:         "ses_1",
		ProjectDir: "/tmp/demo",
		Agent:      "opencode",
	}

	payload := CreateNotificationPayload(NotificationTypeCompleted, session, map[string]interface{}{
		"exit_code": 0,
	})

	expectedKey := "ses_1:completed"
	if payload.DedupKey != expectedKey {
		t.Fatalf("DedupKey = %q, want %q", payload.DedupKey, expectedKey)
	}

	payload2 := CreateNotificationPayload(NotificationTypeError, session, map[string]interface{}{
		"incident_id":   "inc_1",
		"incident_type": "test_failure",
		"severity":      "high",
	})
	expectedKey2 := "ses_1:error:inc_1"
	if payload2.DedupKey != expectedKey2 {
		t.Fatalf("DedupKey = %q, want %q", payload2.DedupKey, expectedKey2)
	}
}

func TestSendNtfyServerError(t *testing.T) {
	ntfyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ntfyServer.Close()

	c := &Client{
		config: Config{
			Enabled:            true,
			NtfyTopic:          "test-topic",
			NtfyURL:            ntfyServer.URL,
			NtfyAllowBroadcast: true,
		},
		httpClient: ntfyServer.Client(),
		dedupSent:  make(map[string]bool),
	}

	payload := NotificationPayload{
		Title:    "Test",
		Body:     "Body",
		DedupKey: "ses_1:completed",
	}

	if err := c.Send(nil, payload); err == nil {
		t.Fatal("expected error when ntfy returns 500")
	}
}
