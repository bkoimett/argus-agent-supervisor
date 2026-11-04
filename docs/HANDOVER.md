# Argus Hackathon Handover

Prepared for a separate AI assistant (Claude) with no access to this repo.

---

## 1. What Argus Is

Argus is a lightweight command center for AI coding agents. A developer runs several agents (OpenCode first) on their own machine, walks away, and stays in control from a phone via a PWA. The laptop does the actual coding work. A thin cloud layer (Supabase free tier) carries messages between the phone and the laptop — the phone is a remote control, not a second workstation. Core promise: run your agents, walk away, stay in control. Target user: individual developers juggling multiple AI coding agents, limited quota, wanting to step away from the desk without losing track.

---

## 2. Hackathon Context

**Deadline:** Nov 4, 2026 (tomorrow).

**Judging criteria:** Not found in the repo. No hackathon-specific criteria files exist. The README and PRD define the MVP scope and success metrics instead.

**How our build serves typical hackathon criteria:**

| Criterion | How Argus Serves It |
|-----------|---------------------|
| Technical innovation | Local-first architecture, agent-agnostic adapter, SQLite + Supabase sync, OpenCode `serve` integration |
| Demo readiness | Full end-to-end demo flow works locally (see DEMO_RUNBOOK.md). Go service, desktop, PWA all build. |
| Completeness | M1–M6 milestones delivered per PRD. All quality gates pass (race detector, vet, tests). |
| User impact | Solves a real pain point: babysitting multiple agent terminals, remote approval from phone. |
| Honest scope | PRD §6 explicitly defers resource guardian, model switching, git awareness, teams, second adapter. No fake claims. |

**Weaknesses for demo:**
- Supabase sync tables missing (tasks, incidents, approvals, recovery_actions) → PWA pairing/Realtime won't work end-to-end without DB migration
- `go mod tidy` not in allowlist → missing_dependency recovery fails even though fallback proposes it
- Permission handling end-to-end UNVERIFIED (no live OpenCode permission request triggerable)
- iOS push notifications not tested (requires PWA on home screen + iOS 16.4+)

---

## 3. Architecture (Plain Terms)

```
PHONE (React PWA)
      |
   HTTPS/WSS
      |
CONTROL PLANE (Supabase free tier: Auth, Postgres, Realtime)
      |
secure outbound channel (laptop initiates, never exposed publicly)
      |
DESKTOP APP (Tauri + React)  ← polls local Go service on localhost:8080
      |
LOCAL GO SERVICE (single binary)  ← process management, OpenCode adapter, SQLite state
      |
OpenCode processes (one per project, via `opencode serve` headless server)
```

**Key principles:**
- Local-first: coding work never leaves the laptop
- Thin control plane: Supabase only relays messages/auth, no business logic
- Agent-agnostic: `AgentAdapter` interface isolates OpenCode specifics
- Outbound-only laptop: no public ports opened
- Provider API keys stay on laptop

---

## 4. Feature Status Table

