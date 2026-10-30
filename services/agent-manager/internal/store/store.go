// Package store provides SQLite persistence for Argus state.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/argus-command-center/argus/services/agent-manager/internal/state"
	_ "github.com/mattn/go-sqlite3"
)

// Store handles all database operations.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store with the given database path.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Transaction executes fn within a database transaction.
func (s *Store) Transaction(fn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx rollback: %v (original: %w)", rbErr, err)
		}
		return err
	}
	return tx.Commit()
}

// RespondRecoveryApprovalTx atomically updates approval, incident, and recovery action for a recovery approval.
func (s *Store) RespondRecoveryApprovalTx(approvalID, recoveryID string, approved bool, decidedBy string, now time.Time) error {
	return s.Transaction(func(tx *sql.Tx) error {
		// Update approval
		approvalStatus := state.ApprovalStatusApproved
		if !approved {
			approvalStatus = state.ApprovalStatusRejected
		}
		decision := approved
		_, err := tx.Exec(`
			UPDATE approvals SET status=?, responded_at=?, decision=?, decided_by=? WHERE id=?
		`, approvalStatus, now, decision, decidedBy, approvalID)
		if err != nil {
			return fmt.Errorf("update approval: %w", err)
		}

		// Update incident
		incidentStatus := state.IncidentStatusApproved
		if !approved {
			incidentStatus = state.IncidentStatusRejected
		}
		_, err = tx.Exec(`
			UPDATE incidents SET status=?, approved_at=? WHERE id=(SELECT incident_id FROM recovery_actions WHERE id=?)
		`, incidentStatus, nullTimeIfNotApproved(approved, now), recoveryID)
		if err != nil {
			return fmt.Errorf("update incident: %w", err)
		}

		// Update recovery action
		raStatus := state.IncidentStatusApproved
		if !approved {
			raStatus = state.IncidentStatusRejected
		}
		_, err = tx.Exec(`
			UPDATE recovery_actions SET status=?, approved_at=? WHERE id=?
		`, raStatus, nullTimeIfNotApproved(approved, now), recoveryID)
		if err != nil {
			return fmt.Errorf("update recovery action: %w", err)
		}
		return nil
	})
}

// nullTimeIfNotApproved returns a sql.NullTime for approved_at (null when not approved).
func nullTimeIfNotApproved(approved bool, t time.Time) sql.NullTime {
	if approved {
		return sql.NullTime{Time: t, Valid: true}
	}
	return sql.NullTime{Valid: false}
}

