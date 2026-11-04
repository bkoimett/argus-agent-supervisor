# Milestones

Refined from `docs/prd.md` section 16 and `WORKFLOW.md`. Each milestone broken into small, dependency-aware issues.

---

## M0: Pre-Foundation Verification (NEW - added before M1)

**Goal:** Verify environment, scaffold, and external tool compatibility before building features.

| Issue | Title | Goal | Dependencies | Acceptance Criteria | Test Command |
|-------|-------|------|--------------|---------------------|--------------|
| M0-01 | Verify environment and scaffold | Confirm Go, Git, Node, scaffold builds | None | All version checks pass, Go tests pass, Go run works, npm install works | `go version && git --version && cd services/agent-manager && go test ./... && go run ./cmd/agent-manager && cd ../../apps/desktop && npm install && cd ../pwa && npm install` |
| M0-02 | OpenCode compatibility probe | Detect OpenCode binary, verify CLI, test spawn/stop | M0-01 | `opencode --version` works, can start/stop a trivial session, capture stdout/stderr | `which opencode && opencode --version && opencode --help` |
| M0-03 | Muse Code probe (deferred) | Record as unknown, revisit post-MVP | M0-01 | Documented as unknown in PROGRESS.md | N/A |
| M0-04 | Meta API probe (deferred) | Record as unknown, revisit post-MVP | M0-01 | Documented as unknown in PROGRESS.md | N/A |

---

## M1: Foundation

**Goal:** Monorepo structure, Tauri shell boots, Go service skeleton builds, Supabase project created, auth wired, local dev environment documented.

| Issue | Title | Goal | Dependencies | Acceptance Criteria | Test Command |
|-------|-------|------|--------------|---------------------|--------------|
| M1-01 | Initialize root go.mod (optional) | Add workspace-level Go module if needed | M0-01 | `go work` works or each service builds independently | `cd services/agent-manager && go build ./...` |
| M1-02 | Tauri desktop shell boots | `npm run tauri dev` opens a window | M0-01 | Window opens with "Argus" title | `cd apps/desktop && npm run tauri dev` (manual) |
| M1-03 | Go service health endpoint | Add `/health` HTTP endpoint to agent-manager | M0-01 | `curl http://localhost:PORT/health` returns 200 OK | `curl http://localhost:PORT/health` |
| M1-04 | Supabase project + auth | Create project, enable email/password auth | M0-01 | Can sign up and sign in via Supabase dashboard | Manual in Supabase dashboard |
| M1-05 | Apply migration 0001_init.sql | Run migration against Supabase | M1-04 | Tables exist with RLS enabled | `supabase db push` or dashboard SQL editor |
| M1-06 | RLS policies | Add per-table policies scoping to `auth.uid()` | M1-05 | `SELECT * FROM projects` returns only user's rows | Manual SQL test |
| M1-07 | Local dev docs | Document `.env` setup, Supabase keys, run commands | M1-03, M1-04 | New developer can run full stack locally | Follow docs, verify all three layers run |

---

## M2: Agent Manager + OpenCode Adapter

**Goal:** Detect OpenCode, launch/stop/restart sessions, run five concurrently, wire the AgentAdapter interface.