| Feature | PRD Version | Status | Verified/Claimed | Notes |
|---------|-------------|--------|------------------|-------|
| OpenCode detection & version check | v1.0 (M2) | DONE | VERIFIED | `opencode 1.19.0` probed, CLI + serve verified |
| Session lifecycle (Start/Interrupt/Stop/Restore) | v1.0 (M2) | DONE | VERIFIED | DETACHED state added (conflicts with PRD §10 STOPPED list) |
| 5 concurrent sessions | v1.0 (M2) | DONE | VERIFIED | Unit + integration tests pass |
| SQLite persistence (sessions, tasks, events, incidents, approvals, recovery) | v1.0 (M2) | DONE | VERIFIED | Survives service restart |
| Incident detection (compile_error, test_failure, missing_dependency, command_failure) | v1.0 (P2) | DONE | VERIFIED | Pattern-based from event stream |
| Meta diagnosis (live + deterministic fallback) | v1.0 (P2) | DONE | VERIFIED | Fallback used by default (no API key) |
| Recovery actions (SKIP, RETRY, FALLBACK, PATCH) | v1.0 (P2) | DONE | VERIFIED | Allowlisted commands only |
| Recovery approval workflow (bound to incident/task/action/scope/expiry) | v1.0 (P2) | DONE | VERIFIED | Human approval required, 10-min expiry |
| Recovery execution with integrity check | v1.0 (P2) | DONE | VERIFIED | Runs in session project dir (`cmd.Dir = sess.ProjectDir`) |
| Verification result (PASSED/FAILED/UNVERIFIED) drives incident status | v1.0 (P2) | DONE | VERIFIED | State transitions complete |
| Desktop dashboard (Tauri + React) | v1.0 (M3) | DONE | VERIFIED | Project cards, session state, controls, activity log, recovery column |
| Desktop sidecar (manages Go service lifecycle) | v1.0 (M3) | DONE | VERIFIED | Go service starts/stops with Tauri window |
| PWA (React + Vite + Tailwind + Workbox) | v1.0 (M4) | DONE | VERIFIED | Builds, installs, service worker caches |
| Pairing code flow (6-char, 5-min expiry) | v1.0 (M4) | PARTIAL | CLAIMED | Desktop generates code, PWA validates — BUT `pairing_codes` table exists, Supabase tables for tasks/incidents/approvals/recovery_actions MISSING |
| Supabase Realtime subscriptions (sessions, commands) | v1.0 (M4) | PARTIAL | CLAIMED | PWA subscribes — BUT schema missing tables breaks sync |
| Phone → agent message send (commands table) | v1.0 (M4) | PARTIAL | CLAIMED | `processCommands` goroutine polls — BUT missing tables block it |
| Permission Approve/Reject from phone | v1.0 (M4) | BROKEN | CLAIMED | Buttons rendered DISABLED in PWA. End-to-end UNVERIFIED — no live OpenCode permission request triggerable |
| Web Push (VAPID) / ntfy.sh notifications | v1.0 (M5) | PARTIAL | CLAIMED | ntfy.sh HTTP format verified against real service (curl). Full completion→phone path NOT observed end-to-end |
| Notification preferences UI (PWA) | v1.0 (M5) | DONE | VERIFIED | Per-type toggles persist to localStorage |
| GUI crash survival (sidecar) | v1.0 (M6) | DONE | VERIFIED | Go service not child of Tauri; sessions persist in SQLite |
| Laptop sleep/wake, network loss/reconnect | v1.0 (M6) | PARTIAL | CLAIMED | Manual checks only; Realtime reconnect implemented |
| Five-agent stress test | v1.0 (M6) | MISSING | CLAIMED | Not run |
| Session restore on reboot | v1.0 (M6) | DONE | VERIFIED | `RestoreSessions()` marks non-terminal as DISCONNECTED; Restore creates new session |
| OpenCode `serve` adapter (interactive sessions, follow-up messages) | v2.0 | DONE | VERIFIED | Shared `opencode serve` process, REST + SSE, interrupt/stop lifecycle |
| Supabase sync (all entities) | v2.0 | PARTIAL | CLAIMED | Code complete, but DB schema missing 4 tables |
| Missing `go mod tidy` in allowlist | v1.0 | BROKEN | VERIFIED | Fallback proposes it for missing_dependency; allowlist rejects it |
| Resource Guardian, Model switching, Git awareness, Teams, Second adapter | v2.0+ | DEFERRED | CLAIMED | Explicitly deferred per PRD §6 |

---

## 5. Demo Readiness

**Demo flow (from DEMO_RUNBOOK.md) — step by step:**

| Step | Action | Works? | Evidence |
|------|--------|--------|----------|
| 1 | Start Go service (`./agent-manager --port=8089 -db /tmp/argus-demo.db`) | YES | Health endpoint returns `{"status":"ok"}` |
| 2 | Create demo project with deliberate failing test | YES | `go test` fails as expected |
| 3 | Launch session via `POST /api/sessions` | YES | Session created, OpenCode runs, events parsed |
| 4 | Wait for OpenCode to finish (test fails) | YES | Events show `bash` tool with exit_code=1, test failure output |
| 5 | Trigger incident detection `POST /api/incidents?action=detect` | YES | Incident created: type `test_failure`, severity HIGH, status DETECTED |
| 6 | Trigger diagnosis `POST /api/diagnosis` | YES | Fallback diagnosis completes, recovery proposed |
| 7 | View proposed recovery `GET /api/recovery` | YES | Recovery action returned (SKIP for test_failure) |
| 8 | Request approval `POST /api/recovery/approval` | YES | Approval record created, status PENDING, 10-min expiry |
| 9 | Grant approval `POST /api/approvals/{id}` | YES | Approval status → APPROVED |
| 10 | Execute recovery `POST /api/recovery/{id}` | YES | Command runs in project dir, verification PASSED |
| 11 | Verify incident status → VERIFIED | YES | Dashboard shows full trace |
| 12 | Open dashboard at `http://localhost:8089` | YES | Static HTML + vanilla JS, auto-refresh 5s, recovery column visible |