// migrate creates tables if they don't exist.
func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		project_dir TEXT NOT NULL,
		agent TEXT NOT NULL DEFAULT 'opencode',
		model TEXT,
		status TEXT NOT NULL,
		pid INTEGER,
		started_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		ended_at DATETIME,
		exit_code INTEGER,
		error_message TEXT
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		prompt TEXT NOT NULL,
		status TEXT NOT NULL,
		dependencies TEXT NOT NULL DEFAULT '[]',
		scope TEXT NOT NULL DEFAULT '',
		started_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		ended_at DATETIME,
		exit_code INTEGER,
		error_message TEXT,
		approval_id TEXT REFERENCES approvals(id),
		recovery_count INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_session ON tasks(session_id);

	CREATE TABLE IF NOT EXISTS incidents (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
		type TEXT NOT NULL,
		severity TEXT NOT NULL,
		status TEXT NOT NULL,
		evidence TEXT NOT NULL,
		meta_diagnosis TEXT,
		meta_recovery TEXT,
		detected_at DATETIME NOT NULL,
		diagnosed_at DATETIME,
		proposed_at DATETIME,
		approved_at DATETIME,
		recovered_at DATETIME,
		verified_at DATETIME,
		recovery_attempts INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_incidents_session ON incidents(session_id);
	CREATE INDEX IF NOT EXISTS idx_incidents_task ON incidents(task_id);

	CREATE TABLE IF NOT EXISTS approvals (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
		incident_id TEXT REFERENCES incidents(id) ON DELETE SET NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		payload TEXT NOT NULL,
		scope TEXT NOT NULL DEFAULT '',
		requested_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		responded_at DATETIME,
		decision INTEGER,
		decided_by TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_approvals_session ON approvals(session_id);
	CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);

	CREATE TABLE IF NOT EXISTS recovery_actions (
		id TEXT PRIMARY KEY,
		incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		description TEXT NOT NULL,
		command TEXT NOT NULL,
		args TEXT NOT NULL DEFAULT '[]',
		status TEXT NOT NULL,
		proposed_at DATETIME NOT NULL,
		approved_at DATETIME,
		executed_at DATETIME,
		result TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_recovery_incident ON recovery_actions(incident_id);

	CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		payload TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Session operations

func (s *Store) CreateSession(session *state.Session) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (id, project_dir, agent, model, status, pid, started_at, updated_at, ended_at, exit_code, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.ProjectDir, session.Agent, session.Model, session.Status, session.PID,
		session.StartedAt, session.UpdatedAt, session.EndedAt, session.ExitCode, session.ErrorMessage)
	return err
}

func (s *Store) UpdateSession(session *state.Session) error {
	session.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE sessions SET status=?, pid=?, updated_at=?, ended_at=?, exit_code=?, error_message=? WHERE id=?
	`, session.Status, session.PID, session.UpdatedAt, session.EndedAt, session.ExitCode, session.ErrorMessage, session.ID)
	return err
}

func (s *Store) GetSession(id string) (*state.Session, error) {
	row := s.db.QueryRow(`
		SELECT id, project_dir, agent, model, status, pid, started_at, updated_at, ended_at, exit_code, error_message
		FROM sessions WHERE id=?
	`, id)

	var sess state.Session
	var endedAt sql.NullTime
	var exitCodeInt sql.NullInt64
	var errMsg sql.NullString
	err := row.Scan(&sess.ID, &sess.ProjectDir, &sess.Agent, &sess.Model, &sess.Status, &sess.PID,
		&sess.StartedAt, &sess.UpdatedAt, &endedAt, &exitCodeInt, &errMsg)
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		sess.EndedAt = &endedAt.Time
	}
	if exitCodeInt.Valid {
		v := int(exitCodeInt.Int64)
		sess.ExitCode = &v
	}
	if errMsg.Valid {
		sess.ErrorMessage = errMsg.String
	}
	return &sess, nil
}

func (s *Store) ListSessions() ([]*state.Session, error) {
	rows, err := s.db.Query(`
		SELECT id, project_dir, agent, model, status, pid, started_at, updated_at, ended_at, exit_code, error_message
		FROM sessions ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*state.Session
	for rows.Next() {
		var sess state.Session
		var endedAt sql.NullTime
		var exitCodeInt sql.NullInt64
		var errMsg sql.NullString
		err := rows.Scan(&sess.ID, &sess.ProjectDir, &sess.Agent, &sess.Model, &sess.Status, &sess.PID,
			&sess.StartedAt, &sess.UpdatedAt, &endedAt, &exitCodeInt, &errMsg)
		if err != nil {
			return nil, err
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		if exitCodeInt.Valid {
			v := int(exitCodeInt.Int64)
			sess.ExitCode = &v
		}
		if errMsg.Valid {
			sess.ErrorMessage = errMsg.String
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// Task operations

func (s *Store) CreateTask(task *state.Task) error {
	deps, _ := json.Marshal(task.Dependencies)
	_, err := s.db.Exec(`
		INSERT INTO tasks (id, session_id, prompt, status, dependencies, scope, started_at, updated_at, ended_at, exit_code, error_message, approval_id, recovery_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.SessionID, task.Prompt, task.Status, string(deps), task.Scope,
		task.StartedAt, task.UpdatedAt, task.EndedAt, task.ExitCode, task.ErrorMessage, task.ApprovalID, task.RecoveryCount)
	return err
}

func (s *Store) UpdateTask(task *state.Task) error {
	task.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE tasks SET status=?, updated_at=?, ended_at=?, exit_code=?, error_message=?, approval_id=?, recovery_count=?
		WHERE id=?
	`, task.Status, task.UpdatedAt, task.EndedAt, task.ExitCode, task.ErrorMessage, task.ApprovalID, task.RecoveryCount, task.ID)
	return err
}

func (s *Store) GetTask(id string) (*state.Task, error) {
	row := s.db.QueryRow(`
		SELECT id, session_id, prompt, status, dependencies, scope, started_at, updated_at, ended_at, exit_code, error_message, approval_id, recovery_count
		FROM tasks WHERE id=?
	`, id)

	var task state.Task
	var deps string
	var endedAt sql.NullTime
	var exitCodeInt sql.NullInt64
	var errMsg, approvalID sql.NullString
	err := row.Scan(&task.ID, &task.SessionID, &task.Prompt, &task.Status, &deps, &task.Scope,
		&task.StartedAt, &task.UpdatedAt, &endedAt, &exitCodeInt, &errMsg, &approvalID, &task.RecoveryCount)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(deps), &task.Dependencies)
	if endedAt.Valid {
		task.EndedAt = &endedAt.Time
	}
	if exitCodeInt.Valid {
		v := int(exitCodeInt.Int64)
		task.ExitCode = &v
	}
	if errMsg.Valid {
		task.ErrorMessage = errMsg.String
	}
	if approvalID.Valid {
		task.ApprovalID = &approvalID.String
	}
	return &task, nil
}

func (s *Store) ListTasksBySession(sessionID string) ([]*state.Task, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, prompt, status, dependencies, scope, started_at, updated_at, ended_at, exit_code, error_message, approval_id, recovery_count
		FROM tasks WHERE session_id=? ORDER BY started_at
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*state.Task
	for rows.Next() {
		var task state.Task
		var deps string
		var endedAt sql.NullTime
		var exitCodeInt sql.NullInt64
		var errMsg, approvalID sql.NullString
		err := rows.Scan(&task.ID, &task.SessionID, &task.Prompt, &task.Status, &deps, &task.Scope,
			&task.StartedAt, &task.UpdatedAt, &endedAt, &exitCodeInt, &errMsg, &approvalID, &task.RecoveryCount)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(deps), &task.Dependencies)
		if endedAt.Valid {
			task.EndedAt = &endedAt.Time
		}
		if exitCodeInt.Valid {
			v := int(exitCodeInt.Int64)
			task.ExitCode = &v
		}
		if errMsg.Valid {
			task.ErrorMessage = errMsg.String
		}
		if approvalID.Valid {
			task.ApprovalID = &approvalID.String
		}
		tasks = append(tasks, &task)
	}
	return tasks, rows.Err()
}

// UpdateTasksBySession updates all tasks for a session to a final status.
func (s *Store) UpdateTasksBySession(sessionID string, status state.TaskStatus, exitCode *int) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE tasks SET status=?, updated_at=?, ended_at=?, exit_code=? WHERE session_id=? AND status NOT IN (?, ?)
	`, status, now, now, exitCode, sessionID, state.TaskStatusCompleted, state.TaskStatusFailed)
	return err
}

// Incident operations

func (s *Store) CreateIncident(inc *state.Incident) error {
	evidence, _ := json.Marshal(inc.Evidence)
	metaRecovery, _ := json.Marshal(inc.MetaRecovery)
	_, err := s.db.Exec(`
		INSERT INTO incidents (id, session_id, task_id, type, severity, status, evidence, meta_diagnosis, meta_recovery,
			detected_at, diagnosed_at, proposed_at, approved_at, recovered_at, verified_at, recovery_attempts)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, inc.ID, inc.SessionID, inc.TaskID, inc.Type, inc.Severity, inc.Status,
		string(evidence), inc.MetaDiagnosis, string(metaRecovery),
		inc.DetectedAt, inc.DiagnosedAt, inc.ProposedAt, inc.ApprovedAt, inc.RecoveredAt, inc.VerifiedAt, inc.RecoveryAttempts)
	return err
}

func (s *Store) UpdateIncident(inc *state.Incident) error {
	metaRecovery, _ := json.Marshal(inc.MetaRecovery)
	_, err := s.db.Exec(`
		UPDATE incidents SET status=?, meta_diagnosis=?, meta_recovery=?, diagnosed_at=?, proposed_at=?, approved_at=?, recovered_at=?, verified_at=?, recovery_attempts=?
		WHERE id=?
	`, inc.Status, inc.MetaDiagnosis, string(metaRecovery), inc.DiagnosedAt, inc.ProposedAt, inc.ApprovedAt, inc.RecoveredAt, inc.VerifiedAt, inc.RecoveryAttempts, inc.ID)
	return err
}

func (s *Store) GetIncident(id string) (*state.Incident, error) {
	row := s.db.QueryRow(`
		SELECT id, session_id, task_id, type, severity, status, evidence, meta_diagnosis, meta_recovery,
			detected_at, diagnosed_at, proposed_at, approved_at, recovered_at, verified_at, recovery_attempts
		FROM incidents WHERE id=?
	`, id)

	var inc state.Incident
	var evidence, metaRecovery string
	var taskID, metaDiag sql.NullString
	var diagnosedAt, proposedAt, approvedAt, recoveredAt, verifiedAt sql.NullTime
	err := row.Scan(&inc.ID, &inc.SessionID, &taskID, &inc.Type, &inc.Severity, &inc.Status,
		&evidence, &metaDiag, &metaRecovery,
		&inc.DetectedAt, &diagnosedAt, &proposedAt, &approvedAt, &recoveredAt, &verifiedAt, &inc.RecoveryAttempts)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(evidence), &inc.Evidence)
	if taskID.Valid {
		inc.TaskID = &taskID.String
	}
	if metaDiag.Valid {
		inc.MetaDiagnosis = metaDiag.String
	}
	json.Unmarshal([]byte(metaRecovery), &inc.MetaRecovery)
	if diagnosedAt.Valid {
		inc.DiagnosedAt = &diagnosedAt.Time
	}
	if proposedAt.Valid {
		inc.ProposedAt = &proposedAt.Time
	}
	if approvedAt.Valid {
		inc.ApprovedAt = &approvedAt.Time
	}
	if recoveredAt.Valid {
		inc.RecoveredAt = &recoveredAt.Time
	}
	if verifiedAt.Valid {
		inc.VerifiedAt = &verifiedAt.Time
	}
	return &inc, nil
}

