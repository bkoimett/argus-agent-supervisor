# Build Plan

This file tracks the current work-in-progress plan. Updated after each completed issue.

---

## Current Focus: M0-02 (Next)

**M0-01: DONE** - Environment and scaffold verified.

**Next up: M0-02 OpenCode Compatibility Probe**

### M0-02 Plan

1. Check if `opencode` is installed and in PATH
2. Run `opencode --version` to verify it works
3. Run `opencode --help` to understand CLI flags
4. Test spawning a trivial OpenCode session (e.g., `opencode run "echo hello"` or similar)
5. Verify we can capture stdout/stderr and stop the process
6. Document findings in PROGRESS.md

### Unknowns (Recorded per Instructions)

| Item | Status | Notes |
|------|--------|-------|
| OpenCode API/CLI stability | UNKNOWN | Need to probe in M0-02 |
| OpenCode JSON-RPC protocol | UNKNOWN | Need to probe in M0-02 |
| Muse Code availability | UNKNOWN | Deferred, not in MVP scope |
| Meta API availability | UNKNOWN | Deferred, not in MVP scope |
| Supabase free tier limits (current) | UNKNOWN | Check at M1-04 |

### V1.0 Scope Lock

Per PRD section 6, the following are **explicitly not MVP** and will not be started until M1-M6 are proven with 2+ weeks daily use:

- Resource Guardian (auto pause/stop)
- Model switching / fallback routing
- Git awareness beyond branch name + dirty flag
- Machine resource monitoring (CPU, RAM, disk)
- Task queue, agent handoffs, focus mode
- Teams, workspaces, billing, marketplace
- Native Android/iOS apps
- Any second agent adapter beyond OpenCode

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