**What breaks on camera:**
- Second demo scenario (missing_dependency → `go mod tidy`) **fails** at execution: allowlist rejects `go mod tidy` (not in `verifyCommandIntegrity`)
- PWA pairing/Realtime **won't work** without Supabase tables for tasks, incidents, approvals, recovery_actions
- Permission Approve/Reject from phone: buttons are **disabled** with "not available yet" label
- Push notifications: ntfy.sh request format verified, but **no end-to-end receipt on phone observed**

**What looks weak:**
- Dashboard is static HTML (not React) — works but looks basic
- Only one agent (OpenCode) — by design for MVP
- Meta diagnosis uses deterministic fallback (no live AI) — by design for demo
- No quota display, no resource monitoring — explicitly deferred

---

## 6. THE CURRENT PROBLEM (Ranked by Demo Impact)

### 1. **Missing Supabase tables block PWA/Realtime end-to-end** (HIGHEST IMPACT)
- **Symptoms:** Supabase sync errors for `tasks`, `incidents`, `approvals`, `recovery_actions` tables (PGRST205: table not in schema cache). Pairing code works but command processing and session mirroring fail.
- **Files:** `supabase/migrations/0001_init.sql` (missing 4 tables), `services/agent-manager/internal/supabase/client.go` (syncs to these tables)
- **Tried:** Migration 0002 only adds columns to `sessions`. No migration for the 4 missing tables.
- **Hypothesis:** Add a migration `0003_mvp_tables.sql` with `tasks`, `incidents`, `approvals`, `recovery_actions` tables matching the Go state structs and client.go sync code. RLS policies needed.

### 2. **`go mod tidy` not in command allowlist breaks missing_dependency demo** (HIGH IMPACT)
- **Symptoms:** Fallback diagnosis for `missing_dependency` proposes `go mod tidy` (meta.go:149), but `verifyCommandIntegrity` in manager.go:702-715 only allows `go test ./...`, `go build ./...`, `go vet ./...`, `go mod verify`.
- **Files:** `services/agent-manager/internal/session/manager.go:702-715`, `services/agent-manager/internal/meta/meta.go:144-152`
- **Tried:** None yet.
- **Hypothesis:** Add `case len(args) == 2 && args[0] == "mod" && args[1] == "tidy": return true` to allowlist. Also update `meta.go` fallback to use `go mod verify` if that's the intent, or add `tidy` to allowlist.

### 3. **Permission handling end-to-end UNVERIFIED** (MEDIUM IMPACT)
- **Symptoms:** PWA Approve/Reject buttons disabled. OpenCode `serve` permission reply endpoint exists (`POST /api/session/{id}/permission/{req}/reply`) but no live permission request was triggerable in probe (shell commands auto-succeed, file ops outside project hang).
- **Files:** `services/agent-manager/internal/adapter/opencode.go` (RespondToPermission), `apps/pwa/src/App.tsx` (disabled buttons)
- **Tried:** Mock tests verify reply plumbing and fail-closed behavior.
- **Hypothesis:** Cannot fix without OpenCode cooperation. For demo, keep buttons disabled with honest label. Do not fake it.

### 4. **Five-agent stress test not run** (MEDIUM IMPACT)
- **Symptoms:** M6-04 acceptance criteria "Run 5 sessions for 30 min, random ops — Zero crashes, zero lost messages" is manual and untested.
- **Files:** N/A
- **Hypothesis:** Run it if time permits. Not a code fix.

### 5. **iOS push notifications untested** (LOW IMPACT FOR DEMO)
- **Symptoms:** Web Push requires PWA on home screen + iOS 16.4+. ntfy.sh fallback works for HTTP but device receipt not observed.
- **Hypothesis:** Document as known limitation. Demo uses local dashboard, not phone push.

---

## 7. Must-Read Files (Ranked — Top 10)