func (s *Store) ListIncidentsBySession(sessionID string) ([]*state.Incident, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, task_id, type, severity, status, evidence, meta_diagnosis, meta_recovery,
			detected_at, diagnosed_at, proposed_at, approved_at, recovered_at, verified_at, recovery_attempts
		FROM incidents WHERE session_id=? ORDER BY detected_at DESC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []*state.Incident
	for rows.Next() {
		var inc state.Incident
		var evidence, metaRecovery string
		var taskID, metaDiag sql.NullString
		var diagnosedAt, proposedAt, approvedAt, recoveredAt, verifiedAt sql.NullTime
		err := rows.Scan(&inc.ID, &inc.SessionID, &taskID, &inc.Type, &inc.Severity, &inc.Status,
			&evidence, &metaDiag, &metaRecovery,
			&inc.DetectedAt, &diagnosedAt, &proposedAt, &approvedAt, &recoveredAt, &verifiedAt, &inc.RecoveryAttempts)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(evidence), &inc.Evidence)
		if taskID.Valid {
			inc.TaskID = &taskID.String
		}
		if metaDiag.Valid {
			inc.MetaDiagnosis = metaDiag.String
		}
		json.Unmarshal([]byte(metaRecovery), &inc.MetaRecovery)
		if diagnosedAt.Valid {
			inc.DiagnosedAt = &diagnosedAt.Time
		}
		if proposedAt.Valid {
			inc.ProposedAt = &proposedAt.Time
		}
		if approvedAt.Valid {
			inc.ApprovedAt = &approvedAt.Time
		}
		if recoveredAt.Valid {
			inc.RecoveredAt = &recoveredAt.Time
		}
		if verifiedAt.Valid {
			inc.VerifiedAt = &verifiedAt.Time
		}
		incidents = append(incidents, &inc)
	}
	return incidents, rows.Err()
}

