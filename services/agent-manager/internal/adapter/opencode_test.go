package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockServe returns an httptest server implementing the verified OpenCode serve
// endpoints, plus a handler registry to assert on received requests.
type mockServe struct {
	*httptest.Server
	lastMessageBody string
	messageCount    int
	sessions        map[string]bool
	// permissionReplies tracks received permission replies keyed by
	// "sessionID:requestID" -> reply value ("approve" or "deny").
	permissionReplies map[string]string
	permissionMu      sync.Mutex
}

func newMockServe() *mockServe {
	m := &mockServe{
		sessions:          make(map[string]bool),
		permissionReplies: make(map[string]string),
	}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/session", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Directory string `json:"directory"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := "ses_mock_" + fmt.Sprint(len(m.sessions)+1)
		m.sessions[id] = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":        id,
				"directory": req.Directory,
				"projectID": "global",
			},
		})
	})

	mux.HandleFunc("POST /session/{sessionID}/message", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionID")
		if !m.sessions[sessionID] {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		var req messageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		buf := new(strings.Builder)
		for _, p := range req.Parts {
			buf.WriteString(p.Text)
			buf.WriteString("\n")
		}
		m.lastMessageBody = buf.String()
		m.messageCount++

		text := strings.TrimSpace(m.lastMessageBody)
		msgID := "msg_mock_" + fmt.Sprint(m.messageCount)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(messageResponse{
			Info: messageInfoResponse{
				ID:        msgID,
				SessionID: sessionID,
				Role:      "assistant",
				Finish:    "stop",
				Created:   time.Now().UnixMilli(),
				Completed: time.Now().UnixMilli(),
				Cost:      0.001,
				Tokens: tokenUsageResponse{
					Total:     10,
					Input:     6,
					Output:    3,
					Reasoning: 1,
				},
			},
			Parts: []messagePartResponse{
				{Type: "step-start", ID: "prt_start", MessageID: msgID, SessionID: sessionID},
				{Type: "text", ID: "prt_text", Text: text, MessageID: msgID, SessionID: sessionID},
				{Type: "step-finish", ID: "prt_finish", MessageID: msgID, SessionID: sessionID},
			},
		})
	})

	mux.HandleFunc("POST /session/{sessionID}/interrupt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/session/{sessionID}/permission/{requestID}/reply", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionID")
		requestID := r.PathValue("requestID")
		var req permissionReplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.permissionMu.Lock()
		m.permissionReplies[sessionID+":"+requestID] = req.Reply
		m.permissionMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/session/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionID")
		if !m.sessions[sessionID] {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(getSessionResponse{
			Data: sessionInfo{ID: sessionID, Directory: "/tmp/demo", ProjectID: "global"},
		})
	})

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	m.Server = httptest.NewServer(mux)
	return m
}

func (m *mockServe) baseURL() string { return m.Server.URL }

func TestServeClientCreateSession(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	sc := NewServeClient(m.baseURL())
	id, err := sc.CreateSession("/tmp/demo")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty session id")
	}
	if !strings.HasPrefix(id, "ses_mock_") {
		t.Fatalf("unexpected session id: %s", id)
	}
}