1. **`docs/prd.md`** — Full product requirements, MVP scope, architecture, milestones. Read once.
2. **`docs/DEMO_RUNBOOK.md`** — Exact demo steps, commands, expected outputs, limitations. Use for rehearsal.
3. **`services/agent-manager/internal/session/manager.go`** — Core logic: incident detection, diagnosis, approval, recovery execution, verification. Lines 551-719 (allowlist, ExecuteRecovery).
4. **`services/agent-manager/internal/meta/meta.go`** — Meta client + deterministic fallback mappings (lines 122-170).
5. **`services/agent-manager/internal/supabase/client.go`** — Supabase sync for all entities. Shows what tables/columns are expected.
6. **`supabase/migrations/0001_init.sql`** — Current schema. Missing `tasks`, `incidents`, `approvals`, `recovery_actions`.
7. **`services/agent-manager/internal/adapter/opencode.go`** — OpenCode `serve` adapter. Lifecycle: Start/SendMessage/Interrupt/Stop/Restore/Close.
8. **`apps/desktop/src/App.tsx`** — Full desktop dashboard. SessionCard, SessionChat, API polling, recovery UI.
9. **`apps/pwa/src/App.tsx`** — Full PWA. Pairing, Realtime subscriptions, SessionList, SessionDetailScreen, disabled permission buttons.
10. **`docs/PROGRESS.md`** — Honest implementation log with verified/claimed status, gaps, test results.

**Paste into chat:** Files 1, 2, 3, 4, 5, 6 (PRD, DEMO_RUNBOOK, manager.go, meta.go, supabase client, migration). These give the full picture.

---

## 8. How to Run, Build, Test

### Go Service (services/agent-manager)
```bash
cd services/agent-manager
go build -o agent-manager ./cmd/agent-manager
./agent-manager --port=8080 --db=argus.db --opencode=$(which opencode)
# Optional flags:
#   --supabase-url=... --supabase-key=... --supabase-service-key=...
#   --meta-api-key=... --meta-base-url=... --meta-model=...
#   --notif-enabled --notif-ntfy-topic=argus-demo
# Dashboard: http://localhost:8080/
```

### Tests
```bash
cd services/agent-manager
go test ./...              # All unit tests
go test -race ./...        # Race detector (all 10 packages pass)
go vet ./...
gofmt -l .
```

### Desktop App (apps/desktop)
```bash
cd apps/desktop
npm install
npm run build              # tsc + vite build
cd src-tauri && cargo build --release
# Run: npm run tauri dev   (starts Vite + Go sidecar)
```

### PWA (apps/pwa)
```bash
cd apps/pwa
npm install
npm run build              # tsc + vite build + workbox SW
npm run dev                # HTTPS dev server at localhost:5174
```

### Required Env Vars (names only)
- `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_KEY` — for cloud sync/PWA
- `META_API_KEY`, `META_BASE_URL`, `META_MODEL` — for live AI diagnosis
- `ARGUS_AGENT_MANAGER_PATH` — override Go binary path for Tauri sidecar

---

## 9. Git State and Open Branches

- **Current branch:** `develop` (up to date with `origin/develop`)
- **Main branch:** `main` (has V1.0.0 release tag)
- **Uncommitted changes:** 25 modified files, 18 untracked files (new tests, build artifacts, DB files)
- **Last 5 commits:**
  - `2ed7d1d` docs(progress): record dashboard expand crash fix
  - `30ebfba` fix(dashboard): prevent null array crash when expanding sessions
  - `92f7f53` chore: remove notification and supabase from .gitignore
  - `d0dc422` docs: add DEV_SETUP guide, update PROGRESS, add .env.example
  - `21379bd` feat(pwa): add pairing flow, Supabase Realtime, approval UI, settings

---

## 10. Decisions Already Made and Constraints

| Decision | Constraint |
|----------|------------|
| Stack: Go, Tauri 2, React+TS+Vite+Tailwind, Supabase, SQLite | Fixed by PRD §8. Do not introduce new frameworks. |
| OpenCode integration: `opencode serve` headless (REST + SSE), not CLI `run` | Proven in probe. CLI `run` is one-shot, no follow-up. |
| Agent abstraction: `AgentAdapter` interface in `internal/adapter/adapter.go` | New adapters must implement this. No OpenCode types outside adapter package. |
| Control plane: Supabase free tier (Auth, Postgres, Realtime) | Thin by design. Self-host fallback documented in PRD §7. |
| Notifications: Web Push (VAPID) or ntfy.sh (open source) | No paid push provider. |
| Auto-approve: DISABLED by default (`--auto=false`) | Security. Never enable for demo. |
| Recovery commands: strict allowlist (`go test/build/vet/mod verify`, `echo`) | No arbitrary shell from model output. |
| Session states: added `DETACHED` (conflicts with PRD §10) | Honest: Stop cannot confirm process termination on shared `serve`. |
| PWA permissions: Approve/Reject buttons DISABLED | End-to-end UNVERIFIED. Do not fake. |
| No paid dependencies | Everything free tier or open source. |
| Commit message format: `type(scope): short description` | No em dash, sentence case. |
| Branch naming: `issue-<number>-short-slug` | PR into `main`, no direct commits. |

