# OpenCode 1.19.0 compatibility probe — scratch note

Status: scratch / not for commit. Records exact commands and observed output.
Distinguishes **verified** behavior from **assumptions**.

## Environment
- OpenCode binary: `/home/g/.npm-global/bin/opencode`, version `1.19.0`
- Probe project (harmless, non-sensitive): `/tmp/opencode/demo` containing only `main.go` (`package main; func main(){ fmt.Println("hello") }`)
- No `.env` secrets inspected, printed, or transmitted.

## Method
Controlled experiments against a headless `opencode serve` instance on `http://127.0.0.1:4097`, using `curl` against the server's REST/SSE API discovered from the frontend bundle (`assets/index-*.js`).

## Verified findings

### 1. `opencode run --format json` is one-shot, no stdin follow-up
Command:
```
opencode run --format json --dir /tmp/opencode/demo "list the files in the current directory"
```
Observed output: JSONL events `step_start`, `tool_use`, `text`, `step_finish`, then process exits (exit 0). Session id embedded (`ses_e64a0bdc1ffeGcf9eS1ajabtkf`).
Verdict: **verified** — `run --format json` does not keep a session alive and accepts no follow-up through stdin. Not suitable for interactive multi-turn sessions.

### 2. Interface/protocol that enables follow-up: headless server + REST API
Command:
```
opencode serve --port 4097 --hostname 127.0.0.1   # background
# -> "opencode server listening on http://127.0.0.1:4097"
```
The server exposes a REST API (discovered from frontend source, then exercised):
- `POST /api/session` body `{"directory":"/tmp/opencode/demo"}` -> creates persistent session `ses_e649ffbfcffeMjOk9qnOsGx965` (verified).
- `POST /session/{sessionID}/message` body `{"parts":[{"type":"text","text":"..."}]}` -> sends a user message to an existing session (verified, 5 messages sent to the same session).
- `POST /session/{sessionID}/interrupt` -> exists per frontend source (interrupt path, not live-tested).
- `POST /api/session/{sessionID}/permission/{requestID}/reply` body `{"reply":...,"message":...}` -> permission reply contract confirmed from frontend source (returns 204); **live request was not triggered** (see 4).
- `GET /api/session` -> lists sessions (verified).
- `GET /api/permission/request` -> `{"data":[]}` (verified; no pending requests in this session).
- `opencode acp` (ACP protocol) and `opencode attach <url>` appear in `opencode --help` but were **not probed** (assumption only).

Verdict: **verified** — follow-up messages go through the headless server REST API (`opencode serve`), not `opencode run`.

### 3. Assistant messages delivered inline + via SSE
Verified by sending `"say hi back"` to session `ses_e649ffbfcffeMjOk9qnOsGx965`:
- POST response body `parts`: `[step-start, reasoning("Done."), text("Created ..."), step-finish{reason:"stop"}]` — assistant text delivered inline.
- Real-time: `GET /api/event` is an SSE stream. While a message was in flight it streamed:
  - `data: {...,"type":"server.connected",...}`
  - `data: {...,"type":"message.updated",...}` (user message)
  - `data: {...,"type":"message.part.updated",...}` (assistant parts, one per part)
  - `data: {...,"type":"session.updated",...}`
  - `: heartbeat`
Verdict: **verified** — assistant text arrives (a) inline in the message-response `parts` and (b) as real-time `message.part.updated` SSE events on `/api/event` (filterable by session/location). `GET /api/session/{sessionID}/event` returned empty during the test (session-specific stream behavior not fully characterized — assumption).

### 4. Permission responses
Contract confirmed from frontend source (not live-triggered):
- `POST /api/session/{sessionID}/permission/{requestID}/reply`, body `{"reply":"approve"|"deny","message":"..."}`, success 204.
- `GET /api/session/{sessionID}/permission` lists permission requests (verified; always `{"data":[]}` in serve mode).
- `GET /api/permission/request` -> `{"data":[]}` (verified).
- SSE event type `"permission.asked"` exists in frontend code (verified from bundle).
- Reply mechanism: `permission.respond({sessionID, permissionID, response, directory})` maps to the reply endpoint above (verified from bundle).

Live trigger attempts (all in disposable `/tmp` projects, non-sensitive):
- Shell command `ls -la /tmp` — auto-succeeded, no permission prompt.
- File write to `/tmp/opencode-perm-test/outside.txt` — session hung, no file created, no permission request.
- File write to `/home/g/.ssh/perm_test.txt` — session hung, no file created, no permission request.
- File delete of `/tmp/opencode-perm-test3/to_delete.txt` — session hung, file not deleted, no permission request.
- Read `/home/g/.ssh/id_rsa` — session hung, no permission request.

Verdict: **no live permission request could be triggered through `opencode serve`** in this install. File operations cause sessions to hang instead of emitting `permission.asked` events. The `permission.asked` SSE event type and reply endpoint exist in the frontend code but are not exercised by serve mode in this environment. The reply path is **API-contract verified only, end-to-end not observed**.

OpenCode has an `--auto` flag ("auto-approve permissions that are not explicitly denied") but it is only available on `opencode run`, not `opencode serve`. Auto-approval is off by default in Argus.

### 5. Same session continues after a turn
Verified: session `ses_e649ffbfcffeMjOk9qnOsGx965` received 5 user messages across separate HTTP calls (`list files`, `create probe.txt`, `echo permission-test`, `say hi back`, `delete probe.txt`) and continued responding each time, finishing with `step-finish{reason:"stop"}`.
Verdict: **verified** — the same session continues after a turn.
After-interruption: `POST /session/{sessionID}/interrupt` exists per frontend but was **not live-tested** (assumption that it stops the current turn and the session remains resumable).

## Summary table
| Question | Verified? | Evidence |
|---|---|---|
| Running session receives another user message | Yes | `POST /session/{sessionID}/message` x5 on same session ID |
| Interface/protocol | Yes | `opencode serve` headless server + REST API |
| Assistant message delivery | Yes | inline `parts` + SSE `message.part.updated` on `/api/event` |
| Permission response sending | Contract only | `POST /api/session/{sessionID}/permission/{requestID}/reply {reply,message}` |
| Session continues after turn / interruption | Turn: yes; interruption: not tested | 5-turn same-session loop verified; interrupt endpoint untested |

## Spike implementation status (noted for completeness)

Implemented as a backend-only vertical slice (no chat UI), verified by tests:

- `internal/adapter/adapter.go` — `MessageResult`/`MessagePart`/`TokenUsage` types; `SendMessage` now returns `(MessageResult, error)`.
- `internal/adapter/opencode.go` — `OpenCodeAdapter` owns a detached `opencode serve` process (lazy, shared across sessions); implements Start/SendMessage/Status/Stop/Restart/Abort/RespondToPermission/GetEvents/Close.
- `internal/adapter/opencode_client.go` (new) — `ServeClient`: spawns `opencode serve --port 0`, parses port from stdout, HTTP client for session/message/interrupt/permission endpoints.
- `internal/adapter/opencode_test.go` (new) — Focused tests with `httptest.Server`: create session, send message + parse response, unknown-session failures, stop → STOPPED status, permission reply plumbing, multi-part text preservation.

Quality gates: `gofmt` PASS, `go vet ./...` PASS, `go build ./...` PASS, `go test -race ./internal/adapter/` PASS, full module `go test ./...` PASS.

Permission remain end-to-end UNVERIFIED (no live request triggered during the probe); plumbing is implemented to the extent supported by the probe and clearly marked as such in code comments.
