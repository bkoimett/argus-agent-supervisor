// Command agent-manager is the local Go service that manages coding agent
// sessions for Argus. See ../../README.md and the repo root AGENTS.md
// before extending this.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/adapter"
	"github.com/argus-command-center/argus/services/agent-manager/internal/dashboard"
	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
	"github.com/argus-command-center/argus/services/agent-manager/internal/notification"
	"github.com/argus-command-center/argus/services/agent-manager/internal/session"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/argus-command-center/argus/services/agent-manager/internal/subprocess"
	"github.com/argus-command-center/argus/services/agent-manager/internal/supabase"
)

func main() {
	// Load configuration from a .env file when present. Candidate locations
	// are the working directory, the directory containing this binary, and
	// each parent directory up to the repository root (so a .env placed next
	// to the source tree is picked up regardless of where the binary is
	// launched from). Environment variables and explicit CLI flags always
	// take precedence over .env; .env values are never logged.
	loadEnvFile()

	envStr := func(key, def string) string {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			return v
		}
		return def
	}
	envBool := func(key string, def bool) bool {
		if v, ok := os.LookupEnv(key); ok {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "1", "true", "yes", "on":
				return true
			case "0", "false", "no", "off":
				return false
			}
		}
		return def
	}

	var (
		dbPath             = flag.String("db", envStr("AGENT_MANAGER_DB", "argus.db"), "SQLite database path")
		port               = flag.String("port", envStr("AGENT_MANAGER_PORT", "8080"), "HTTP dashboard port")
		opencodeBin        = flag.String("opencode", envStr("OPENCODE_BINARY", "opencode"), "OpenCode binary path")
		model              = flag.String("model", envStr("OPENCODE_MODEL", ""), "Default model (provider/model)")
		autoApprove        = flag.Bool("auto", envBool("OPENCODE_AUTO_APPROVE", false), "Auto-approve permissions (DANGEROUS)")
		metaAPIKey         = flag.String("meta-api-key", envStr("META_API_KEY", ""), "Meta API key (enables Meta diagnosis)")
		metaBaseURL        = flag.String("meta-base-url", envStr("META_BASE_URL", ""), "Meta API base URL")
		metaModel          = flag.String("meta-model", envStr("META_MODEL", ""), "Meta model to use")
		supabaseURL        = flag.String("supabase-url", envStr("SUPABASE_URL", ""), "Supabase project URL")
		supabaseKey        = flag.String("supabase-key", envStr("SUPABASE_ANON_KEY", ""), "Supabase anon key")
		supabaseSvc        = flag.String("supabase-service-key", envStr("SUPABASE_SERVICE_KEY", ""), "Supabase service key")
		notifEnabled       = flag.Bool("notif-enabled", envBool("NOTIF_ENABLED", false), "Enable push notifications")
		notifVAPIDPub      = flag.String("notif-vapid-public", envStr("NOTIF_VAPID_PUBLIC", ""), "VAPID public key")
		notifVAPIDPriv     = flag.String("notif-vapid-private", envStr("NOTIF_VAPID_PRIVATE", ""), "VAPID private key")
		notifVAPIDSub      = flag.String("notif-vapid-subject", envStr("NOTIF_VAPID_SUBJECT", "mailto:admin@example.com"), "VAPID subject")
		notifNtfyTopic     = flag.String("notif-ntfy-topic", envStr("NOTIF_NTFY_TOPIC", ""), "ntfy.sh topic for fallback")
		notifNtfyURL       = flag.String("notif-ntfy-url", envStr("NOTIF_NTFY_URL", "https://ntfy.sh"), "ntfy.sh URL")
		notifNtfyBroadcast = flag.Bool("notif-ntfy-broadcast", envBool("NOTIF_NTFY_BROADCAST", false), "Allow ntfy broadcast without subscription")
	)
	flag.Parse()

	// Initialize Supabase client
	supabaseConfig := supabase.Config{
		URL:        *supabaseURL,
		AnonKey:    *supabaseKey,
		ServiceKey: *supabaseSvc,
		Enabled:    *supabaseURL != "" && *supabaseKey != "",
	}
	sbClient, err := supabase.NewClient(supabaseConfig)
	if err != nil {
		log.Fatalf("Failed to create Supabase client: %v", err)
	}
	defer sbClient.Close()

	// Initialize notification client
	notifConfig := notification.Config{
		VAPIDPublicKey:     *notifVAPIDPub,
		VAPIDPrivateKey:    *notifVAPIDPriv,
		VAPIDSubject:       *notifVAPIDSub,
		NtfyTopic:          *notifNtfyTopic,
		NtfyURL:            *notifNtfyURL,
		NtfyAllowBroadcast: *notifNtfyBroadcast,
		Enabled:            *notifEnabled,
	}
	notifClient := notification.NewClient(notifConfig)

	// Initialize store
	st, err := store.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}
	defer st.Close()

	// Initialize subprocess manager
	procConfig := subprocess.OpenCodeConfig{
		BinaryPath:  *opencodeBin,
		Model:       *model,
		AutoApprove: *autoApprove,
		Timeout:     5 * time.Minute,
	}
	procMgr := subprocess.NewProcessManager(st, sbClient, notifClient, procConfig)

	// Initialize OpenCode adapter (headless `opencode serve` interface).
	// Owns a detached `opencode serve` process shared across sessions.
	opencodeAdapter := adapter.NewOpenCodeAdapter(*opencodeBin)

	// Initialize meta config
	metaConfig := meta.Config{
		APIKey:  *metaAPIKey,
		BaseURL: *metaBaseURL,
		Model:   *metaModel,
	}
	mgr := session.NewManager(st, procMgr, metaConfig, sbClient, notifClient)

	// Log Meta diagnosis mode
	if metaConfig.APIKey != "" && metaConfig.BaseURL != "" {
		modelName := metaConfig.Model
		if modelName == "" {
			modelName = "default"
		}
		log.Printf("Meta diagnosis: live (model %s)", modelName)
	} else {
		log.Printf("Meta diagnosis: fallback (not configured)")
	}

	// Restore sessions that were running when the service stopped.
	// OpenCode processes are not children of the GUI and do not survive
	// restarts, so sessions in non-terminal states are marked DISCONNECTED.
	if err := mgr.RestoreSessions(); err != nil {
		log.Printf("failed to restore sessions: %v", err)
	}

	// Initialize dashboard server
	startTime := time.Now()
	dash := dashboard.NewServer(mgr, st, *port, opencodeAdapter, sbClient, startTime)
	dash.MetaEnvPath = dashboard.ResolveMetaEnvPath()

	// Start command processing loop (phone → agent messaging via Supabase commands table).
	// The laptop initiates the outbound connection to Supabase; nothing is exposed publicly.
	if sbClient != nil && sbClient.IsEnabled() {
		go processCommands(sbClient, opencodeAdapter)
	}

	// Start dashboard in background. A bind failure (e.g. port already in use by a
	// stale instance) is fatal: the new sidecar must exit rather than linger as a
	// zombie that the UI could silently mistake for a fresh service.
	dashErrCh := make(chan error, 1)
	go func() {
		if err := dash.Start(); err != nil && err != http.ErrServerClosed {
			dashErrCh <- err
		}
	}()
	select {
	case err := <-dashErrCh:
		log.Fatalf("dashboard server failed to start: %v", err)
	case <-time.After(100 * time.Millisecond):
		// Give the server a moment to bind; if it fails shortly after, the
		// goroutine above will still surface it via log output on the next
		// signal cycle. The readiness check is owned by the Tauri sidecar.
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")

	// Stop all running sessions
	for _, sessionID := range procMgr.ListSessions() {
		if err := procMgr.StopSession(sessionID); err != nil {
			log.Printf("Failed to stop session %s: %v", sessionID, err)
		}
	}

	// Shut down the OpenCode adapter and its managed serve process.
	if err := opencodeAdapter.Close(); err != nil {
		log.Printf("Failed to close OpenCode adapter: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	log.Println("Shutdown complete")
}

// processCommands polls Supabase for pending commands and executes them via the adapter.
// The laptop initiates the outbound connection to Supabase; nothing is exposed publicly.
func processCommands(cmdStore commandStore, adptr adapter.AgentAdapter) {
	for {
		cmds, err := cmdStore.ListPendingCommands(context.Background())
		if err != nil {
			log.Printf("command poll error: %v", err)
		} else {
			for _, cmd := range cmds {
				processCommand(cmdStore, adptr, cmd)
			}
		}
		time.Sleep(2 * time.Second)
	}
}

// commandStore is the subset of Supabase operations needed for command processing.
type commandStore interface {
	ListPendingCommands(ctx context.Context) ([]*state.Command, error)
	SyncCommand(ctx context.Context, cmd *state.Command) error
	SyncEvent(ctx context.Context, evt *state.Event) error
}

func processCommand(cmdStore commandStore, adptr adapter.AgentAdapter, cmd *state.Command) {
	ctx := context.Background()
	switch cmd.Type {
	case "send_message":
		var payload struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			log.Printf("invalid send_message payload: %v", err)
			cmd.Status = "FAILED"
			_ = cmdStore.SyncCommand(ctx, cmd)
			return
		}
		result, err := adptr.SendMessage(cmd.SessionID, payload.Message)
		if err != nil {
			log.Printf("send message failed: %v", err)
			cmd.Status = "FAILED"
			_ = cmdStore.SyncCommand(ctx, cmd)
			return
		}
		// Persist the exchange so it's visible on phone and desktop.
		now := time.Now()
		_ = cmdStore.SyncEvent(ctx, &state.Event{
			SessionID: cmd.SessionID,
			Type:      "user_message",
			Timestamp: now,
			Payload:   map[string]any{"text": payload.Message},
		})
		_ = cmdStore.SyncEvent(ctx, &state.Event{
			SessionID: cmd.SessionID,
			Type:      "assistant_message",
			Timestamp: now,
			Payload: map[string]any{
				"text":      result.Text,
				"messageID": result.MessageID,
				"finish":    result.Finish,
				"tokens":    result.Tokens,
			},
		})
		cmd.Status = "EXECUTED"
		_ = cmdStore.SyncCommand(ctx, cmd)
	case "stop", "abort":
		if err := adptr.Stop(cmd.SessionID); err != nil {
			log.Printf("stop session failed: %v", err)
			cmd.Status = "FAILED"
			_ = cmdStore.SyncCommand(ctx, cmd)
			return
		}
		cmd.Status = "EXECUTED"
		_ = cmdStore.SyncCommand(ctx, cmd)
	default:
		log.Printf("unknown command type: %s", cmd.Type)
		cmd.Status = "FAILED"
		_ = cmdStore.SyncCommand(ctx, cmd)
	}
}

// loadEnvFile reads KEY=VALUE lines from candidate .env files and populates
// the process environment for keys not already set. Candidate locations are
// the working directory, the directory containing this binary, and each
// parent directory up to the filesystem root (so a .env placed next to the
// source tree is found regardless of where the binary is launched from).
// Environment variables and explicit CLI flags take precedence over .env.
// Secret values are never logged.
func loadEnvFile() {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".env"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, ".env"))
		// Walk up from the binary directory looking for a repo-root .env.
		d := dir
		for i := 0; i < 6; i++ {
			parent := filepath.Dir(d)
			if parent == d {
				break
			}
			candidates = append(candidates, filepath.Join(parent, ".env"))
			d = parent
		}
	}

	seen := make(map[string]bool)
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			val = strings.TrimSpace(val)
			val = strings.Trim(val, "\"")
			val = strings.Trim(val, "'")
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			// Environment variables and CLI flags always win over .env.
			if _, ok := os.LookupEnv(key); !ok {
				_ = os.Setenv(key, val)
			}
		}
	}
}