// Approval operations

func (s *Store) CreateApproval(a *state.Approval) error {
	payload, _ := json.Marshal(a.Payload)
	_, err := s.db.Exec(`
		INSERT INTO approvals (id, session_id, task_id, incident_id, type, status, title, description, payload, scope, requested_at, expires_at, responded_at, decision, decided_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.SessionID, a.TaskID, a.IncidentID, a.Type, a.Status, a.Title, a.Description,
		string(payload), a.Scope, a.RequestedAt, a.ExpiresAt, a.RespondedAt, a.Decision, a.DecidedBy)
	return err
}

func (s *Store) UpdateApproval(a *state.Approval) error {
	payload, _ := json.Marshal(a.Payload)
	var decision sql.NullBool
	if a.Decision != nil {
		decision = sql.NullBool{Bool: *a.Decision, Valid: true}
	}
	_, err := s.db.Exec(`
		UPDATE approvals SET status=?, responded_at=?, decision=?, decided_by=?, payload=? WHERE id=?
	`, a.Status, a.RespondedAt, decision, a.DecidedBy, string(payload), a.ID)
	return err
}

func (s *Store) GetApproval(id string) (*state.Approval, error) {
	row := s.db.QueryRow(`
		SELECT id, session_id, task_id, incident_id, type, status, title, description, payload, scope, requested_at, expires_at, responded_at, decision, decided_by
		FROM approvals WHERE id=?
	`, id)

	var a state.Approval
	var payload string
	var taskID, incidentID, decidedBy sql.NullString
	var respondedAt sql.NullTime
	var decision sql.NullBool
	err := row.Scan(&a.ID, &a.SessionID, &taskID, &incidentID, &a.Type, &a.Status, &a.Title, &a.Description,
		&payload, &a.Scope, &a.RequestedAt, &a.ExpiresAt, &respondedAt, &decision, &decidedBy)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(payload), &a.Payload)
	if taskID.Valid {
		a.TaskID = &taskID.String
	}
	if incidentID.Valid {
		a.IncidentID = &incidentID.String
	}
	if respondedAt.Valid {
		a.RespondedAt = &respondedAt.Time
	}
	if decision.Valid {
		a.Decision = &decision.Bool
	}
	if decidedBy.Valid {
		a.DecidedBy = decidedBy.String
	}
	return &a, nil
}

func (s *Store) ListPendingApprovals() ([]*state.Approval, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, task_id, incident_id, type, status, title, description, payload, scope, requested_at, expires_at, responded_at, decision, decided_by
		FROM approvals WHERE status=? AND expires_at > ? ORDER BY requested_at
	`, state.ApprovalStatusPending, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []*state.Approval
	for rows.Next() {
		var a state.Approval
		var payload string
		var taskID, incidentID, decidedBy sql.NullString
		var respondedAt sql.NullTime
		var decision sql.NullBool
		err := rows.Scan(&a.ID, &a.SessionID, &taskID, &incidentID, &a.Type, &a.Status, &a.Title, &a.Description,
			&payload, &a.Scope, &a.RequestedAt, &a.ExpiresAt, &respondedAt, &decision, &decidedBy)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(payload), &a.Payload)
		if taskID.Valid {
			a.TaskID = &taskID.String
		}
		if incidentID.Valid {
			a.IncidentID = &incidentID.String
		}
		if respondedAt.Valid {
			a.RespondedAt = &respondedAt.Time
		}
		if decision.Valid {
			a.Decision = &decision.Bool
		}
		if decidedBy.Valid {
			a.DecidedBy = decidedBy.String
		}
		approvals = append(approvals, &a)
	}
	return approvals, rows.Err()
}

