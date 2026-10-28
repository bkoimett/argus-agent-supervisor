# Argus
### Product Requirements Document

Version 2.0
Status: Build ready
Initial platform: Linux
Initial agent: OpenCode
Initial mobile experience: PWA
Author: Benjamin Koimett

---

## 1. Vision

A lightweight command center for AI coding agents.

A developer runs several coding agents at once on their own machine, walks away, and stays in control from a phone.

The laptop does the actual work. A thin cloud layer carries messages between the phone and the laptop. The phone is a remote control, not a second workstation.

Core promise: run your agents, walk away, stay in control.

## 2. Problem

Developers running more than one AI coding agent end up juggling multiple terminals, multiple tabs, permission prompts, and quota limits, with no single place to see what is happening or to step in remotely. A quota can run out mid task with nobody watching.

## 3. Target user

Primary: an individual developer who runs several agents on several repos, has limited free tier or paid model quota, and wants to step away from the desk without losing track of what the agents are doing.

Secondary, later: small dev teams, agencies, and AI focused shops. Do not design for them yet. Every decision that trades solo-dev simplicity for team features is a decision to postpone.

## 4. Design principles

- **Local first.** The actual coding work never leaves the developer's machine.
- **Cheap to build and cheap to run.** Every dependency should have a free tier or a self-hosted, open source path. No line item should require a credit card before the product has a single paying user.
- **Agent agnostic.** OpenCode is the first adapter, not a permanent dependency.
- **Small surface area.** Fewer screens and fewer states, done reliably, beats a feature-complete dashboard that breaks under real use.
- **Honest about estimates.** Any number shown to the user (quota, resource use) is labeled as observed or confirmed. Never guess and present it as fact.

## 5. MVP scope

The MVP proves one loop, end to end, reliably:

> Start an agent on the laptop. Close the laptop lid or walk away. Get a notification on the phone when the agent needs input. Respond from the phone. See the agent continue.

Everything in the MVP exists to make that loop work five times over, for up to five concurrent sessions, on Linux, with OpenCode.

In scope:

- Add a project (local directory) and launch an OpenCode session against it
- Desktop dashboard showing session state for every running project
- Stop, restart, pause and resume a session from the desktop
- Device pairing between phone and laptop
- PWA showing the same session states, with the ability to read the agent's question and answer it or approve/deny a permission request
- Push notification when a session needs input, errors, or completes
- Session survives the desktop GUI closing, and reconnects automatically when the network returns
- Basic quota display where the provider exposes it (a simple bar, no automation)

## 6. Explicitly not MVP

Deferred, in priority order, revisited only after the core loop above is proven with real daily use:

1. Resource Guardian (automatic pause or stop rules)
2. Model switching / fallback routing
3. Git awareness beyond a raw branch name and dirty/clean flag
4. Machine resource monitoring (CPU, RAM, disk)
5. Task queue, agent handoffs, focus mode
6. Teams, workspaces, billing, marketplace
7. Native Android/iOS apps
8. Any second agent adapter beyond OpenCode

Building any of these before the core loop is proven is scope creep that puts the whole MVP at risk.

## 7. Architecture

```
PHONE (React PWA)
      |
   HTTPS/WSS
      |
   CONTROL PLANE  <-- Supabase free tier: Auth, Postgres, Realtime
      |
 secure outbound channel (laptop initiates, never exposed publicly)
      |
DESKTOP APP (Tauri + React)
      |
LOCAL GO SERVICE  <-- process management, OpenCode adapter, SQLite state
      |
   OpenCode processes (one per project)
```

The laptop always initiates the outbound connection. Nothing on the laptop is ever reachable from the public internet directly.

**Cost-aware note:** the control plane is intentionally a thin message and auth layer. If Supabase's free tier is ever outgrown, the same Go service already running locally can be repointed at a small self-hosted Postgres instance (Docker, on a $4 to $6/month VPS) with a minimal WebSocket relay written in Go. Keeping the control plane thin now is what makes that swap cheap later.

## 8. Technology stack

| Layer | Choice | Why |
|---|---|---|
| Desktop shell | Tauri 2 | Free, open source, far smaller than Electron |
| Local service | Go | Single static binary, no runtime to install, easy cross-compilation later |
| Frontend | React + TypeScript + Vite + Tailwind | Free, large ecosystem, fast dev loop |
| Backend | Supabase (free tier) | Auth, Postgres, Realtime in one place, generous free tier |
| Mobile | React PWA | No app store review, no $99/year Apple developer fee to start |
| Local persistence | SQLite | Zero cost, zero ops, ships inside the Go binary |
| Notifications | Web Push (VAPID, free) or ntfy.sh (open source, self-hostable) | Avoids a paid push provider entirely |

Nothing in this stack requires a paid plan to reach the MVP definition of done in section 15.

## 9. Agent abstraction

```
AgentAdapter
  start()
  stop()
  restart()
  status()
  sendMessage()
  abort()
  respondToPermission()
  getEvents()

MVP:
  AgentAdapter -> OpenCodeAdapter

Future:
  AgentAdapter -> OpenCode, Claude Code, Codex, Gemini CLI, others
```

