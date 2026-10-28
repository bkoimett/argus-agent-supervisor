// Command agent-manager is the local Go service that manages coding agent
// sessions for Argus. See ../../README.md and the repo root AGENTS.md
// before extending this.
package main

import (
	"fmt"
	"os"
)

func main() {
	// TODO (M1): start a local health check endpoint the desktop app can
	// poll on launch, per WORKFLOW.md M1 checkpoint.
	// TODO (M2): wire session.Manager and adapter.OpenCodeAdapter here.
	fmt.Println("argus agent-manager: scaffold only, not yet implemented")
	os.Exit(0)
}
