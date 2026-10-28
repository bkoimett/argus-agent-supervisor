# WORKFLOW.md

How work gets planned, built, and shipped in this repository.

## Milestones

These map to `docs/prd.md` section 16. The AI assistant sets these up as GitHub milestones on the repository, each with its own set of issues, before writing code for that milestone.

1. **M1, Foundation.** Monorepo structure, Tauri shell boots, Go service skeleton builds, Supabase project created, auth wired, local dev environment documented.
2. **M2, Agent manager and OpenCode adapter.** Detect OpenCode, launch, stop and restart a session, run five sessions concurrently, `AgentAdapter` interface implemented.
3. **M3, Desktop dashboard.** Project cards, live session state, start, stop, pause and resume controls, activity log.
4. **M4, Pairing and PWA.** Pairing code flow, Supabase Realtime wiring, PWA dashboard, respond to a permission request from the phone.
5. **M5, Notifications.** Web push or ntfy.sh fallback, event driven rules, notification preferences screen.
6. **M6, Reliability hardening.** GUI crash recovery, laptop sleep and wake, network loss and reconnect, five agent stress test.

Each milestone gets broken into individual GitHub issues before work starts. Issues are small enough to be finished and checkpointed in a single sitting.

## Issue format

Every issue the AI creates should include:

- A one line title matching the format `[M<number>] short description`
- What done looks like, as a short checklist
- Which package it touches, see the `AGENTS.md` directory map
- Which milestone it belongs to

## The commit and push rule

**The AI never runs `git commit` or `git push` without asking first, every single time, no exceptions.**

Before committing, the AI:

1. Summarizes what changed and why, in plain language
2. Lists the files touched
3. States which issue this closes or progresses
4. Waits for an explicit go ahead

Only after that confirmation does the AI stage, commit, and push. The same applies to opening a pull request or merging one. Nothing lands on `main` without a review step, even if the AI is confident the change is correct.

## Branching

One branch per issue, named `issue-<number>-short-slug`. Pull request into `main`, referencing the issue it closes. No direct commits to `main`.

## Checkpoints and manual testing

At the end of each milestone, before it is marked complete, the AI provides a short list of manual checks for you to run yourself. These are not automated tests, they are a human sanity check that the milestone actually works in practice.

- **M1:** desktop app opens, Go service starts and responds to a health check, Supabase auth login succeeds
- **M2:** launch five OpenCode sessions from the Go service directly, no UI needed yet, confirm all five run and can be stopped individually
- **M3:** launch and stop sessions from the desktop dashboard, confirm state updates live without a manual refresh
- **M4:** pair a phone, see the same session state on the phone that is showing on the desktop, respond to a permission prompt from the phone and watch the desktop session continue
- **M5:** trigger each notification type, needs input, error, completed, and confirm it arrives on the phone, including a real test on an iOS device
- **M6:** close the laptop lid mid session, reopen it, confirm the session is still there, kill the desktop GUI process directly and confirm OpenCode keeps running, disconnect the network and confirm state syncs once it is back

The AI should not mark a milestone as done until you confirm these checks passed.

## When the AI should stop and ask instead of proceeding

- Before any commit or push, see above
- Before adding a new dependency, especially anything with a paid tier
- Before deviating from the stack or architecture in `docs/prd.md` and `DESIGN.md`
- Before starting work on anything listed as not MVP in `docs/prd.md` section 6
- When a task turns out to span more than one package and needs sub agent delegation per `AGENTS.md`, confirm the split before starting