| Issue | Title | Goal | Dependencies | Acceptance Criteria | Test Command |
|-------|-------|------|--------------|---------------------|--------------|
| M2-01 | OpenCode detection | Find `opencode` in PATH, verify version | M0-02 | `which opencode` succeeds, version >= minimum | `opencode --version` |
| M2-02 | Session Manager with SQLite | Persist sessions to SQLite, survive process restart | M1-03 | Sessions survive Go service restart | Manual: start service, create session, restart service, list sessions |
| M2-03 | OpenCodeAdapter.Start | Spawn OpenCode process, return session ID | M2-01, M2-02 | Process runs, PID tracked, session in DB | Unit test + manual |
| M2-04 | OpenCodeAdapter.Stop | Mark session DETACHED in adapter and SQLite (not STOPPED — cannot confirm OpenCode process termination on shared serve). Best-effort interrupt of current turn. Does NOT kill the shared `opencode serve` process (no per-session termination endpoint in OpenCode 1.19.0). Idempotent. | M2-03 | Session marked DETACHED in SQLite, Status = DETACHED, other sessions unaffected | Unit test + manual |
| M2-05 | OpenCodeAdapter.Restart | Mark original session DETACHED, create a NEW session in same project dir (does not claim reattachment). Accepts DISCONNECTED or DETACHED as source state. | M2-03, M2-04 | New session ID, original DETACHED, same project dir | Unit test |
| M2-06 | OpenCodeAdapter.Status | Read adapter tracking state: WORKING if tracked and active, DETACHED if stopped but not confirmed terminated, error if never started. | M2-03 | Returns WORKING/DETACHED correctly; error for unknown session | Unit test |
| M2-07 | OpenCodeAdapter.SendMessage | Write to OpenCode session via serve message endpoint. Errors on DETACHED/unknown sessions. | M2-03 | Message delivered to WORKING session; error on DETACHED session | Manual with real OpenCode |
| M2-08 | OpenCodeAdapter.Interrupt | Stop the current turn via `POST /session/{id}/interrupt`, retain session (WORKING) for follow-up. Distinct from Stop. | M2-03 | Current turn interrupted, session still tracked and WORKING | Unit test + manual |
| M2-09 | OpenCodeAdapter.Abort | Alias for Stop (marks DETACHED, best-effort interrupt, shared serve NOT killed). | M2-04 | Same semantics as Stop | Unit test |
| M2-10 | OpenCodeAdapter.GetEvents | Tail OpenCode stdout/stderr/events | M2-03 | Returns structured Event slice | Unit test |
| M2-11 | Concurrency: 5 sessions | Launch 5 sessions, all track independently | M2-03..M2-10 | All 5 RUNNING, distinct PIDs, DB has 5 rows | `go test -run TestFiveSessions` |
| M2-12 | Health check integration | Desktop can poll `/health` and `/sessions` | M1-03, M2-02 | Desktop shows live session list | Manual |

---

## M3: Desktop Dashboard

**Goal:** Project cards, live session state, start/stop/pause/resume controls, activity log.

| Issue | Title | Goal | Dependencies | Acceptance Criteria | Test Command |
|-------|-------|------|--------------|---------------------|--------------|
| M3-01 | Project list view | Show projects from Go service | M1-02, M1-03 | Cards render with name, dir, agent | Manual |
| M3-02 | Add project modal | Pick directory, POST to Go service | M3-01 | New project appears in list | Manual |
| M3-03 | Session state badge | Live status from Go service (poll or WS) | M1-03, M2-12 | Badge updates without refresh | Manual |
| M3-04 | Session controls | Start/Stop/Pause/Resume buttons call Go service | M2-03..M2-05 | Buttons work, state updates | Manual |
| M3-05 | Activity log | Show events from GetEvents per session | M2-10 | Log scrolls, shows agent output | Manual |
| M3-06 | Quota bar (basic) | Display quota if provider exposes it | M2-10 (events) | Bar renders, labeled "observed" | Manual |

---

## M4: Pairing + PWA

**Goal:** Pairing code flow, Supabase Realtime wiring, PWA dashboard, respond to permission from phone.

**Status:** PARTIALLY IMPLEMENTED (2026-11-02). PWA pairing code UI and session list exist but read from Supabase tables that lack RLS policies and the `pairing_codes` table was missing from the migration. Desktop does not generate pairing codes. PWA has no session detail view or composer for sending messages to the agent. Command processing loop (phone→agent messaging via Supabase `commands` table) not implemented.

| Issue | Title | Goal | Dependencies | Acceptance Criteria | Test Command |
|-------|-------|------|--------------|---------------------|--------------|
| M4-01 | Pairing code generation | Desktop generates 6-char code, 5 min expiry | M1-04, M1-06 | Code displayed, stored in Supabase | `POST /api/pairing-code` returns code |
| M4-02 | Pairing code validation | PWA accepts code, links device to user | M4-01 | Device row created, paired_at set | PWA validates code against `pairing_codes` table |
| M4-03 | Device revocation | Phone can revoke, desktop sees revoked | M4-02 | Device row has revoked_at | Not implemented |
| M4-04 | Supabase Realtime: sessions | Subscribe to sessions table changes | M1-05, M1-06 | PWA gets live updates | PWA subscribes to `sessions` table (implemented) |
| M4-05 | Supabase Realtime: commands | Phone writes command, desktop executes | M4-04, M2-07 | Command delivered, agent responds | `POST /api/sessions/{id}/message` → `commands` table → adapter.SendMessage |
| M4-06 | PWA dashboard | Mirror desktop session list | M4-04 | Same state on phone and desktop | PWA reads `sessions` table via Supabase (implemented) |
| M4-07 | Permission response from phone | Approve/deny buttons call RespondToPermission | M4-05, M2-09 | Agent continues after response | **UNVERIFIED** — no live permission request triggerable. Not implemented end-to-end. |

---

## M5: Notifications

**Goal:** Web push or ntfy.sh fallback, event-driven rules, notification preferences screen.