The UI and the control plane never talk to OpenCode directly. Everything goes through the adapter interface, which is what keeps a second agent from being a rewrite later.

## 10. Session states

```
STARTING, WORKING, WAITING, PERMISSION_REQUIRED,
PAUSED, COMPLETED, ERROR, CRASHED, DISCONNECTED, STOPPED
```

## 11. Notifications

Event-driven, not chatty. Notify for: needs input, permission request, completed, crashed, important error, computer disconnected, quota warning. Never notify on ordinary tool calls or routine progress messages.

**Platform risk to design around:** iOS only supports web push for a PWA that has been added to the home screen, and only from iOS 16.4 onward. This needs to be tested early and called out clearly in onboarding, with ntfy.sh as a documented fallback for anyone on an unsupported iOS version.

## 12. Data model (MVP)

```
users, devices, projects, sessions, events, commands, notification_preferences
```

`providers` and `usage_snapshots` are added when quota display (section 5) is built, not before. Everything under section 6 gets its own tables only when that feature is actually being built.

## 13. Security

- Row Level Security on every Supabase table
- Temporary pairing code (5 minute expiry) to link a phone to a laptop
- Device revocation, at any time, from the phone
- No public port ever opened on the laptop
- Provider API keys stay on the laptop, never uploaded to the control plane
- All traffic over HTTPS/WSS

## 14. Reliability and offline behavior

The Go service must survive: GUI closing, GUI crash, internet loss, Supabase outage, laptop sleep/wake, agent crash, laptop restart. OpenCode processes are never children of the GUI process. On restart, the app lists sessions that were running and offers to restore them.

If the internet drops, OpenCode keeps running locally, the Go service keeps local state in SQLite, and everything syncs to Supabase and the phone once the connection returns.

## 15. MVP definition of done

A developer can: install the app, add up to five projects, launch five OpenCode sessions, see all five from one dashboard, walk away, open the PWA on their phone, see every session's state, get a notification that matters, read the agent's question, answer it from the phone, watch the agent continue, and come back to the laptop with nothing lost.

## 16. Milestones

- **M1, Foundation:** monorepo, Tauri shell, Go service skeleton, Supabase project, auth, local dev environment.
- **M2, Agent manager + OpenCode adapter:** detect OpenCode, launch/stop/restart sessions, run five concurrently, wire the adapter interface.
- **M3, Desktop dashboard:** project cards, session state, basic controls, activity log.
- **M4, Pairing + PWA:** pairing code flow, Supabase Realtime wiring, PWA dashboard, respond to a permission request from the phone.
- **M5, Notifications:** web push (or ntfy fallback), event-driven rules, notification preferences.
- **M6, Reliability hardening:** GUI crash recovery, laptop sleep/wake, network loss and reconnect, five-agent stress test.

Everything from section 6 becomes M7 onward, and only after M1 to M6 have been used daily, by you, for at least a couple of weeks.

## 17. Cost estimate

- **Building the MVP:** effectively free. Every tool listed in section 8 has a free tier or is open source. The only real cost is your time.
- **Running it solo, or for a small beta:** free, inside Supabase's free tier limits (roughly 50,000 monthly active users, 500MB database, 2 concurrent realtime connections per project on the current free plan; check current limits before committing, since these change).
- **If a self-hosted fallback becomes necessary:** a $4 to $6/month VPS is enough for Postgres plus a small Go relay for a few hundred users.
- **App store costs, if native mobile is ever built:** $99/year for Apple, a one-time roughly $25 fee for Google. Not needed for the PWA-based MVP.

## 18. Risks and mitigations

| Risk | Mitigation |
|---|---|
| OpenCode's API changes or is unstable | Keep the adapter interface strict, isolate all OpenCode-specific code in one package |
| Supabase free tier limits hit sooner than expected | Thin control plane by design, self-host fallback documented in section 7 |
| iOS push notifications unreliable | Test on real iOS hardware early, ntfy.sh fallback documented in onboarding |
| Scope creep into resource guardian / analytics before MVP is proven | Section 6 is the guardrail, revisit only after two weeks of real daily use |
| Single developer, limited time | Milestones are ordered so each one is independently useful, so stopping after M4 or M5 still leaves something usable |

## 19. Future roadmap (unchanged in spirit, sequenced after MVP proof)

Multi-agent support (Claude Code, Codex, Gemini CLI adapters), focus mode, task queue, agent handoffs, resource guardian, model switching, teams and workspaces, developer marketplace, native mobile apps, cross-platform desktop (Windows, macOS), analytics.

## 20. Success metrics

MVP: five concurrent agents run reliably, remote commands succeed reliably, reconnection works after a network drop, sessions survive a GUI close, session state is readable at a glance, quota warnings appear before exhaustion where the provider exposes real data.

Product, once past MVP: daily active users, agents managed per user, sessions per day, remote interactions per day, retention, notification engagement, free to paid conversion.

## 21. North star

"I can start my AI developers in the morning, walk away, and Argus tells me only when they need me."
