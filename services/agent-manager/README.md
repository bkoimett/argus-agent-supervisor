# agent-manager

The local Go service. Owns process lifecycle for coding agent sessions, exposes the `AgentAdapter` interface, and keeps local state in SQLite.

See `AGENTS.md` and `DESIGN.md` at the repo root before working here.

## Layout

```
cmd/agent-manager     entry point
internal/adapter       AgentAdapter interface and the OpenCode implementation
internal/session       session lifecycle and state
```

## Build

```bash
go build ./...
```

## Status

Scaffold only. M2 in WORKFLOW.md is where this gets built out: detect OpenCode, launch, stop, restart, run five sessions concurrently.
