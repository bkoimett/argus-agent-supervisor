// Package dashboard provides a simple HTTP API for the Argus dashboard.
package dashboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/session"
	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	"github.com/argus-command-center/argus/services/agent-manager/internal/store"
	"github.com/google/uuid"
)

// Server is the HTTP dashboard server.
type Server struct {
	mgr   *session.Manager
	store *store.Store
	port  string
}

// NewServer creates a new dashboard server.
func NewServer(mgr *session.Manager, st *store.Store, port string) *Server {
	return &Server{mgr: mgr, store: st, port: port}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Serve static dashboard UI
	mux.Handle("/", http.FileServer(http.Dir("./internal/dashboard/static")))

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// Session endpoints
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/", s.handleSessionByID)

	// Task endpoints
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/tasks/", s.handleTaskByID)

	// Incident endpoints
	mux.HandleFunc("/api/incidents", s.handleIncidents)
	mux.HandleFunc("/api/incidents/", s.handleIncidentByID)

	// Approval endpoints
	mux.HandleFunc("/api/approvals", s.handleApprovals)
	mux.HandleFunc("/api/approvals/", s.handleApprovalByID)

	// Recovery endpoints
	mux.HandleFunc("/api/recovery", s.handleRecovery)
	mux.HandleFunc("/api/recovery/", s.handleRecoveryByID)
	mux.HandleFunc("/api/recovery/approval", s.handleRecoveryApproval)

	// Diagnosis endpoint
	mux.HandleFunc("/api/diagnosis", s.handleDiagnosis)

	// Event endpoints
	mux.HandleFunc("/api/events", s.handleEvents)

	// Dashboard summary
	mux.HandleFunc("/api/dashboard", s.handleDashboard)

	// Wrap with CORS middleware for local development (Vite dev server) and Tauri
	handler := corsMiddleware(mux)

	server := &http.Server{
		Addr:         ":" + s.port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return server.ListenAndServe()
}

// corsMiddleware adds permissive CORS headers for known local origins.
func corsMiddleware(next http.Handler) http.Handler {
	allowed := map[string]bool{
		"http://localhost:5173": true, // Vite dev server
		"http://127.0.0.1:5173": true,
		"http://localhost:8080": true, // Go service self
		"http://127.0.0.1:8080": true,
		"tauri://localhost":       true, // Tauri custom scheme (production)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "false")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions, err := s.mgr.ListSessions()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, sessions)
	case http.MethodPost:
		var req struct {
			ProjectDir string `json:"project_dir"`
			Model      string `json:"model"`
			Prompt     string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.ProjectDir == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("project_dir is required"))
			return
		}
		session, task, err := s.mgr.CreateSession(r.Context(), req.ProjectDir, req.Model, req.Prompt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"session": session,
			"task":    task,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/sessions/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		session, err := s.mgr.GetSession(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, session)
	case http.MethodDelete:
		if err := s.mgr.StopSession(id); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		tasks, err := s.mgr.GetSessionTasks(sessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	case http.MethodPost:
		var req struct {
			Prompt       string   `json:"prompt"`
			Scope        string   `json:"scope"`
			Dependencies []string `json:"dependencies"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		task, err := s.mgr.SubmitTask(sessionID, req.Prompt, req.Scope, req.Dependencies)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, task)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/tasks/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	// For simplicity, we don't have a GetTask in manager, would need to add
	writeError(w, http.StatusNotImplemented, nil)
}

func (s *Server) handleIncidents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		incidents, err := s.mgr.GetSessionIncidents(sessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, incidents)
	case http.MethodPost:
		var req struct {
			SessionID string          `json:"session_id"`
			TaskID    *string         `json:"task_id"`
			Type      string          `json:"type"`
			Severity  string          `json:"severity"`
			Evidence  json.RawMessage `json:"evidence"`
			Action    string          `json:"action"` // "detect"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.Action == "detect" {
			// Use sessionID from query param for detect action
			incidents, err := s.mgr.DetectIncidentsFromEvents(sessionID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, incidents)
			return
		}
		inc := &state.Incident{
			ID:         uuid.New().String(),
			SessionID:  req.SessionID,
			TaskID:     req.TaskID,
			Type:       req.Type,
			Severity:   state.IncidentSeverity(req.Severity),
			Status:     state.IncidentStatusDetected,
			Evidence:   req.Evidence,
			DetectedAt: time.Now(),
		}
		if err := s.mgr.CreateIncident(inc); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, inc)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleIncidentByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/incidents/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		incident, err := s.mgr.GetIncident(id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, nil)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, incident)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		approvals, err := s.mgr.GetPendingApprovals()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, approvals)
	case http.MethodPost:
		var req struct {
			SessionID   string          `json:"session_id"`
			TaskID      *string         `json:"task_id"`
			IncidentID  *string         `json:"incident_id"`
			Type        string          `json:"type"`
			Title       string          `json:"title"`
			Description string          `json:"description"`
			Payload     json.RawMessage `json:"payload"`
			Scope       string          `json:"scope"`
			ExpiresIn   int             `json:"expires_in_seconds"` // default 300
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.ExpiresIn == 0 {
			req.ExpiresIn = 300
		}
		approval := &state.Approval{
			ID:          uuid.New().String(),
			SessionID:   req.SessionID,
			TaskID:      req.TaskID,
			IncidentID:  req.IncidentID,
			Type:        state.ApprovalType(req.Type),
			Status:      state.ApprovalStatusPending,
			Title:       req.Title,
			Description: req.Description,
			Payload:     req.Payload,
			Scope:       req.Scope,
			RequestedAt: time.Now(),
			ExpiresAt:   time.Now().Add(time.Duration(req.ExpiresIn) * time.Second),
		}
		if err := s.mgr.RequestApproval(approval); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, approval)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleApprovalByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/approvals/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Approved  *bool  `json:"approved"`
			DecidedBy string `json:"decided_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.Approved == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("approved field is required"))
			return
		}
		if req.DecidedBy == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("decided_by is required"))
			return
		}
		if err := s.mgr.RespondApproval(id, *req.Approved, req.DecidedBy); err != nil {
			// Map specific errors to appropriate status codes
			status := http.StatusInternalServerError
			if err == sql.ErrNoRows {
				status = http.StatusNotFound
			} else {
				errMsg := err.Error()
				if strings.Contains(errMsg, "not pending") || strings.Contains(errMsg, "expired") {
					status = http.StatusConflict // 409
				} else if strings.Contains(errMsg, "missing") || strings.Contains(errMsg, "invalid") {
					status = http.StatusBadRequest
				} else if strings.Contains(errMsg, "not found") {
					status = http.StatusNotFound
				}
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "responded"})
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleRecovery(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		incidentID := r.URL.Query().Get("incident_id")
		actions, err := s.mgr.ListRecoveryActions(incidentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, actions)
	case http.MethodPost:
		var req struct {
			IncidentID  string   `json:"incident_id"`
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Command     string   `json:"command"`
			Args        []string `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		action := &state.RecoveryAction{
			ID:          uuid.New().String(),
			IncidentID:  req.IncidentID,
			Type:        state.RecoveryActionType(req.Type),
			Description: req.Description,
			Command:     req.Command,
			Args:        req.Args,
		}
		if err := s.mgr.ProposeRecovery(req.IncidentID, action); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, action)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleRecoveryByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/recovery/"):]
	if id == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		action, err := s.mgr.GetRecoveryAction(id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, nil)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, action)
	case http.MethodPost:
		// Execute recovery
		result, err := s.mgr.ExecuteRecovery(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

// Recovery approval endpoints
func (s *Server) handleRecoveryApproval(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// Request recovery approval for a recovery action
		var req struct {
			RecoveryID string `json:"recovery_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.RecoveryID == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("recovery_id is required"))
			return
		}
		approval, err := s.mgr.RequestRecoveryApproval(req.RecoveryID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, approval)
	default:
		writeError(w, http.StatusMethodNotAllowed, nil)
	}
}

// Diagnosis endpoint
func (s *Server) handleDiagnosis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	var req struct {
		IncidentID string `json:"incident_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := s.mgr.DiagnoseIncident(req.IncidentID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "diagnosis completed"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, nil)
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}

	events, err := s.mgr.GetSessionEvents(sessionID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.mgr.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if sessions == nil {
		sessions = []*state.Session{}
	}

	type DashboardSession struct {
		*state.Session
		Tasks           []*state.Task           `json:"tasks"`
		Incidents       []*state.Incident       `json:"incidents"`
		Approvals       []*state.Approval       `json:"approvals"`
		RecentEvents    []*state.Event          `json:"recent_events"`
		RecoveryActions []*state.RecoveryAction `json:"recovery_actions"`
	}

	dashboard := make([]DashboardSession, 0, len(sessions))
	for _, sess := range sessions {
		tasks, _ := s.mgr.GetSessionTasks(sess.ID)
		if tasks == nil {
			tasks = []*state.Task{}
		}
		incidents, _ := s.mgr.GetSessionIncidents(sess.ID)
		if incidents == nil {
			incidents = []*state.Incident{}
		}
		approvals, _ := s.store.ListPendingApprovals()
		if approvals == nil {
			approvals = []*state.Approval{}
		}
		// Filter approvals for this session
		var sessApprovals []*state.Approval
		for _, a := range approvals {
			if a.SessionID == sess.ID {
				sessApprovals = append(sessApprovals, a)
			}
		}
		events, _ := s.mgr.GetSessionEvents(sess.ID, 20)
		if events == nil {
			events = []*state.Event{}
		}
		recoveryActions, _ := s.mgr.ListRecoveryActions("")
		if recoveryActions == nil {
			recoveryActions = []*state.RecoveryAction{}
		}

		// Filter recovery actions for this session's incidents
		var sessRecoveryActions []*state.RecoveryAction
		for _, ra := range recoveryActions {
			for _, inc := range incidents {
				if ra.IncidentID == inc.ID {
					sessRecoveryActions = append(sessRecoveryActions, ra)
					break
				}
			}
		}
		if sessRecoveryActions == nil {
			sessRecoveryActions = []*state.RecoveryAction{}
		}

		dashboard = append(dashboard, DashboardSession{
			Session:         sess,
			Tasks:           tasks,
			Incidents:       incidents,
			Approvals:       sessApprovals,
			RecentEvents:    events,
			RecoveryActions: sessRecoveryActions,
		})
	}

	writeJSON(w, http.StatusOK, dashboard)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err error) {
	msg := "internal error"
	if err != nil {
		msg = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
