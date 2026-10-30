# Argus V1 — Hackathon Demo Runbook

## Overview

This runbook documents the exact steps to demonstrate the Argus V1 MVP loop:

1. Start the local Go service (agent-manager)
2. Launch a demo project with a deliberate failure
3. Observe incident detection, diagnosis, and recovery proposal
4. Approve the recovery action
5. Execute recovery and verify it runs in the correct project directory
6. View the dashboard with recovery actions visible

---

## Prerequisites

### Required Software

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.22+ | `go install golang.org/dl/go1.22@latest` |
| OpenCode | Latest | `npm install -g opencode` or build from source |
| Git | Any | System package manager |

### Command‑line Flags (not environment variables)

The `agent-manager` binary reads **flags**, not environment variables. Pass them directly when starting the service.

| Flag | Default | Description |
|------|---------|-------------|
| `-db` | `argus.db` | SQLite database path |
| `-port` | `8080` | HTTP dashboard port |
| `-opencode` | `opencode` | OpenCode binary path (or full path) |
| `-model` | `""` | Default model (provider/model) |
| `-auto` | `false` | Auto‑approve permissions (DANGEROUS – **do not use in demo**) |
| `-meta-api-key` | `""` | Meta API key (enables live Meta diagnosis) |
| `-meta-base-url` | `https://api.meta.example.com/v1` | Meta API base URL |
| `-meta-model` | `meta-llama-3-70b` | Meta model to use |

Example start command (see **Starting the Agent Manager** below).

---

## Building Argus

```bash
cd /home/g/meta/argus-agent-supervisor/services/agent-manager
go build -o agent-manager ./cmd/agent-manager
```

Verify the binary:
```bash
./agent-manager -h
```

---

## Starting the Agent Manager

### Terminal 1 — Start the service

```bash
cd /home/g/meta/argus-agent-supervisor/services/agent-manager

# Use isolated demo database and port 8089
./agent-manager \
  -db /tmp/argus-demo.db \
  -port 8089 \
  -opencode "$(which opencode)" \
  -model ""
```

Expected output:
```
2026/10/30 10:00:00 Starting dashboard on :8089
```

Health check:
```bash
curl http://localhost:8089/health
# {"status":"ok"}
```

---

## Creating a Reproducible Demo Failure

### Terminal 2 — Create isolated demo project

```bash
# Create a clean demo directory
DEMO_PROJ=$(mktemp -d -t argus-demo-XXXXXX)
echo "Demo project: $DEMO_PROJ"

# Initialize a minimal Go module
cat > "$DEMO_PROJ/go.mod" <<'EOF'
module demo

go 1.22
EOF

# Create a test that FAILS deterministically
cat > "$DEMO_PROJ/fail_test.go" <<'EOF'
package demo

import "testing"

func TestAlwaysFails(t *testing.T) {
    t.Fatal("DELIBERATE FAILURE FOR DEMO")
}
EOF

# Verify the test fails
cd "$DEMO_PROJ" && go test -v ./...
# Should show: FAIL  demo  TestAlwaysFails  DELIBERATE FAILURE FOR DEMO
```

---

## Running the Demo Session

### Terminal 2 — Launch session via API

```bash
# Create a session pointing at the demo project
SESSION_RESP=$(curl -s -X POST http://localhost:8089/api/sessions \
  -H "Content-Type: application/json" \
  -d "{\"project_dir\":\"$DEMO_PROJ\",\"model\":\"\",\"prompt\":\"run tests\"}")

echo "$SESSION_RESP" | jq .
SESSION_ID=$(echo "$SESSION_RESP" | jq -r .session.id)
TASK_ID=$(echo "$SESSION_RESP" | jq -r .task.id)

echo "Session: $SESSION_ID"
echo "Task: $TASK_ID"
```

### Terminal 1 — Observe agent output

The agent will run and the test will fail. Watch the dashboard or events:

```bash
# Watch events in real time
curl "http://localhost:8089/api/events?session_id=$SESSION_ID&limit=20" | jq .
```

Wait ~10-15 seconds for the test to run and fail.

---

## Triggering Incident Detection (manual step)

Argus does **not** automatically create incidents from events in the background. After the OpenCode run finishes, invoke the detection endpoint:

```bash
# Ask the manager to scan recent events and create incidents
curl -s -X POST "http://localhost:8089/api/incidents?session_id=$SESSION_ID" \
  -H "Content-Type: application/json" \
  -d '{"action":"detect"}' | jq .
```

Then list incidents:

```bash
curl "http://localhost:8089/api/incidents?session_id=$SESSION_ID" | jq .

# Or via dashboard summary
curl "http://localhost:8089/api/dashboard" | jq .
```

You should see an incident with:
- `type`: "test_failure" (or "compile_error" / "missing_dependency")
- `severity`: "HIGH"
- `status`: "DETECTED"
- `evidence`: Contains the failure output

---

## Triggering Diagnosis

```bash
# Get the incident ID
INCIDENT_ID=$(curl -s "http://localhost:8089/api/incidents?session_id=$SESSION_ID" | jq -r '.[0].id')
echo "Incident: $INCIDENT_ID"

# Request diagnosis
curl -s -X POST http://localhost:8089/api/diagnosis \
  -H "Content-Type: application/json" \
  -d "{\"incident_id\":\"$INCIDENT_ID\"}" | jq .
```

**Important**: By default (no `META_API_KEY`), Argus uses the **deterministic fallback diagnosis**. This produces a `RETRY` recovery action with command `go test ./...`. The live Meta API is **not** used unless `META_API_KEY` is set.

---

## Viewing Proposed Recovery

```bash
# List recovery actions for this incident
curl "http://localhost:8089/api/recovery?incident_id=$INCIDENT_ID" | jq .
```

Expected recovery action:
```json
[
  {
    "id": "...",
    "incident_id": "...",
    "type": "RETRY",
    "description": "Re-run tests after potential flaky failure",
    "command": "go",
    "args": ["test", "./..."],
    "status": "PROPOSED",
    "proposed_at": "..."
  }
]
```

---

## Requesting Recovery Approval

```bash
RECOVERY_ID=$(curl -s "http://localhost:8089/api/recovery?incident_id=$INCIDENT_ID" | jq -r '.[0].id')
echo "Recovery: $RECOVERY_ID"

# Request approval (creates an approval record)
curl -s -X POST http://localhost:8089/api/recovery/approval \
  -H "Content-Type: application/json" \
  -d "{\"recovery_id\":\"$RECOVERY_ID\"}" | jq .
```

Note the `approval.id` from the response.

---

## Granting Approval (Human-in-the-loop)

```bash
APPROVAL_ID=$(curl -s "http://localhost:8089/api/approvals" | jq -r '.[0].id')
echo "Approval: $APPROVAL_ID"

# Approve the recovery
curl -s -X POST "http://localhost:8089/api/approvals/$APPROVAL_ID" \
  -H "Content-Type: application/json" \
  -d '{"approved":true,"decided_by":"demo-user"}' | jq .
```

The recovery action status transitions: `PROPOSED` → `APPROVED`.

---

## Executing Recovery

```bash
# Execute the approved recovery
curl -s -X POST "http://localhost:8089/api/recovery/$RECOVERY_ID" | jq .
```

This runs `go test ./...` **in the demo project directory** (`$DEMO_PROJ`), not the agent-manager directory. The fix in `manager.go:ExecuteRecovery` sets `cmd.Dir = sess.ProjectDir`.

Expected result:
```json
{
  "status": "PASSED",
  "command": "go test ./...",
  "exit_code": 0,
  "stdout": "ok  demo  0.001s\n",
  "duration_ms": ...,
  "integrity_ok": true,
  "notes": "Verification passed"
}
```

The incident status becomes `VERIFIED`.

---

## Verifying Correct Working Directory

The test file `fail_test.go` was **not** modified. The recovery re-ran the **same failing test**. How did it pass?

Because the deterministic fallback proposes `go test ./...` which runs all tests. In the demo, we only created one test file that fails. But wait — the demo test *still fails*!

**Correction**: The test *will fail again*. The demo shows the **mechanism works** — the command executes in the correct directory. The verification result will show `FAILED` with the test output.

To show a *passing* recovery, you would need a test that passes on retry (e.g., flaky test simulation). For this demo, the key observation is:

> The recovery command executes in `$DEMO_PROJ` (the session's project directory), proven by the test output referencing files in that directory.

```bash
# Check final incident status
curl "http://localhost:8089/api/incidents?session_id=$SESSION_ID" | jq .
```

---

## Viewing the Dashboard

Open in browser: **http://localhost:8089**

The dashboard table includes a **Recovery** column showing:
- Recovery type (RETRY, RESTART, etc.)
- Status badge (PROPOSED, APPROVED, RECOVERING, VERIFIED, FAILED)
- Description
- Command + args (e.g., `go test ./...`)
- Truncated incident ID

The dashboard auto-refreshes every 5 seconds (`setInterval(render, 5000)`).

---

## Demonstrating a Successful Verification (Manual Preparation + Recovery)

The deterministic fallback for `test_failure` proposes `go test ./...`, which will fail again on the same broken test. To show a **passing verification** without pretending Argus autonomously repairs code, use a *missing‑dependency* incident where the fix is a manual `go mod tidy` performed by the developer **before** the recovery runs.

### 1. Create a project with a missing import

```bash
DEMO_PROJ2=$(mktemp -d -t argus-demo2-XXXXXX)
cat > "$DEMO_PROJ2/go.mod" <<'EOF'
module demo2
go 1.22
EOF

cat > "$DEMO_PROJ2/main.go" <<'EOF'
package main

import _ "github.com/nonexistent/pkg"   // missing on purpose

func main() {}
EOF
```

### 2. Launch a session

```bash
SESSION_RESP2=$(curl -s -X POST http://localhost:8089/api/sessions \
  -H "Content-Type: application/json" \
  -d "{\"project_dir\":\"$DEMO_PROJ2\",\"model\":\"\",\"prompt\":\"build\"}")

SESSION_ID2=$(echo "$SESSION_RESP2" | jq -r .session.id)
echo "Session 2: $SESSION_ID2"
```

Wait for the OpenCode run to finish (it will error on the missing import).

### 3. Detect the incident

```bash
curl -s -X POST "http://localhost:8089/api/incidents?session_id=$SESSION_ID2" \
  -H "Content-Type: application/json" \
  -d '{"action":"detect"}' | jq .

INCIDENT_ID2=$(curl -s "http://localhost:8089/api/incidents?session_id=$SESSION_ID2" | jq -r '.[0].id')
echo "Incident 2: $INCIDENT_ID2"
```

You should see `type: "missing_dependency"`.

### 4. Diagnose (fallback proposes `go mod tidy`)

```bash
curl -s -X POST http://localhost:8089/api/diagnosis \
  -H "Content-Type: application/json" \
  -d "{\"incident_id\":\"$INCIDENT_ID2\"}" | jq .
```

### 5. **Manual preparation** – developer fixes the environment

```bash
cd "$DEMO_PROJ2" && go mod tidy   # downloads the (non‑existent) module → will still fail, but demonstrates the step
# For a truly passing demo, replace the import with a real one, e.g. "github.com/google/uuid"
# and run `go mod tidy` again. Then the subsequent `go mod tidy` in recovery will succeed.
```

*Important*: This manual step is **outside Argus**. It shows the human‑in‑the‑loop preparation that makes the subsequent verification pass.

### 6. Request & grant approval

```bash
RECOVERY_ID2=$(curl -s "http://localhost:8089/api/recovery?incident_id=$INCIDENT_ID2" | jq -r '.[0].id')
curl -s -X POST http://localhost:8089/api/recovery/approval \
  -H "Content-Type: application/json" \
  -d "{\"recovery_id\":\"$RECOVERY_ID2\"}" | jq .

APPROVAL_ID2=$(curl -s "http://localhost:8089/api/approvals" | jq -r '.[0].id')
curl -s -X POST "http://localhost:8089/api/approvals/$APPROVAL_ID2" \
  -H "Content-Type: application/json" \
  -d '{"approved":true,"decided_by":"demo-user"}' | jq .
```

### 7. Execute recovery (runs `go mod tidy` in the **project directory**)

```bash
curl -s -X POST "http://localhost:8089/api/recovery/$RECOVERY_ID2" | jq .
```

If the manual `go mod tidy` succeeded, the verification result will be:

```json
{
  "status": "PASSED",
  "command": "go mod tidy",
  "exit_code": 0,
  "stdout": "",
  "integrity_ok": true,
  "notes": "Verification passed"
}
```

The incident status becomes `VERIFIED`. This demonstrates that the recovery command executed in the correct project directory (thanks to the `cmd.Dir = sess.ProjectDir` fix) and that verification reflects the *actual* result of the allow‑listed command.

---

## Resetting the Demo Environment

```bash
# Stop the agent manager (Ctrl+C in Terminal 1)

# Clean up demo databases
rm -f /tmp/argus-demo.db /tmp/argus-demo.db-shm /tmp/argus-demo.db-wal

# Clean up demo projects
rm -rf "$DEMO_PROJ" "$DEMO_PROJ2"
```

---

## OpenCode Integration — Limitations & Known Unsupported

| Capability | Status | Notes |
|------------|--------|-------|
| `opencode run --format=json` | ✅ Works | Core execution path |
| Structured event stream (step_start, tool_use, step_finish, text) | ✅ Works | Parsed and persisted |
| `--auto` flag | ⚠️ Exists but DISABLED by default | Security risk: auto-approves all permissions |
| `opencode serve` | ❌ Unstable | Crashes after 2-5 requests |
| SSE event streaming (`/api/event/subscribe`) | ❌ Broken | Returns HTML |
| Programmatic permission response via API | ❌ Not available | No API to approve/deny prompts |
| Session fork/continue via CLI | ❌ Not supported | Requires server attachment |
| Native permission pausing in OpenCode | ❌ Not supported | Argus manages approvals externally |

**Argus does not claim**: native permission pausing, parked approvals inside OpenCode, or reliable session resume. The integration is a **CLI wrapper** with deterministic fallback diagnosis.

---

## Meta API — Live vs Fallback

| Mode | Condition | Behavior |
|------|-----------|----------|
| **Live Meta** | `META_API_KEY` set & valid | Calls Meta API for diagnosis + recovery plan |
| **Fallback** | `META_API_KEY` empty (default) | Deterministic: incident type → fixed recovery action |

Fallback mapping (in `meta.go`):
- `test_failure` → RETRY with `go test ./...`
- `compile_error` → RETRY with `go build ./...`
- `missing_dependency` → RETRY with `go mod tidy`

The demo uses **fallback mode** by design — no external API dependency.

---

## Safety Checklist for Demo

- [ ] Using isolated temp directory (`mktemp -d`)
- [ ] Using isolated SQLite database (`/tmp/argus-demo.db`)
- [ ] `--auto` flag **NOT** used
- [ ] No manual approval bypass
- [ ] Recovery command is allowlisted (`go test ./...`)
- [ ] Approval required before execution
- [ ] Approval bound to exact recovery action ID
- [ ] Approval expires after 10 minutes
- [ ] Verification command re-checked for integrity after execution

---

## Troubleshooting

| Issue | Resolution |
|-------|------------|
| `opencode: command not found` | Set `OPENCODE_BIN` to full path or install globally |
| Dashboard shows no sessions | Check `SESSION_ID` from POST response; verify project dir exists |
| Incident not detected | Wait for OpenCode to finish; check events API for `tool_use` with `exit: 1` |
| Recovery not proposed | Diagnosis must complete first; check incident status is `PROPOSED` |
| Approval fails | Ensure approval status is `PENDING` and not `EXPIRED` |
| Recovery executes in wrong dir | Verify `cmd.Dir = sess.ProjectDir` in `manager.go:ExecuteRecovery` |

---

## Files Referenced

| File | Purpose |
|------|---------|
| `services/agent-manager/cmd/agent-manager/main.go` | Service entry point, flags |
| `services/agent-manager/internal/session/manager.go` | Core logic: diagnosis, approval, recovery execution |
| `services/agent-manager/internal/dashboard/dashboard.go` | HTTP API endpoints |
| `services/agent-manager/internal/dashboard/static/index.html` | Dashboard UI (renders recovery column) |
| `services/agent-manager/internal/meta/meta.go` | Meta client + deterministic fallback |
| `services/agent-manager/internal/subprocess/subprocess.go` | OpenCode subprocess management |
| `services/agent-manager/internal/session/recovery_workdir_test.go` | Regression test proving correct working directory |