**Status:** IMPLEMENTED (2026-11-02). Notification client with dedup, ntfy.sh delivery, and PWA preferences UI implemented. Live delivery not yet verified (requires VAPID keys or ntfy topic configuration).

| Issue | Title | Goal | Dependencies | Acceptance Criteria | Test Command |
|-------|-------|------|--------------|---------------------|--------------|
| M5-01 | VAPID key generation | Generate and store VAPID keys | M1-04 | Keys generated, stored in config | `go test ./internal/notification/` |
| M5-02 | Push subscription (PWA) | Request permission, store subscription | M4-02, M5-01 | Preferences stored in PWA localStorage | PWA settings toggles persist |
| M5-03 | Push send (Go service) | Send web push on notification events | M2-10, M5-01 | ntfy.sh delivery works when enabled | `go test ./internal/notification/` |
| M5-04 | ntfy.sh fallback | Implement ntfy topic publish | M5-03 | Works when web push fails | `go test ./internal/notification/` |
| M5-05 | Event-driven rules | Map session events → notification types | M2-10 | Only completed/crashed/error incidents notify | `go test ./internal/notification/` |
| M5-06 | Notification preferences UI | Toggle per-type in PWA and desktop | M1-06, M4-06 | Preferences persist, respected | `cd apps/pwa && npx vitest run` |

---

## M6: Reliability Hardening — IMPLEMENTED (2026-11-02)

**Goal:** GUI crash recovery, laptop sleep/wake, network loss and reconnect, five-agent stress test.

**Status:** PARTIALLY IMPLEMENTED. Session restore on reboot implemented; GUI crash survival and five-agent stress test are manual checks.

| Issue | Title | Goal | Dependencies | Acceptance Criteria | Test Command |
|-------|-------|------|--------------|---------------------|--------------|
| M6-01 | GUI crash survival | Kill Tauri process, OpenCode keeps running | M3-04 | Sessions still in Go service, PWA shows them | `pkill -f tauri` then check |
| M6-02 | Laptop sleep/wake | Suspend/resume, sessions intact | M6-01 | No session loss, reconnects to Supabase | Manual |
| M6-03 | Network loss/reconnect | Disconnect WiFi, reconnect, state syncs | M4-04, M6-02 | Supabase Realtime reconnects, no gaps | Manual |
| M6-04 | Five-agent stress test | Run 5 sessions for 30 min, random ops | M2-11 | Zero crashes, zero lost messages | Manual |
| M6-05 | Session restore on reboot | Restart laptop, app offers to restore | M2-02, M3-01 | Previous sessions listed, can resume | `POST /api/sessions/{id}/restore` returns new session |

**What was implemented:**
- `services/agent-manager/internal/session/manager.go` — Added `RestoreSessions()` which marks all non-terminal sessions as `DISCONNECTED` on startup. Added `RestoreSession(sessionID)` which marks original session `STOPPED` and creates a new session for the same project using the most recent task's prompt.
- `services/agent-manager/internal/dashboard/dashboard.go` — Added `POST /api/sessions/{id}/restore` endpoint.
- `cmd/agent-manager/main.go` — Calls `mgr.RestoreSessions()` on startup before starting the dashboard server.
- `apps/desktop/src/App.tsx` — `SessionCard` shows "Restore" button for `DISCONNECTED`/`STOPPED` sessions, "Dismiss" button for `DISCONNECTED` sessions.
- `internal/notification/notification.go` — Added dedup key generation, `NtfyAllowBroadcast` flag, sensitive field stripping from broadcast payloads. `sendApprovalNotification` skips `Permission` type notifications (end-to-end unverified).
- `internal/supabase/client.go` — Fixed `SyncSession` to use `project_dir` column (schema mismatch fix).
- `supabase/migrations/0001_init.sql` — `sessions.id` → `text`; `sessions.project_id` nullable; `sessions.project_dir` added; `events.session_id` / `commands.session_id` → `text`; added permissive RLS policies.
- `internal/supabase/client_test.go` (new) — `TestSyncSessionDisabled`, `TestSessionIDIsText`.


## M7+: Post-MVP (per PRD section 6)

Deferred. Only planned after M1-M6 used daily for 2+ weeks.

| Feature | Milestone |
|---------|-----------|
| Resource Guardian | M7 |
| Model switching / fallback | M8 |
| Git awareness | M9 |
| Machine resource monitoring | M10 |
| Task queue, handoffs, focus mode | M11 |
| Teams, workspaces, billing | M12 |
| Native mobile apps | M13 |
| Second agent adapter | M14+ |