# Argus Deployment Guide

This guide covers building and deploying Argus for local development and production use.

## Prerequisites

- Go 1.22+
- Node.js 20+ (for desktop and PWA)
- Rust/Cargo (for Tauri desktop app)
- Supabase account (free tier works)
- OpenCode CLI installed

## Quick Start (Local Development)

### 1. Build the Go Service

```bash
cd services/agent-manager
go build -o agent-manager ./cmd/agent-manager
```

### 2. Configure Environment

Create `.env` in the repository root or service directory:

```bash
# Required for Supabase sync
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_ANON_KEY=your-anon-key
SUPABASE_SERVICE_KEY=your-service-role-key

# Required for Meta API (assistant supervision)
META_API_KEY=your-api-key
META_BASE_URL=https://api.openai.com/v1
META_MODEL=gpt-4o-mini

# Optional: Notifications
NOTIF_ENABLED=true
NOTIF_NTFY_TOPIC=argus-notifications
NOTIF_NTFY_URL=https://ntfy.sh

# Optional: Override defaults
AGENT_MANAGER_DB=argus.db
AGENT_MANAGER_PORT=8080
OPENCODE_BINARY=opencode
```

### 3. Apply Supabase Migrations

Run migrations in order against your Supabase database:

```bash
# Using Supabase CLI
supabase db push

# Or manually via Supabase SQL Editor (run in order):
# 1. supabase/migrations/0001_init.sql
# 2. supabase/migrations/0002_add_session_columns.sql
# 3. supabase/migrations/0003_mvp_tables.sql
```

Verify tables exist: `sessions`, `tasks`, `incidents`, `approvals`, `recovery_actions`, `events`, `commands`, `devices`, `projects`, `pairing_codes`, `notification_preferences`

### 4. Start the Go Service

```bash
cd services/agent-manager
./agent-manager --port=8080
```

Health check: `curl http://localhost:8080/health` → `{"status":"ok","started_at":...}`

### 5. Build and Run Desktop App

```bash
cd apps/desktop
npm install
npm run build
cd src-tauri
cargo build --release
./target/release/argus
```

The desktop app auto-starts the Go service on port 8080.

### 6. Build and Serve PWA

```bash
cd apps/pwa
npm install
npm run build
# Serve dist/ via HTTPS (required for PWA features):
npx serve -s dist -l 5173 --ssl-cert cert.pem --ssl-key key.pem
```

For local HTTPS, generate a self-signed cert:
```bash
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes -subj "/CN=localhost"
```

### 7. Pair Phone with Desktop

1. Open desktop app → click "Generate Pairing Code"
2. Open PWA on phone → enter 6-character code
3. Phone now mirrors desktop sessions

## Detailed Configuration

### Argus Assistant (Meta API)

The assistant supervision layer requires a Meta-compatible API (OpenAI, Ollama, LM Studio, etc.).

**Environment variables:**
- `META_API_KEY` - API key (required for live mode)
- `META_BASE_URL` - API base URL (e.g., `https://api.openai.com/v1` or `http://localhost:11434/v1`)
- `META_MODEL` - Model name (e.g., `gpt-4o-mini`, `llama3.1`)

**Behavior:**
- If both `META_API_KEY` and `META_BASE_URL` are set → Live mode
- Otherwise → Fallback mode (deterministic diagnosis)

**Desktop UI:** Settings → Model Configuration → Test Connection / Save

### Supabase Sync

The Go service syncs session data to Supabase using the **service role key** (backend only). The anon key is used only for PWA realtime subscriptions.

**Required environment:**
- `SUPABASE_URL` - Project URL
- `SUPABASE_ANON_KEY` - Anon key (for PWA realtime)
- `SUPABASE_SERVICE_KEY` - Service role key (for backend writes)

**RLS Policies:** Migrations create permissive policies for `authenticated` role. For production, scope to `auth.uid()` after integrating Supabase Auth.

### Notifications

Two delivery methods:
1. **ntfy.sh** (simpler) - Set `NOTIF_NTFY_TOPIC` and `NOTIF_ENABLED=true`
2. **Web Push (VAPID)** - Generate keys and configure VAPID settings

PWA notification preferences are stored in localStorage.

## Building Distributable Releases

### Desktop App (Tauri)

```bash
cd apps/desktop
npm run build
cd src-tauri
cargo tauri build --target x86_64-unknown-linux-gnu
# Output: src-tauri/target/release/bundle/
```

**Platform-specific notes:**
- Linux: Generates `.deb`, `.AppImage`, `.tar.gz`
- macOS: Generates `.dmg` (requires macOS build machine)
- Windows: Generates `.msi` (requires Windows build machine)

