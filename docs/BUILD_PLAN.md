# Build Plan

This file tracks the current work-in-progress plan. Updated after each completed issue.

---

## Current Focus: OpenCode Session Lifecycle (Interrupt vs Stop)

**M0-01: DONE** - Environment and scaffold verified.

**M0-02: DONE** - OpenCode compatibility probe completed (`opencode` 1.19.0, `opencode serve` headless interface verified).

### In progress: Lifecycle semantics (M5/M6 report)

Make `Interrupt`/`Stop`/`Restore` semantically correct and consistent across Go service, desktop UI, PWA, and docs.

- **Interrupt:** stop current turn, retain session (WORKING). `POST /api/sessions/{id}/interrupt`.
- **Stop:** mark session DETACHED in adapter and SQLite (not STOPPED — cannot confirm OpenCode process termination on shared serve). Best-effort interrupt, do NOT kill shared serve server (no per-session termination endpoint in OpenCode 1.19.0). Idempotent. `DELETE /api/sessions/{id}`.
- **Restore:** mark original DETACHED, create NEW session for same project (do not claim reattachment). Accepts DISCONNECTED or DETACHED source state. `POST /api/sessions/{id}/restore`.

### Acceptance Criteria
- [ ] `Interrupt` stops turn, session retained, follow-up succeeds (mock + live tests)
- [ ] `Stop` → DETACHED in SQLite and adapter, other sessions unaffected (mock + live tests)
- [ ] Repeated Stop/Interrupt calls are idempotent (mock tests)
- [ ] `Close` reaps serve process, no orphaned OpenCode server (live test)
- [ ] Desktop UI shows DETACHED with explanation + Restore button (verified wiring)
- [ ] PWA shows DETACHED status badge (verified)
- [ ] `go test -race ./...`, `go vet ./...`, `go build ./...`, `gofmt` all PASS
- [ ] PROGRESS.md and MILESTONES.md updated with actual behavior + honest gaps
- [ ] PRD §10 state list conflict documented (DETACHED not in PRD list)

### Unknowns (Recorded per Instructions)
| Item | Status | Notes |
|------|--------|-------|
| OpenCode per-session termination endpoint | UNKNOWN / NOT AVAILABLE | OpenCode 1.19.0 `serve` has no per-session kill; Stop cannot confirm process termination of one session |
| OpenCode API/CLI stability | PROBED | `opencode serve` (1.19.0) headless interface verified; `opencode run` one-shot |

---

## Issue Template (for reference)

When creating new issues, use this format:

```markdown
### M<num>-<num>: <Title>

**Goal:** <one sentence>

**Dependencies:** [M<num>-<num>, ...]

**Package:** <services/agent-manager | apps/desktop | apps/pwa | supabase/migrations>

**Acceptance Criteria:**
- [ ] <specific, testable condition>
- [ ] <specific, testable condition>

**Test Command:** <command that verifies the criteria>

**Definition of Done:**
- All acceptance criteria pass
- Tests added/updated
- PROGRESS.md updated
- MILESTONES.md updated if issue list changes
- CHANGELOG.md updated if user-facing change
```