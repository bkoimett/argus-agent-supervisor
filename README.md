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

Early build. See [docs/prd.md](docs/prd.md) for the full product requirements and [WORKFLOW.md](WORKFLOW.md) for the current milestone.

## Repository layout

```
apps/desktop             Tauri + React desktop app
apps/pwa                 React PWA, the phone facing remote control
services/agent-manager   Go local service, process management and agent adapters
supabase                 Database schema and control plane config
docs                     Product, marketing and monetization documents
```

## Stack

Tauri 2, Go, React with TypeScript, Vite and Tailwind, Supabase (Postgres, Auth, Realtime), SQLite locally.

## Getting started

This is a scaffold. Each package will get its own detailed setup notes as it fills in. Starting points:

```bash
# Go service
cd services/agent-manager
go build ./...

# Desktop app
cd apps/desktop
npm install
npm run dev

# PWA
cd apps/pwa
npm install
npm run dev
```

## Guides for AI assistants working in this repo

- [AGENTS.md](AGENTS.md), how to work in this repo efficiently and keep token use low
- [DESIGN.md](DESIGN.md), architecture and design standards
- [WORKFLOW.md](WORKFLOW.md), how issues, milestones, commits and checkpoints work

## License

MIT, see [LICENSE](LICENSE).