// RecoveryAction operations

func (s *Store) CreateRecoveryAction(ra *state.RecoveryAction) error {
	args, _ := json.Marshal(ra.Args)
	var result []byte
	if ra.Result != nil {
		result, _ = json.Marshal(ra.Result)
	}
	_, err := s.db.Exec(`
		INSERT INTO recovery_actions (id, incident_id, type, description, command, args, status, proposed_at, approved_at, executed_at, result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ra.ID, ra.IncidentID, ra.Type, ra.Description, ra.Command, string(args), ra.Status, ra.ProposedAt, ra.ApprovedAt, ra.ExecutedAt, string(result))
	return err
}

func (s *Store) UpdateRecoveryAction(ra *state.RecoveryAction) error {
	var result []byte
	if ra.Result != nil {
		result, _ = json.Marshal(ra.Result)
	}
	_, err := s.db.Exec(`
		UPDATE recovery_actions SET status=?, approved_at=?, executed_at=?, result=? WHERE id=?
	`, ra.Status, ra.ApprovedAt, ra.ExecutedAt, string(result), ra.ID)
	return err
}

func (s *Store) GetRecoveryAction(id string) (*state.RecoveryAction, error) {
	row := s.db.QueryRow(`
		SELECT id, incident_id, type, description, command, args, status, proposed_at, approved_at, executed_at, result
		FROM recovery_actions WHERE id=?
	`, id)

	var ra state.RecoveryAction
	var args, result string
	var approvedAt, executedAt sql.NullTime
	err := row.Scan(&ra.ID, &ra.IncidentID, &ra.Type, &ra.Description, &ra.Command, &args, &ra.Status,
		&ra.ProposedAt, &approvedAt, &executedAt, &result)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(args), &ra.Args)
	if approvedAt.Valid {
		ra.ApprovedAt = &approvedAt.Time
	}
	if executedAt.Valid {
		ra.ExecutedAt = &executedAt.Time
	}
	if result != "" {
		json.Unmarshal([]byte(result), &ra.Result)
	}
	return &ra, nil
}

func (s *Store) ListRecoveryActionsByIncident(incidentID string) ([]*state.RecoveryAction, error) {
	return s.ListRecoveryActions(incidentID)
}

func (s *Store) ListRecoveryActions(incidentID string) ([]*state.RecoveryAction, error) {
	query := `
		SELECT id, incident_id, type, description, command, args, status, proposed_at, approved_at, executed_at, result
		FROM recovery_actions
	`
	args := []interface{}{}
	if incidentID != "" {
		query += " WHERE incident_id=?"
		args = append(args, incidentID)
	}
	query += " ORDER BY proposed_at"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*state.RecoveryAction
	for rows.Next() {
		var ra state.RecoveryAction
		var argsStr, result string
		var approvedAt, executedAt sql.NullTime
		err := rows.Scan(&ra.ID, &ra.IncidentID, &ra.Type, &ra.Description, &ra.Command, &argsStr, &ra.Status,
			&ra.ProposedAt, &approvedAt, &executedAt, &result)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(argsStr), &ra.Args)
		if approvedAt.Valid {
			ra.ApprovedAt = &approvedAt.Time
		}
		if executedAt.Valid {
			ra.ExecutedAt = &executedAt.Time
		}
		if result != "" {
			json.Unmarshal([]byte(result), &ra.Result)
		}
		actions = append(actions, &ra)
	}
	return actions, rows.Err()
}

// Event operations

func (s *Store) AppendEvent(event *state.Event) error {
	payload, _ := json.Marshal(event.Payload)
	_, err := s.db.Exec(`
		INSERT INTO events (session_id, type, timestamp, payload) VALUES (?, ?, ?, ?)
	`, event.SessionID, event.Type, event.Timestamp, string(payload))
	return err
}

func (s *Store) GetEvents(sessionID string, limit int) ([]*state.Event, error) {
	rows, err := s.db.Query(`
		SELECT session_id, type, timestamp, payload FROM events WHERE session_id=? ORDER BY timestamp DESC LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*state.Event
	for rows.Next() {
		var e state.Event
		var payload string
		err := rows.Scan(&e.SessionID, &e.Type, &e.Timestamp, &payload)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(payload), &e.Payload)
		events = append(events, &e)
	}
	return events, rows.Err()
}
