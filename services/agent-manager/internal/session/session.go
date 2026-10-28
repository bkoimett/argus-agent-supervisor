// Package session manages the set of active sessions and their local
// (SQLite-backed) state. TODO (M2): back this with actual SQLite storage,
// see docs/prd.md section 12 for the data model.
package session

import "github.com/argus-command-center/argus/services/agent-manager/internal/adapter"

// Session is a single running agent session against one project.
type Session struct {
	ID         string
	ProjectDir string
	Status     adapter.Status
}

// Manager tracks all active sessions. TODO (M2): implement with SQLite
// persistence so state survives a GUI restart, per docs/prd.md section 14.
type Manager struct {
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
}
