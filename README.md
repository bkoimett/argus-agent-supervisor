# Argus
### Agent Registry, Guidance & Unified Supervision

Argus. Multiple coding agents. One command center.

Argus is a lightweight command center for AI coding agents. Run several agents at once on your own machine, walk away, and stay in control from your phone.

## Why Argus

Named after Argus Panoptes, the hundred eyed giant of Greek myth who never truly slept. Argus watches every agent session at once so you don't have to.

## What it does

- Launch and manage several AI coding agent sessions (OpenCode first, others later) from one dashboard
- Get notified the moment an agent needs your input, hits an error, or finishes
- Respond, approve, or deny requests from your phone, anywhere
- See quota and session state at a glance
- Recover automatically from a closed laptop lid, a lost connection, or a crash

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
- `GET /api/meta/config` returns the assistant configuration status (`base_url`, `model`, `has_key`, `configured`). The API key itself is never returned.
- `POST /api/meta/config` saves the assistant configuration (see below)
- `POST /api/meta/test-connection` tests the provider with the submitted values without saving them
- `PATCH /api/sessions/{id}` with body `{ "supervision_enabled": false }` toggles GUS supervision for one session

## Assistant (GUS) configuration

The desktop dashboard includes an Argus assistant panel (GUS) that diagnoses incidents and proposes recovery actions through a Meta-compatible, OpenAI-style API (for example Meta AI, OpenAI, Ollama, or LM Studio).

To configure it:

1. Start the Go service, then open the desktop dashboard and choose Settings.
2. Enter the API base URL (including `/v1` if your provider requires it), the model name, and your API key.
3. Choose Test Connection to verify the provider accepts the key and offers the model. Testing uses the values currently in the form, including a newly entered key, and does not save anything.
4. Choose Save. The service writes `META_BASE_URL`, `META_API_KEY`, and `META_MODEL` to its `.env` file (file permissions `0600`), reloads the file to verify the write, and only then reports success. Saved settings survive a service restart. You can also set `ARGUS_ENV_FILE` to choose which `.env` file is used.
5. Restart note: if the dashboard still reports the assistant as not configured after saving, make sure no older agent-manager process is still holding port 8080, and start the newly built service.

The API key is stored only on your machine in the `.env` file. It is never stored in the browser, never returned by the API, and never written to service logs. In Settings, a blank key field keeps the already saved key; enter a new key only to replace it.

Each session card, and the opened session view, has a Show GUS / Hide GUS control and a Supervision toggle. Supervision is stored per session in the local database. It only takes effect when a model is configured; when no model is configured, the GUS panel says so and supervision controls are disabled instead of implying the assistant is working.

### Limitations (MVP)

- Only the Go agent-manager runs; desktop (Tauri) and PWA are scaffolds.
- OpenCode integration uses the CLI (`opencode run --format=json`), not the unstable HTTP server.
- No push notifications yet (web‑push / ntfy fallback not implemented).
- Meta diagnosis uses a deterministic fallback unless `--meta-api-key` is supplied.
- Approval workflow is manual via the dashboard; no mobile UI yet.
- No quota display, no model switching, no git awareness, no resource guardian.

## Guides for AI assistants working in this repo

- [AGENTS.md](AGENTS.md), how to work in this repo efficiently and keep token use low
- [DESIGN.md](DESIGN.md), architecture and design standards
- [WORKFLOW.md](WORKFLOW.md), how issues, milestones, commits and checkpoints work

## License

MIT, see [LICENSE](LICENSE).