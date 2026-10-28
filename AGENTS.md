# AGENTS.md

Guidance for any AI assistant working in this repository. Read this before touching code.

## Project in one paragraph

Argus is a command center for AI coding agents. A Go service on the developer's machine manages agent sessions, a Tauri desktop app shows them, a React PWA lets the developer control them from a phone, and Supabase carries messages between the phone and the laptop. Full detail lives in `docs/prd.md`. Read it once, don't re-read it for every task, refer back to `DESIGN.md` for architecture specifics as needed.

## Directory map

Read this table instead of exploring the tree. Only open a directory when a task actually touches it.

| Path | What it is | Touch it when |
|---|---|---|
| `services/agent-manager` | Go local service, process management, agent adapters | Working on session lifecycle, OpenCode integration, local state |
| `apps/desktop` | Tauri + React desktop dashboard | Working on the desktop UI |
| `apps/pwa` | React PWA, phone remote control | Working on the mobile facing UI |
| `supabase/migrations` | Database schema | Adding or changing a table |
| `docs` | PRD, marketing, monetization | Reference only, do not edit unless asked |
| `.github` | Issue templates, CI, PR template | Workflow and CI tasks only |

## Reducing token consumption

This is a monorepo. Loading the whole tree for a single package task wastes context and money. Follow these rules.

1. **Scope every task to one package first.** Before reading any file, decide which row in the directory map above the task belongs to. Read only that package's files.
2. **Use targeted search, not full reads.** Grep or glob for the symbol, file, or string needed instead of opening entire files to skim them.
3. **Delegate cross package work to sub agents.** If a task genuinely spans more than one package, for example a new event type that touches the Go service, the Supabase schema, and both frontends, split it into one sub agent per package, each given only that package's directory and a short shared contract (the event shape, the API surface) to implement against. Do not let one agent hold the full context of all four packages at once, it is slower and burns more tokens than it saves.
4. **Summarize, don't paste.** When reporting back on a large file or a long log, summarize the relevant part instead of quoting it in full.
5. **Don't re derive what's already documented.** Architecture decisions live in `DESIGN.md`, product scope lives in `docs/prd.md`. Reference them, don't re explain them in every response.

## When to use a sub agent

Good candidates for a dedicated sub agent, each scoped to a single package:

- Go service work (session lifecycle, the OpenCode adapter, SQLite state)
- Desktop UI work (Tauri and React)
- PWA work (the phone facing React app)
- Supabase schema and Realtime wiring

A coordinating agent should define the shared contract (event names, payload shapes, API routes) once, hand it to each sub agent, and only integrate the pieces at the end. This keeps each sub agent's context window small and focused, and keeps the overall task cheaper to run.

## Working conventions

- **Language and framework choices are fixed** by `docs/prd.md` section 8. Don't introduce a new framework or swap a dependency without asking first.
- **No new paid dependencies.** Everything in this stack has a free tier or is open source, by design. Flag it and ask before adding anything that requires a paid plan.
- **Commit messages:** `type(scope): short description`, for example `feat(agent-manager): add session pause`. Types: feat, fix, docs, chore, refactor, test.
- **Branch naming:** `issue-<number>-short-slug`, matching the GitHub issue the branch closes.
- **Tests live next to the code they test**, `_test.go` beside the Go source, `*.test.tsx` beside the React component.

## Before you commit or push

See `WORKFLOW.md`. In short: never run `git commit` or `git push` without presenting a summary of the change and getting an explicit go ahead first. This applies every time, not just for the first commit.

## Design and copy standards

See `DESIGN.md` for the full detail. The short version: no gradients, no generic AI blob visual style, no em dash or long dash characters in any copy, UI text, or commit message, sentence case over title case, plain direct language over filler.