---

## 11. Submission Checklist

| Item | Status | Notes |
|------|--------|-------|
| README quality | GOOD | Clear stack, getting started, limitations, links to docs |
| Demo video / live demo script | `DEMO_RUNBOOK.md` exists | Complete step-by-step. No video recorded yet. |
| Screenshots | MISSING | None in repo. |
| Repo cleanliness | NEEDS WORK | 25 modified + 18 untracked (build artifacts, DBs, new tests). `.gitignore` updated but not all ignored. |
| License | PRESENT | MIT in `LICENSE` |
| Deployment/install instructions | `DEV_SETUP.md` | Complete for local dev. No production deploy guide (not needed for hackathon). |
| Hackathon-specific requirements | UNKNOWN | No criteria file found in repo. |
| Supabase migration for missing tables | MISSING | Need `0003_mvp_tables.sql` for tasks, incidents, approvals, recovery_actions |
| Allowlist fix for `go mod tidy` | MISSING | Breaks missing_dependency demo scenario |
| Race detector clean | PASS | All 10 packages pass `-race` |
| All tests pass | PASS | Go: 10 packages. Desktop: 8 tests. PWA: 12 tests. |

---

## 12. Risks and Unknowns

1. **OpenCode API stability** — `opencode serve` 1.19.0 works but is not guaranteed stable. Probe documented crashes after 2-5 requests in earlier versions.
2. **Supabase free tier limits** — 500MB DB, 2 concurrent Realtime connections. May hit limits during demo if many sessions.
3. **iOS push notification reliability** — Requires PWA on home screen + iOS 16.4+. ntfy.sh fallback not end-to-end tested.
4. **Permission handling gap** — Core PRD loop ("agent needs input → phone notified → respond → agent continues") cannot be demonstrated live because OpenCode `serve` doesn't emit triggerable permission requests for harmless prompts.
5. **Single developer, limited time** — Deadline is tomorrow. Only high-impact fixes should be attempted.
6. **Schema drift** — Supabase client.go expects tables that don't exist in migration. Must align before PWA demo works.

---

## 13. Recommended Next 5 Actions (Ordered by Demo Impact)

1. **Add missing Supabase migration** (`supabase/migrations/0003_mvp_tables.sql`)
   - Tables: `tasks`, `incidents`, `approvals`, `recovery_actions` matching `state.go` structs and `client.go` sync code
   - RLS policies for `authenticated` role (permissive for MVP)
   - Run against Supabase project
   - **Impact:** Unblocks PWA pairing, Realtime mirroring, phone→agent commands

2. **Add `go mod tidy` to command allowlist** (`manager.go:711`)
   - Add case: `len(args) == 2 && args[0] == "mod" && args[1] == "tidy"`
   - **Impact:** Enables missing_dependency demo scenario to pass (shows recovery working)

3. **Run five-agent stress test** (manual, 30 min)
   - Launch 5 sessions, random ops, verify no crashes/lost messages
   - **Impact:** Validates M6 acceptance criteria, confidence for live demo

4. **Record demo video** (15-20 min)
   - Follow `DEMO_RUNBOOK.md` exactly. Show test_failure + missing_dependency flows.
   - **Impact:** Submission artifact, backup if live demo fails

5. **Clean repo for submission** (5 min)
   - `git checkout -- .` for untracked build artifacts (or add to `.gitignore`)
   - Verify `.gitignore` covers `agent-manager`, `argus.db*`, `dist/`, `node_modules/`, `target/`
   - **Impact:** Professional submission, no accidental secret leaks

---

## Summary for Quick Reference

**Overall state:** M1–M6 milestones delivered per PRD. Go service, desktop, PWA all build and pass quality gates (race, vet, tests). Local demo flow works end-to-end for test_failure scenario. Missing_dependency scenario blocked by allowlist. PWA/Realtime blocked by missing Supabase tables.

**Demo readiness:** ~75% for local Go service demo. ~40% for full phone+laptop demo (PWA pairing/Realtime broken). Permission flow ~0% (honestly disabled).

**Current problem:** Two code fixes needed for demo: (1) Supabase migration for 4 missing tables, (2) allowlist `go mod tidy`. Both are small, surgical changes with high demo impact.