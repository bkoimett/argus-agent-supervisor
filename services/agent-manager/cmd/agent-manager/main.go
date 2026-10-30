// Command agent-manager is the local Go service that manages coding agent
// sessions for Argus. See ../../README.md and the repo root AGENTS.md
// before extending this.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/dashboard"
	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
	"github.com/argus-command-center/argus/services/agent-manager/internal/session"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/argus-command-center/argus/services/agent-manager/internal/subprocess"
)

func main() {
	var (
		dbPath      = flag.String("db", "argus.db", "SQLite database path")
		port        = flag.String("port", "8080", "HTTP dashboard port")
		opencodeBin = flag.String("opencode", "opencode", "OpenCode binary path")
		model       = flag.String("model", "", "Default model (provider/model)")
		autoApprove = flag.Bool("auto", false, "Auto-approve permissions (DANGEROUS)")
		metaAPIKey  = flag.String("meta-api-key", "", "Meta API key (enables Meta diagnosis)")
		metaBaseURL = flag.String("meta-base-url", "https://api.meta.example.com/v1", "Meta API base URL")
		metaModel   = flag.String("meta-model", "meta-llama-3-70b", "Meta model to use")
	)
	flag.Parse()

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
	procMgr := subprocess.NewProcessManager(st, procConfig)

	// Initialize meta config
	metaConfig := meta.Config{
		APIKey:  *metaAPIKey,
		BaseURL: *metaBaseURL,
		Model:   *metaModel,
		Enabled: *metaAPIKey != "",
	}

	// Initialize session manager
	mgr := session.NewManager(st, procMgr, metaConfig)

	// Initialize dashboard server
	dash := dashboard.NewServer(mgr, st, *port)

	// Start dashboard in background
	go func() {
		if err := dash.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("Dashboard server error: %v", err)
		}
	}()

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

	time.Sleep(500 * time.Millisecond)
	log.Println("Shutdown complete")
}
