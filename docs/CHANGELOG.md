# Changelog

All notable changes to Argus will be documented in this file.

## [Unreleased] - 2026-10-29

### P0 - Go Control Plane (Foundation)

#### Added
- **State Models** (`internal/state/state.go`): Complete domain models for Session, Task, Incident, Approval, RecoveryAction, VerificationResult with all status enums
- **SQLite Persistence** (`internal/store/store.go`): Full CRUD for all entities with foreign keys, indexes, WAL mode
- **OpenCode Subprocess Manager** (`internal/subprocess/subprocess.go`): Manages `opencode run --format=json` subprocesses, parses JSON-lines events, persists to store
- **Controlled Test Agent** (`internal/subprocess/subprocess.go`): Reproducible test scenarios (compile error, test failure, missing dependency, success)
- **Session Manager** (`internal/session/manager.go`): Coordinates sessions, tasks, incidents, approvals, recovery with task queue and approval workers
- **HTTP Dashboard API** (`internal/dashboard/dashboard.go`): REST endpoints for sessions, tasks, incidents, approvals, events, recovery
- **Main Entry Point** (`cmd/agent-manager/main.go`): Wires all components, handles graceful shutdown

#### Fixed
- Subprocess context cancellation bug: was using HTTP request context instead of background context, causing immediate process termination

#### Verified
- Go build: PASS
- All unit tests: PASS (state, store, subprocess, session packages)
- End-to-end: Session creation → OpenCode execution → event parsing → persistence → API query

### P1 - Safe Approvals & Independent Continuation

#### Added
- **Approval Workflow**: Bound to exact request ID, action, scope, expiry; duplicate detection; stale request rejection
- **Approval States**: PENDING → APPROVED/REJECTED/EXPIRED with task status transitions (PARKED → RUNNING/FAILED)
- **Deterministic Independence Checks**: Different scope = independent; same scope = not independent (model never decides)
- **Incident Detection**: Analyzes event stream for compile errors, test failures, missing dependencies with severity classification
- **API Endpoints**: 
  - `POST /api/incidents?action=detect` - trigger incident detection
  - `POST /api/approvals` - create approval (with duplicate check)
  - `GET /api/approvals` - list pending
  - `POST /api/approvals/{id}` - respond (approve/reject)

#### Security
- `--auto` flag defaults to `false`, only enabled explicitly on CLI
- No automatic permission bypass in default mode
- All approvals require explicit human decision

#### Tests
- `TestManagerApprovalWorkflow` - full approve flow with duplicate rejection
- `TestManagerApprovalRejection` - reject flow with task FAILED status
- `TestManagerApprovalExpiry` - expired approval cannot be approved
- `TestManagerIncidentDetection` - detects 3 incident types from events
- `TestManagerIndependentContinuation` - scope-based independence logic

### Integration Verified

```
# Start server
./agent-manager --port=9006

# Create session
curl -X POST localhost:9006/api/sessions -d '{"project_dir":"/tmp/test-project","prompt":"echo test"}'

# Result: Session COMPLETED, exit_code=0, 8 events persisted
# Dashboard shows events: step_start, tool_use(bash), step_finish, text, step_finish(reason=stop)

# Create approval
curl -X POST localhost:9006/api/approvals -d '{"session_id":"...","task_id":"...","type":"PERMISSION",...}'

# Approve
curl -X POST localhost:9006/api/approvals/{id} -d '{"approved":true}'

# Result: Task status RUNNING, pending approvals cleared

# Detect incidents
curl -X POST "localhost:9006/api/incidents?session_id=..." -d '{"action":"detect"}'

# Result: Incidents created for failure patterns
```

### P2 - Detect, Diagnose, Recover, Verify

#### Added
- **Meta Model API Client** (`internal/meta/meta.go`): HTTP client for Meta diagnosis with deterministic fallback
- **Incident Diagnosis** (`internal/session/manager.go:DiagnoseIncident`): Sends redacted evidence to Meta, stores diagnosis and recovery plan
- **Recovery Proposal**: Automatic creation of RecoveryAction from Meta/fallback diagnosis
- **Recovery Approval Workflow**: `RequestRecoveryApproval` creates approval bound to incident/task/action/scope/expiry
- **Safe Recovery Execution**: `ExecuteRecovery` runs only allowlisted commands (go build/test/mod, echo) with integrity checks
- **Verification with Integrity**: Captures exit code, output, duration; validates command allowlist; marks UNVERIFIED if integrity fails
- **Complete State Transitions**: DETECTED → DIAGNOSING → PROPOSED → APPROVED → RECOVERING → VERIFIED/FAILED/REJECTED
- **API Endpoints**:
  - `POST /api/diagnosis` - trigger Meta diagnosis for incident
  - `POST /api/recovery/approval` - request approval for recovery action
  - `POST /api/recovery/{id}` - execute approved recovery

#### Security
- Meta API disabled by default (requires `--meta-api-key` flag)
- Fallback deterministic diagnosis used when Meta unavailable
- Recovery commands restricted to allowlist: `go build|test|mod|vet`, `echo`
- No arbitrary shell command execution from model output
- Every recovery requires explicit human approval via approval queue
- Approval binding: exact incident ID, task ID, action type, scope, 10-min expiry
- Duplicate/stale approval detection and rejection

#### Tests
- `TestRecoveryWorkflow` - full recovery loop: detect → diagnose → propose → request approval → reject (verify blocked) → approve → execute → verify
- Verifies rejection path blocks execution
- Verifies approval enables execution
- Verifies integrity checks and verification statuses

#### Integration Verified

```
# Start server (Meta disabled, uses fallback)
./agent-manager --port=9010

# Create session that will succeed
curl -X POST localhost:9010/api/sessions -d '{"project_dir":"/tmp/test-project","prompt":"echo test"}'

# Wait for completion, then create incident from events
curl -X POST "localhost:9010/api/incidents?session_id=..." -d '{"action":"detect"}'

# Diagnose incident (uses fallback)
curl -X POST localhost:9010/api/diagnosis -d '{"incident_id":"..."}'

# Request recovery approval
curl -X POST localhost:9010/api/recovery/approval -d '{"recovery_id":"..."}'

# Approve recovery
curl -X POST localhost:9010/api/approvals/{approval_id} -d '{"approved":true,"decided_by":"user"}'

# Execute recovery
curl -X POST localhost:9010/api/recovery/{recovery_id}

# Result: VerificationResult with status PASSED/FAILED/UNVERIFIED, integrity check, incident VERIFIED/FAILED
```

### All Tests Pass

```
go test ./...
ok  internal/state
ok  internal/store
ok  internal/subprocess
ok  internal/session
```