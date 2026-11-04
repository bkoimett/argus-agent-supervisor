# Argus
### Agent Registry, Guidance & Unified Supervision

Argus. Multiple coding agents. One command center.

Argus is a lightweight command center for AI coding agents. Run several agents at once on your own machine, walk away, and stay in control from your phone.

![Home Screen](img/home_screen.png)

## Why Argus

Named after Argus Panoptes, the hundred eyed giant of Greek myth who never truly slept. Argus watches every agent session at once so you don't have to.

## What it does

- Launch and manage several AI coding agent sessions (OpenCode first, others later) from one dashboard
- Get notified the moment an agent needs your input, hits an error, or finishes
- Respond, approve, or deny requests from your phone, anywhere
- See quota and session state at a glance
- Recover automatically from a closed laptop lid, a lost connection, or a crash

![Live Sessions](img/live_sessions.png)

## System Architecture

```mermaid
flowchart TB
    subgraph Phone["📱 Phone (React PWA)"]
        PWA[PWA Dashboard\nSession list\nApproval composer]
    end

    subgraph Cloud["☁️ Control Plane (Supabase Free Tier)"]
        Auth[(Auth)]
        DB[(Postgres\nSessions, Events, Commands)]
        RT[Realtime\nWebSocket]
    end

    subgraph Laptop["💻 Laptop (Local-First)"]
        Desktop[Tauri Desktop App\nProject cards\nLive session badges]
        GoSvc[Go Service\nProcess manager\nAgent adapters\nSQLite state]
        OpenCode[(OpenCode processes\nOne per project)]
    end

    PWA <-- HTTPS/WSS --> Auth
    PWA <-- HTTPS/WSS --> DB
    PWA <-- WSS --> RT
    RT --> GoSvc
    GoSvc --> OpenCode
    Desktop --> GoSvc
    GoSvc -.-> DB
    GoSvc -.-> RT
```

*The laptop always initiates the outbound connection. Nothing on the laptop is ever reachable from the public internet directly. Provider API keys stay on the laptop, never uploaded to the control plane.*

## Status

Early build (MVP P2). See [docs/prd.md](docs/prd.md) for the full product requirements and [WORKFLOW.md](WORKFLOW.md) for the current milestone.

## Repository layout

```
apps/desktop             Tauri + React desktop app (not yet wired)
apps/pwa                 React PWA, the phone facing remote control (not yet wired)
services/agent-manager   Go local service, process management and agent adapters
supabase                 Database schema and control plane config
docs                     Product, marketing and monetization documents
```

## Stack

Tauri 2, Go, React with TypeScript, Vite and Tailwind, Supabase (Postgres, Auth, Realtime), SQLite locally.

## Built with Muse Wonder

This project was developed using **Muse Wonder** — an AI-native coding assistant that helped accelerate development across the stack:

- **Architecture & scaffolding** — Initial monorepo structure, Tauri + Go service skeleton, Supabase schema design
- **Agent adapter implementation** — OpenCode adapter with process lifecycle management (start, stop, restart, interrupt, send message)
- **Desktop dashboard** — React components for project cards, session state badges, activity logs, and quota bars
- **PWA remote control** — Pairing code flow, Supabase Realtime subscriptions, approval composer for permission requests
- **Notification system** — Web Push (VAPID) and ntfy.sh fallback with deduplication and event-driven rules
- **Reliability hardening** — Session restore on reboot, GUI crash survival, network loss/reconnect handling

Muse Wonder's ability to maintain context across the full stack (Go, React, TypeScript, SQL, Tauri config) made it possible to build a working multi-agent command center in a single hackathon sprint.

## Getting started (agent-manager only — the only runnable component right now)

```bash
# Build the Go service
cd services/agent-manager
go build -o agent-manager ./cmd/agent-manager

# Run the service (dashboard on http://localhost:8080)
./agent-manager --port=8080

# Optional: enable Meta diagnosis (requires API key)
# ./agent-manager --port=8080 --meta-api-key=YOUR_KEY --meta-base-url=https://api.meta.example.com/v1
```

Open **http://localhost:8080/** in a browser to see the dashboard UI.

![New Session](img/new_session_main.png)
![Agent Config](img/agent_config.png)

### Quick demo flow for reviewers

1. **Start the service** — runs the Go agent-manager with HTTP dashboard on port 8080
2. **Add a project** — click "Add Project", pick a local directory
3. **Launch a session** — click "Start" on the project card, enter a prompt
4. **Watch it work** — see live status badge cycle: STARTING → WORKING → WAITING/PERMISSION_REQUIRED
5. **Respond from the dashboard** — approve/deny permission requests inline
6. **Pair with PWA** (when wired) — generate pairing code, open PWA on phone, enter code

### API endpoints (for scripting)

- `GET /health` – health check
- `GET /api/dashboard` – full session/task/incident/approval snapshot
- `POST /api/sessions` – start a new OpenCode session  
  body: `{ "project_dir": "/path/to/project", "prompt": "your prompt", "model": "optional" }`
- `GET /api/events?session_id=...` – recent events for a session
- `POST /api/incidents?session_id=...` – body `{ "action": "detect" }` runs incident detection
- `POST /api/diagnosis` – body `{ "incident_id": "..." }` requests Meta/fallback diagnosis
- `POST /api/recovery/approval` – body `{ "recovery_id": "..." }` creates approval for a recovery action
- `POST /api/approvals/{id}` – body `{ "approved": true, "decided_by": "user" }` approves/rejects

### Limitations (MVP)

- Only the Go agent-manager runs; desktop (Tauri) and PWA are scaffolds.
- OpenCode integration uses the CLI (`opencode run --format=json`), not the unstable HTTP server.
- No push notifications yet (web‑push / ntfy fallback not implemented).
- Meta diagnosis uses a deterministic fallback unless `--meta-api-key` is supplied.
- Approval workflow is manual via the dashboard; no mobile UI yet.
- No quota display, no model switching, no git awareness, no resource guardian.

![Gus Questions](img/gus_questions.png)
![Full Gus Workflow](img/full_gus_work.png)

## Hackathon highlights

| Category | Achievement |
|----------|-------------|
| **Architecture** | Local-first, agent-agnostic design with clean adapter interface — swap OpenCode for Claude Code, Codex, Gemini CLI without touching UI or control plane |
| **Offline-first** | Sessions survive GUI crash, laptop sleep/wake, network loss; SQLite state syncs when connection returns |
| **Security** | Zero public ports on laptop; outbound-only connection; API keys never leave machine; RLS on every Supabase table |
| **Cost** | Entire stack runs on free tiers (Supabase, ntfy.sh, Web Push) — no credit card required to demo |
| **Built with AI** | Muse Wonder accelerated full-stack development: Go service, Tauri desktop, React PWA, Supabase schema, notifications |

## What's next (Post-MVP)

- Resource Guardian (auto-pause on quota/limits)
- Model switching & fallback routing
- Git awareness (branch, dirty state, diff summary)
- Second agent adapter (Claude Code, Codex, Gemini CLI)
- Native mobile apps

## Guides for AI assistants working in this repo

- [AGENTS.md](AGENTS.md), how to work in this repo efficiently and keep token use low
- [DESIGN.md](DESIGN.md), architecture and design standards
- [WORKFLOW.md](WORKFLOW.md), how issues, milestones, commits and checkpoints work

## License

MIT, see [LICENSE](LICENSE).