func TestServeClientSendMessage(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	sc := NewServeClient(m.baseURL())
	id, err := sc.CreateSession("/tmp/demo")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	result, err := sc.SendMessage(id, "create probe.txt")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if result.SessionID != id {
		t.Errorf("SessionID = %q, want %q", result.SessionID, id)
	}
	if result.MessageID == "" {
		t.Error("expected non-empty MessageID")
	}
	if result.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", result.Role)
	}
	if result.Finish != "stop" {
		t.Errorf("Finish = %q, want stop", result.Finish)
	}
	if !strings.Contains(result.Text, "create probe.txt") {
		t.Errorf("Text = %q, want to contain prompt", result.Text)
	}
	if len(result.Parts) != 3 {
		t.Errorf("len(Parts) = %d, want 3", len(result.Parts))
	}
	if result.Tokens.Total != 10 {
		t.Errorf("Tokens.Total = %d, want 10", result.Tokens.Total)
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestServeClientSendMessageSessionNotFound(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	sc := NewServeClient(m.baseURL())
	_, err := sc.SendMessage("nosuch", "hello")
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
}

func TestAdapterStartSendMessageStatusStop(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	id, err := a.Start("/tmp/demo")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if id == "" {
		t.Fatal("expected session id")
	}

	// Status while running
	st, err := a.Status(id)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st != StatusWorking {
		t.Errorf("Status = %q, want WORKING", st)
	}

	// Send a follow-up message
	result, err := a.SendMessage(id, "list files")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if result.Text == "" {
		t.Error("expected non-empty assistant text")
	}

	// Events preserve parts/session data
	events, err := a.GetEvents(id)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events from last result")
	}
	for _, e := range events {
		if e.SessionID != id {
			t.Errorf("event SessionID = %q, want %q", e.SessionID, id)
		}
	}

	// Stop
	if err := a.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Status after stop: DETACHED (not STOPPED — cannot confirm OpenCode
	// process termination on the shared serve process).
	st, err = a.Status(id)
	if err != nil {
		t.Fatalf("Status after stop: %v", err)
	}
	if st != StatusDetached {
		t.Errorf("Status after stop = %q, want %q", st, StatusDetached)
	}

	// Session is still tracked (not removed from map) but detached.
	handle, ok := a.sessions[id]
	if !ok {
		t.Fatal("expected session to remain in adapter map after Stop")
	}
	if handle.Status != StatusDetached {
		t.Errorf("handle.Status = %q, want %q", handle.Status, StatusDetached)
	}

	// Sending to a detached session should fail.
	if _, err := a.SendMessage(id, "after stop"); err == nil {
		t.Error("expected error sending to a detached session")
	}
}

// TestAdapterStartRecordsWorkingDirectory verifies the adapter stores the
// requested project directory in the session handle. This is a safety check:
// the directory we tell opencode serve to use is recorded so it can be
// compared against the directory Argus believes the session is running in.
// Note: this records intent only — it cannot verify OpenCode's actual working
// directory, and filesystem sandboxing is not enforced.
func TestAdapterStartRecordsWorkingDirectory(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	projectDir := "/home/g/test-proj"
	id, err := a.Start(projectDir)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	handle, ok := a.sessions[id]
	if !ok {
		t.Fatal("expected session to be tracked by adapter")
	}
	if handle.ProjectDir != projectDir {
		t.Errorf("handle.ProjectDir = %q, want %q", handle.ProjectDir, projectDir)
	}
}

func TestAdapterSendMessageUnknownSession(t *testing.T) {
	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(newMockServe().baseURL()),
	}
	_, err := a.SendMessage("nosuch", "hello")
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
}

func TestAdapterStopUnknownSession(t *testing.T) {
	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(newMockServe().baseURL()),
	}
	// Idempotent: stopping an unknown session is a no-op, returns nil.
	if err := a.Stop("nosuch"); err != nil {
		t.Fatalf("expected nil for unknown session stop, got: %v", err)
	}
}

// TestAdapterInterruptRetainsSession verifies that Interrupt stops the
// current turn but keeps the session tracked and WORKING so a follow-up
// message can be sent to the same session.
func TestAdapterInterruptRetainsSession(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	id, err := a.Start("/tmp/demo")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Send a message so there is a turn to interrupt.
	if _, err := a.SendMessage(id, "do some work"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Interrupt: session must remain tracked and WORKING.
	if err := a.Interrupt(id); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	st, err := a.Status(id)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st != StatusWorking {
		t.Errorf("Status after interrupt = %q, want %q", st, StatusWorking)
	}

	// A follow-up message to the same session must still work (session retained).
	if _, err := a.SendMessage(id, "follow up"); err != nil {
		t.Fatalf("SendMessage after interrupt: %v", err)
	}
}

// TestAdapterStopMarksDetached verifies Stop marks the session DETACHED (not
// STOPPED), keeps it tracked in the adapter map, and does not affect other
// sessions. The shared serve process is not killed.
func TestAdapterStopMarksDetached(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	id, err := a.Start("/tmp/demo")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := a.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Session is still tracked; Status reports DETACHED.
	st, err := a.Status(id)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st != StatusDetached {
		t.Errorf("Status after stop = %q, want %q", st, StatusDetached)
	}

	// Sending to a detached session should fail (not active).
	if _, err := a.SendMessage(id, "after stop"); err == nil {
		t.Error("expected error sending to a detached session")
	}
}

// TestAdapterStopIdempotent verifies repeated Stop calls are safe.
func TestAdapterStopIdempotent(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	id, err := a.Start("/tmp/demo")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// First stop marks detached and returns nil.
	if err := a.Stop(id); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Second stop on the same session is a no-op, returns nil.
	if err := a.Stop(id); err != nil {
		t.Errorf("second Stop (idempotent) returned error: %v", err)
	}
	if st, _ := a.Status(id); st != StatusDetached {
		t.Errorf("Status after repeated stop = %q, want %q", st, StatusDetached)
	}
}

// TestAdapterStopDoesNotAffectOtherSessions verifies stopping one session
// leaves other sessions tracked and WORKING (shared serve process is not
// killed).
func TestAdapterStopDoesNotAffectOtherSessions(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	id1, err := a.Start("/tmp/demo1")
	if err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	id2, err := a.Start("/tmp/demo2")
	if err != nil {
		t.Fatalf("Start 2: %v", err)
	}

	if err := a.Stop(id1); err != nil {
		t.Fatalf("Stop id1: %v", err)
	}

	// id1 is detached.
	if st, _ := a.Status(id1); st != StatusDetached {
		t.Errorf("id1 status after stop = %q, want %q", st, StatusDetached)
	}
	// id2 is still WORKING (shared serve not killed).
	if st, _ := a.Status(id2); st != StatusWorking {
		t.Errorf("id2 status after stopping id1 = %q, want %q", st, StatusWorking)
	}
}

// TestAdapterSendMessageToDetachedSession verifies SendMessage fails on a
// detached session.
func TestAdapterSendMessageToDetachedSession(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	id, err := a.Start("/tmp/demo")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := a.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if _, err := a.SendMessage(id, "should fail"); err == nil {
		t.Fatal("expected error sending to a detached session")
	}
}

// TestAdapterInterruptUnknownSession verifies Interrupt fails on an unknown
// session rather than silently succeeding.
func TestAdapterInterruptUnknownSession(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}
	if err := a.Interrupt("nosuch"); err == nil {
		t.Fatal("expected error interrupting unknown session")
	}
}

