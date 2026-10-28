# DESIGN.md

Architecture and design standards for Argus. Read this before writing UI code or making a structural decision.

## Architecture overview

```
PHONE (PWA)
   |
HTTPS/WSS
   |
SUPABASE          Auth, Postgres, Realtime, control plane, kept thin by design
   |
outbound only, laptop initiates the connection
   |
DESKTOP APP (Tauri + React)
   |
GO SERVICE        process management, agent adapters, SQLite state
   |
OpenCode processes, one per project
```

Full detail and rationale: `docs/prd.md`.

## Module boundaries

- The **Go service** owns process lifecycle and all OpenCode specific logic, behind the `AgentAdapter` interface. Nothing outside `services/agent-manager` should ever import an OpenCode specific type.
- The **desktop app** talks to the Go service over a local channel, never directly to OpenCode.
- The **PWA** never talks to the laptop directly. Everything goes through Supabase Realtime.
- The **Supabase layer** is a message and auth relay, not a business logic layer. Session state transitions and quota math live in the Go service, not in Supabase functions, so the whole thing stays portable to a self hosted fallback later.

Adding a new agent (Claude Code, Codex, and so on) later means writing a new adapter that satisfies the existing `AgentAdapter` interface. It should never require changes to the desktop app, the PWA, or the database schema.

## Visual design principles

The goal is a control room, not a marketing landing page. Argus watches, it doesn't perform.

**Avoid:**

- Gradients of any kind, especially purple to blue or pink to orange blends
- Glassmorphism, frosted panels, glow effects behind buttons
- Generic rounded everything cards with heavy drop shadows
- Emoji used as primary status indicators in the UI (fine in docs and commit messages, not in the dashboard itself)
- Centering every element by default, most of this product is a dense status view, not a hero section
- Stock AI generated illustration style, isometric blobs, abstract circuit patterns

**Prefer:**

- A small, deliberate color palette: a dark neutral base, one accent color reserved for attention states (needs input, error), muted tones for everything else
- Real status indicators: color plus text plus icon together, never color alone
- Dense, scannable layout, this is a monitoring tool, information density is a feature
- Monospace or a clean technical sans for anything showing agent output, logs, or status text
- Motion used only to draw attention to a state change, never as decoration

## Writing and copy standards

Applies to UI copy, marketing copy, commit messages, and this documentation itself.

- **No em dash or long dash character, anywhere.** Use a period, a comma, or restructure the sentence instead.
- **No hyphen used as a dash substitute either**, hyphens are for compound words only.
- Sentence case for headings and buttons, not Title Case.
- Short sentences. Cut filler like "in order to", "it is important to note that", "furthermore".
- Say what happened, not that something "seamlessly" or "effortlessly" happened. Skip the adverbs.
- Numbers and states are shown as data, a bar, a count, a label, not narrated in prose where a glance would do.

## Naming

Product name: Argus. Full form, Agent Registry, Guidance & Unified Supervision, belongs in the README and marketing material, not repeated in UI chrome. Tagline: "Argus. Multiple coding agents. One command center."