**Free CI options:** GitHub Actions (linux/macos/windows runners included in free tier)

### PWA

```bash
cd apps/pwa
npm run build
# Deploy dist/ to any static hosting with HTTPS:
# - Netlify, Vercel, Cloudflare Pages (free tiers)
# - GitHub Pages + custom domain + Cloudflare
```

**PWA Requirements:**
- HTTPS required for service worker
- `manifest.webmanifest` and `sw.js` must be served from root
- For offline support, ensure all assets are precached

## Phone-to-Laptop Remote Access

### Option A: Supabase Realtime (Recommended)

1. Supabase handles auth, database, and WebSocket relay
2. Laptop Go service initiates outbound connection to Supabase
3. Phone PWA connects to same Supabase project
4. No port forwarding, no public IP needed

**Security:** Pairing codes expire in 5 minutes. Devices can be revoked from phone.

### Option B: Tailscale / VPN (For local-network-only)

1. Install Tailscale on laptop and phone
2. Access laptop via Tailscale IP (e.g., `http://100.x.y.z:8080`)
3. No Supabase required for basic dashboard

### Option C: ngrok / Cloudflare Tunnel (Temporary)

```bash
# Quick tunnel for testing
npx ngrok http 8080
# Use the https://xxx.ngrok-free.app URL on phone
```

## Local Testing Checklist

- [ ] Go service builds: `go build ./...` in services/agent-manager
- [ ] Go tests pass: `go test -race ./...` in services/agent-manager
- [ ] Supabase migrations applied and verified
- [ ] Go service starts on port 8080
- [ ] Health endpoint returns 200 OK
- [ ] Desktop app builds: `npm run build && cargo build --release`
- [ ] Desktop app launches and connects to Go service
- [ ] PWA builds: `npm run build` in apps/pwa
- [ ] PWA serves over HTTPS
- [ ] Pairing code generated on desktop, accepted on PWA
- [ ] Session created on desktop appears on PWA
- [ ] Follow-up message sent from PWA appears on desktop
- [ ] Assistant supervision: configure Meta API in Settings, test connection
- [ ] Incident detection: create failing task, trigger diagnosis
- [ ] Report generation: click "Generate Report" in session chat, verify .md download
- [ ] Notifications: enable ntfy topic, trigger completion event
- [ ] Clean shutdown: Ctrl+C in Go service, verify sessions marked DISCONNECTED

## Shutdown Procedure

```bash
# Stop Go service
Ctrl+C in terminal running agent-manager

# Stop desktop app
Close window or Ctrl+C in Tauri terminal

# Stop PWA dev server
Ctrl+C in serve/ngrok terminal
```

Sessions survive Go service restart (marked DISCONNECTED, restorable from desktop).

## Migration Files Reference

| File | Purpose | Must Apply? |
|------|---------|-------------|
| `0001_init.sql` | Core tables (sessions, events, commands, devices, projects, pairing_codes, notification_preferences) | Yes |
| `0002_add_session_columns.sql` | Adds agent, model, pid, error_message, updated_at to sessions | Yes |
| `0003_mvp_tables.sql` | MVP tables (tasks, incidents, approvals, recovery_actions) | Yes |

**Order matters:** Apply 0001 → 0002 → 0003

## Security Notes

- API keys never leave the laptop (stored in Go service env, not in PWA bundle)
- Supabase service role key only used in Go backend
- PWA uses anon key for realtime subscriptions only
- Pairing codes are single-use, 5-minute expiry
- Web Push VAPID keys generated locally
- Report generation excludes secrets

## Troubleshooting

| Issue | Resolution |
|-------|------------|
| `opencode: command not found` | Install OpenCode or set `OPENCODE_BINARY` to full path |
| Dashboard shows no sessions | Check Go service on port 8080, verify `/api/dashboard` returns data |
| Supabase sync errors (42501) | Verify service role key is set, RLS policies allow `authenticated` |
| PWA pairing fails | Check Supabase URL/anon key, ensure `pairing_codes` table exists |
| Meta API "not configured" | Set `META_API_KEY` and `META_BASE_URL` in Go service env |
| Desktop app won't start | Ensure Go service binary exists at expected path, check Tauri logs |
| Report download fails | Check `/api/sessions/{id}/report` endpoint, verify Go service has store access |

## Architecture Summary

```
Phone (PWA) ←HTTPS/WSS→ Supabase ←outbound→ Laptop (Go Service) → OpenCode
                     ↑                              ↑
              realtime subscriptions          SQLite (local truth)
```

The laptop always initiates outbound connections. Nothing is publicly exposed.