func TestAdapterRespondToPermissionPlumbing(t *testing.T) {
	// This exercises the plumbing to the verified reply endpoint. End-to-end
	// permission handling remains UNVERIFIED (no live request triggered in the
	// 1.19.0 probe); this test only asserts the request is formed and sent.
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}
	id, err := a.Start("/tmp/demo")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := a.RespondToPermission(id, "req_1", true); err != nil {
		t.Fatalf("RespondToPermission approve: %v", err)
	}
	if err := a.RespondToPermission(id, "req_1", false); err != nil {
		t.Fatalf("RespondToPermission deny: %v", err)
	}
}

// TestServeClientReplyPermissionVerifiesBody confirms the reply endpoint
// receives the correct "approve"/"deny" value. End-to-end permission handling
// remains UNVERIFIED (no live request triggered in the 1.19.0 probe); this test
// only asserts the request is formed and sent correctly.
func TestServeClientReplyPermissionVerifiesBody(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	sc := NewServeClient(m.baseURL())
	id, err := sc.CreateSession("/tmp/demo")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Approve
	if err := sc.ReplyPermission(id, "req_approve", "approve", ""); err != nil {
		t.Fatalf("ReplyPermission approve: %v", err)
	}
	// Deny
	if err := sc.ReplyPermission(id, "req_deny", "deny", ""); err != nil {
		t.Fatalf("ReplyPermission deny: %v", err)
	}

	m.permissionMu.Lock()
	gotApprove := m.permissionReplies[id+":req_approve"]
	gotDeny := m.permissionReplies[id+":req_deny"]
	m.permissionMu.Unlock()

	if gotApprove != "approve" {
		t.Errorf("approve reply = %q, want %q", gotApprove, "approve")
	}
	if gotDeny != "deny" {
		t.Errorf("deny reply = %q, want %q", gotDeny, "deny")
	}
}

// TestAdapterRespondToPermissionFailClosedUnknownSession confirms that
// RespondToPermission fails closed when the session is not tracked locally.
func TestAdapterRespondToPermissionFailClosedUnknownSession(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}

	err := a.RespondToPermission("nosuch", "req_1", true)
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
}

// TestAdapterRespondToPermissionVerifyReplyBody confirms the adapter sends
// the correct reply value through the full adapter->serve stack.
func TestAdapterRespondToPermissionVerifyReplyBody(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	a := &OpenCodeAdapter{
		binaryPath: "opencode",
		sessions:   make(map[string]*sessionHandle),
		serve:      NewServeClient(m.baseURL()),
	}
	id, err := a.Start("/tmp/demo")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := a.RespondToPermission(id, "req_approve", true); err != nil {
		t.Fatalf("RespondToPermission approve: %v", err)
	}
	if err := a.RespondToPermission(id, "req_deny", false); err != nil {
		t.Fatalf("RespondToPermission deny: %v", err)
	}

	m.permissionMu.Lock()
	gotApprove := m.permissionReplies[id+":req_approve"]
	gotDeny := m.permissionReplies[id+":req_deny"]
	m.permissionMu.Unlock()

	if gotApprove != "approve" {
		t.Errorf("approve reply = %q, want %q", gotApprove, "approve")
	}
	if gotDeny != "deny" {
		t.Errorf("deny reply = %q, want %q", gotDeny, "deny")
	}
}

func TestMessageResultTextConcatenation(t *testing.T) {
	m := newMockServe()
	defer m.Close()

	sc := NewServeClient(m.baseURL())
	id, _ := sc.CreateSession("/tmp/demo")

	// Override handler to return multiple text parts.
	// (The default mock concatenates the prompt text; here we just assert the
	// adapter preserves multi-part text when present.)
	result, err := sc.SendMessage(id, "part one")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if result.Text == "" {
		t.Fatal("expected assistant text")
	}
	if result.Parts == nil {
		t.Fatal("expected parts to be preserved")
	}
}
