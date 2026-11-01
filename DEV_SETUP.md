# Argus Local Development Environment

## Prerequisites

- Go 1.22+
- Node.js 20+
- Rust 1.75+ (for Tauri desktop app)
- SQLite3
- OpenCode CLI (`opencode` in PATH)

## Quick Start (Desktop App — recommended)

The Tauri desktop app will automatically start the Go sidecar if the binary is discoverable.

```bash
# 1. Build the Go service (once, and after any Go changes)
cd services/agent-manager
go build -o agent-manager ./cmd/agent-manager

# 2. Run the Tauri desktop app (starts Vite dev server + Go sidecar)
cd ../../apps/desktop
npm run tauri dev
```

The desktop window opens at `http://localhost:5173` (Vite) and proxies `/api/*` to the Go service on `http://localhost:8080`. The sidecar is launched from the built binary location (resolved automatically).

### Running the Go service standalone (for browser-only testing)

If you only want the HTTP API / static dashboard without the Tauri wrapper:

```bash
cd services/agent-manager
./agent-manager --port=8080
# Dashboard available at http://localhost:8080/
```

### Running the PWA (phone remote)

```bash
cd apps/pwa
npm run dev
# Served at http://localhost:5174 (HTTPS via vite dev server for PWA features)
```

## Environment Variables

Create a `.env` file in the repository root (or export in your shell). **Do not commit real secrets.**

```bash
# Supabase (optional - for cloud sync and PWA realtime)
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_ANON_KEY=your-anon-key
SUPABASE_SERVICE_KEY=your-service-role-key

# Meta API (optional - for AI diagnosis)
META_API_KEY=your-meta-api-key
META_BASE_URL=https://api.meta.example.com/v1
META_MODEL=meta-llama-3-70b

# Optional: force the desktop sidecar to use a specific Go binary
# ARGUS_AGENT_MANAGER_PATH=/absolute/path/to/agent-manager
```

Or pass the same values as CLI flags to the Go service (useful when running it manually):

```bash
./agent-manager \
  --port=8080 \
  --db=argus.db \
  --opencode=opencode \
  --supabase-url=https://your-project.supabase.co \
  --supabase-key=your-anon-key \
  --supabase-service-key=your-service-role-key \
  --meta-api-key=your-meta-api-key \
  --notif-enabled \
  --notif-ntfy-topic=argus-demo
```

**Required for which feature?**
- Supabase URL + anon key → PWA realtime sync, pairing codes, cloud dashboard mirror.
- Supabase service key → server‑side admin writes (pairing code cleanup).
- Meta API key → AI‑assisted incident diagnosis (fallback used if absent).
- Notification flags → push via Web Push (VAPID) or ntfy.sh fallback.

## Supabase Setup (only needed for PWA / cloud features)

1. Create a new Supabase project at https://supabase.com
2. Run the migration in `supabase/migrations/0001_init.sql` in the SQL editor
3. Enable Email/Password auth in Authentication → Providers
4. Add RLS policies for each table (see migration file for table names)
5. Copy the project URL and anon key to your `.env` (or export them)

## OpenCode Installation

```bash
# Install OpenCode (if not already installed)
# See https://github.com/opencode-ai/opencode for latest instructions

# Verify installation
opencode --version
```

## Project Structure

```
argus-agent-supervisor/
├── services/agent-manager/     # Go local service
│   ├── cmd/agent-manager/      # Entry point
│   ├── internal/
│   │   ├── adapter/            # AgentAdapter interface & OpenCode impl
│   │   ├── dashboard/          # HTTP API + static dashboard
│   │   ├── meta/               # Meta API client + fallback
│   │   ├── notification/       # Web Push + ntfy client
│   │   ├── session/            # Session/Task/Incident/Recovery logic
│   │   ├── state/              # Domain models
│   │   ├── store/              # SQLite persistence
│   │   ├── subprocess/         # OpenCode process management
│   │   └── supabase/           # Supabase sync client
│   ├── go.mod
│   └── agent-manager           # Built binary (git‑ignored)
├── apps/desktop/               # Tauri + React desktop app
│   ├── src/                    # React frontend
│   ├── src-tauri/              # Tauri backend (Rust)
│   ├── package.json
│   └── Cargo.toml
├── apps/pwa/                   # React PWA (phone remote)
│   ├── src/
│   ├── package.json
│   └── vite.config.ts
├── supabase/
│   └── migrations/             # Database schema
└── docs/
    ├── PRD.md                  # Product requirements
    ├── PROGRESS.md             # Implementation progress
    ├── MILESTONES.md           # Milestone definitions
    ├── DESIGN.md               # Design standards
    ├── BUILD_PLAN.md           # Current build plan
    └── DEMO_RUNBOOK.md         # Demo instructions
```

## Running Tests

```bash
# Go service tests
cd services/agent-manager
go test ./...
go test -race ./...
go vet ./...

# Desktop app (typecheck + lint)
cd apps/desktop
npm run build   # runs tsc + vite build

# PWA (typecheck + lint)
cd apps/pwa
npm run build
```

## Building for Production

```bash
# Go service
cd services/agent-manager
go build -o agent-manager ./cmd/agent-manager

# Desktop app (bundles sidecar automatically if binary present next to Rust exe)
cd apps/desktop
npm run build
cd src-tauri && cargo build --release

# PWA
cd apps/pwa
npm run build
```

## Common Issues

### OpenCode not found
Ensure `opencode` is in your PATH or specify with `--opencode=/path/to/opencode`

### Port 8080 already in use
Change the port with `--port=9000` (both Go service and Vite proxy must match).

### Database locked
Ensure only one instance of the Go service is running.

### Tauri build fails on Linux
Install dependencies: `sudo apt-get install libwebkit2gtk-4.1-dev libgtk-3-dev libayatana-appindicator3-dev librsvg2-dev`

### CORS errors when opening the Vite preview in a browser
The Vite dev server proxies `/api` to `http://localhost:8080`. Make sure the Go service is running on that port (the Tauri sidecar starts it automatically). If you run the Go service manually, start it **before** `npm run tauri dev`.

### Sidecar not found on first Tauri run
The sidecar resolver looks for `agent-manager` next to the Rust binary, then falls back to `../../../services/agent-manager/agent-manager`. After building the Go binary once, the fallback works. Alternatively export `ARGUS_AGENT_MANAGER_PATH=/full/path/to/agent-manager`.

### Stopping the app
- Close the Tauri window → sidecar process is killed automatically.
- If you started the Go service manually, stop it with `Ctrl‑C` in its terminal.
- `pkill -f agent-manager` cleans any stray processes.