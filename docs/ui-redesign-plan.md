# Argus UI Redesign Plan

**Branch:** `ui/linear-redesign` (from `hackathon-final` commit 65f5d91)
**Goal:** Design-only overhaul to Linear-style minimalist interface. No functional changes.

---

## 1. Current Architecture Summary

### Desktop Dashboard (apps/desktop)
- **Framework:** React 18 + TypeScript + Vite + Tailwind
- **Tauri 2** shell with Go sidecar (`agent-manager` binary)
- **Entry:** `src/main.tsx` → `src/App.tsx`
- **Components:** `SessionChat`, `NewSessionForm`, inline `SessionCard`, `SessionSection`, `StatusBadge`
- **API:** Polls Go service at `http://localhost:8080/api/dashboard` every 5s; `/api/events` every 3s
- **Static HTML fallback:** Served by Go at `internal/dashboard/static/index.html` (vanilla JS, used if React not built)

### Mobile PWA (apps/pwa)
- **Framework:** React 18 + TypeScript + Vite + Tailwind + Workbox (vite-plugin-pwa)
- **Entry:** `src/main.tsx` → `src/App.tsx`
- **Supabase:** Realtime subscriptions on `sessions`, `approvals` tables; REST for commands/events
- **Screens:** Pairing, Session List, Session Detail, Settings (notifications, Supabase config, device unpair)
- **Manifest:** `vite.config.ts` → PWA manifest with icons in `public/`
- **Service Worker:** Workbox auto-generated, caches assets + Supabase NetworkFirst

### Shared Styling (both apps)
- **Tailwind** with identical `tailwind.config.js` color palette:
  - `background: #1e1e1e`, `surface: #252526`, `surfaceHover: #2d2d2d`, `border: #333`
  - `text: #d4d4d4`, `textMuted: #888`
  - `accent: #0e639c`, `accentHover: #1177bb`
  - Semantic: `success: #4ec9b0`, `warning: #dcdcaa`, `error: #f44747`, `info: #9cdcfe`
- **Fonts:** System UI sans + `ui-monospace` stack
- **No CSS variables / design tokens** — colors hardcoded in Tailwind config
- **No dark/light mode** — dark only

### Status States (from `state.go` and duplicated in both frontends)
| State | Desktop/PWA Badge Colors |
|-------|-------------------------|
| WORKING/RUNNING/COMPLETED | Green bg/text/dot |
| STARTING | Blue |
| WAITING/PARKED | Amber |
| PAUSED | Gray |
| ERROR/CRASHED | Red |
| DISCONNECTED/DETACHED/STOPPED | Gray |
| RECOVERING/DIAGNOSING | Purple |
| PENDING/PROPOSED/DETECTED | Blue/Amber |

### Components Inventory
**Desktop:**
- Header (logo, connection indicator, New Session, Settings)
- Session sections: Live / Disconnected / Past (grid cards)
- SessionCard: project dir, agent, status badge, PID, Restore/Stop/Dismiss buttons
- SessionChat: conversation timeline, tool activity (details), GUS assistant panel, composer
- NewSessionForm modal: directory picker (Tauri), prompt textarea
- Settings modal: Supervision toggle, Meta API config (base URL, key, model), Supabase info, Config persistence
- Error/Success banners

**PWA:**
- Pairing screen (6-char code input)
- Header tabs: Sessions / Settings
- SessionList: cards with status, approvals (disabled buttons + notice)
- SessionDetailScreen: conversation timeline, composer
- SettingsScreen: Device name, Supabase URL/Key, 7 notification toggles (localStorage), Unpair

### Risk Points (selectors/DOM hooks that must stay stable)
- `data-testid` not used; tests use `getByRole`, `getByText`, `getByPlaceholderText`
- WebSocket/SSE: none in frontend (desktop polls REST, PWA uses Supabase Realtime `channel.on('postgres_changes')`)
- Class names used by tests: none directly; but Tailwind utilities in components
- `StatusBadge` component duplicated in both apps — must keep in sync
- `statusBadge()` function duplicated — single source needed
- Tauri `@tauri-apps/plugin-dialog` `open()` for directory picker
- PWA `localStorage` keys: `argus_paired`, `argus_device_id`, `argus_user_id`, `argus_supabase_url`, `argus_supabase_key`, `argus_device_name`, `argus_notif_prefs`
- PWA Supabase channel name: `'argus-changes'`

### Tests Baseline
| Package | Command | Result |
|---------|---------|--------|
| Desktop | `npm run test` | 10 passed (SessionChat) |
| PWA | `npm run test` | 12 passed (fetchSessionEvents, sendCommandToSession, Settings, SessionList) |
| Go service | `go test ./...` | 10 packages OK |
| Go service | `go test -race ./...` | All pass (23s) |

### Fonts, Icons, Images
- **Fonts:** System UI stack only (no web fonts, works offline)
- **Icons:** Inline SVG (no icon library, no CDN)
- **Images:** PWA manifest icons in `public/` (192x192, 512x512, apple-touch-icon, favicon) — self-hosted
- **Offline:** PWA caches all static assets via Workbox; Supabase calls NetworkFirst with 24h cache

---

## 2. Theme System Plan

### CSS Variables (Design Tokens)
Introduce `:root` variables in `index.css` (both apps) for:
- **Colors:** `--bg`, `--surface`, `--surface-hover`, `--border`, `--text`, `--text-muted`, `--accent`, `--accent-hover`, `--success`, `--warning`, `--error`, `--info`
- **Semantic status:** `--status-working`, `--status-waiting`, `--status-error`, `--status-disconnected`, `--status-recovering`, `--status-pending`
- **Typography:** `--font-sans`, `--font-mono`, `--text-base`, `--text-sm`, `--text-xs`
- **Spacing:** `--space-1`..`--space-8` (map to 4px increments)
- **Radius:** `--radius-sm`, `--radius-md`, `--radius-lg`
- **Shadows:** `--shadow-sm`, `--shadow-md`, `--shadow-lg` (subtle elevation)
- **Transitions:** `--transition-fast`, `--transition-normal`

### Light Mode
Add `@media (prefers-color-scheme: light)` override with inverted palette:
- `--bg: #ffffff`, `--surface: #f5f5f5`, `--surface-hover: #eeeeee`, `--border: #dddddd`
- `--text: #1a1a1a`, `--text-muted: #666666`
- Accent stays `#0e639c` (accessible on both)

### Toggle & Persistence
- Add `ThemeProvider` context in each app
- Toggle button in header (desktop) / settings (PWA)
- Persist to `localStorage` (`argus_theme`: 'light' | 'dark' | 'system')
- Apply `data-theme` attribute on `<html>` for CSS selector scoping
- Default: system preference

### Migration Strategy
1. Add CSS variables to `index.css` (both apps) — maps 1:1 to current Tailwind colors
2. Update `tailwind.config.js` to reference CSS variables via `theme.extend.colors` using `rgb(var(--color) / <alpha-value>)` pattern
3. Replace hardcoded Tailwind classes in components with semantic variable-based classes (e.g., `bg-surface` → `bg-[var(--surface)]`)
4. Add `ThemeProvider` + toggle UI
5. Verify tests pass unchanged (they test behavior, not classes)

---

## 3. Component-by-Component Visual Changes (Ordered by Risk)

| Priority | Component | Changes | Risk |
|----------|-----------|---------|------|
| 1 | `StatusBadge` + `statusBadge()` | Single shared component; CSS variable colors; 1px border; denser padding | Low (isolated, tested) |
| 2 | Desktop `SessionCard` | Hairline border, tighter padding, status badge inline, button group aligned right | Medium (interaction targets) |
| 3 | Desktop `SessionSection` grid | Reduce gap, denser cards, remove section note text | Low |
| 4 | Desktop Header | Logo typography, connection pill smaller, toggle theme button | Low |
| 5 | Desktop `SessionChat` | Conversation density, monospace timestamps, tool activity summary bar, GUS panel elevation | Medium (many elements) |
| 6 | Desktop `NewSessionForm` modal | Compact inputs, focus ring via CSS var | Low |
| 7 | Desktop Settings modal | Form density, toggle switches, config status pills | Low |
| 8 | PWA PairingScreen | Centered card, mono code input, subtle focus | Low |
| 9 | PWA Header tabs | Underline indicator, tighter | Low |
| 10 | PWA `SessionList` card | Match desktop card density, approvals collapsed by default | Medium |
| 11 | PWA `SessionDetailScreen` | Match desktop chat density | Low |
| 12 | PWA SettingsScreen | Grouped sections, toggle density | Low |
| 13 | Empty/Loading/Error states | Consistent skeleton/empty illustrations (SVG) | Low |
| 14 | Modals/Drawers | Backdrop blur removed, hairline border, consistent radius | Low |

**Design tokens applied throughout:**
- 1px borders (`border-[var(--border)]`)
- 4px/8px radius (`rounded-[var(--radius-md)]`)
- Subtle shadows on elevated panels (`shadow-[var(--shadow-sm)]`)
- Accent only for primary actions and attention states
- Motion: 150ms ease-out on hover/focus/state changes

---

## 4. Selectors/DOM Hooks That Must Stay Stable

| Selector / Hook | Location | Reason |
|-----------------|----------|--------|
| `getByRole('button', { name: 'Send' })` | SessionChat, SessionDetailScreen | Test |
| `getByPlaceholderText(/type a follow-up message/i)` | SessionChat, SessionDetailScreen | Test |
| `getByRole('button', { name: 'Interrupt current work' })` | SessionChat | Test |
| `getByRole('button', { name: 'End session' })` | SessionChat | Test |
| `getByRole('button', { name: 'Approve' })` | SessionList | Test (disabled) |
| `getByRole('button', { name: 'Reject' })` | SessionList | Test (disabled) |
| `getByRole('checkbox', { name: /completed|error|.../i })` | SettingsScreen | Test |
| `getByRole('button', { name: /settings/i })` | App header | Test |
| `localStorage` keys (7 keys) | PWA | Persistence |
| Supabase channel `'argus-changes'` | PWA App | Realtime |
| Tauri `open({ directory: true })` | NewSessionForm | Native dialog |
| `StatusBadge` component props | Both apps | Shared status display |

**Do not change:** component prop interfaces, event handler names, API call shapes, localStorage keys, Supabase table/channel names.

---

## 5. Verification Plan

### Baseline Screenshots (before changes)
Capture every distinct screen/state in both apps:
1. Desktop: Empty dashboard, Live sessions grid, Disconnected section, Past section
2. Desktop: SessionChat (empty, with messages, with errors, with tool activity open, GUS panel open)
3. Desktop: NewSessionForm (empty, with error, browser mode)
4. Desktop: Settings modal (unconfigured, configured, tested, saved, error)
5. Desktop: Error banner, success toast, loading spinner
6. PWA: Pairing screen (empty, with error)
7. PWA: Session list (empty, with sessions, with approvals)
8. PWA: SessionDetail (empty, with messages)
9. PWA: Settings (all sections)
10. Both: Light mode (after toggle), Dark mode

### After Changes
- Same screenshots, visual diff
- All existing tests pass (`npm run test` both apps, `go test -race ./...`)
- No console errors in dev tools
- PWA builds and installs (`npm run build` → verify manifest, SW)
- Tauri dev runs (`npm run tauri dev`)

---

## 6. Implementation Sequence

1. **Shared tokens:** Create `packages/ui-tokens` (or shared CSS file) with CSS variables; import in both `index.css`
2. **Tailwind config:** Update both `tailwind.config.js` to use CSS variables
3. **StatusBadge:** Extract to shared component (or keep duplicated but single-source function)
4. **Desktop:** App.tsx → SessionCard → SessionChat → NewSessionForm → Settings → Header
5. **PWA:** PairingScreen → Header → SessionList → SessionDetail → Settings
6. **ThemeProvider:** Add to both apps, wire toggle, persist
7. **Screenshots:** Baseline → after → diff
8. **Tests:** Run full suite, fix any act() warnings

---

## 7. Out of Scope (Explicit)
- New features or behavior changes
- Animation library (use CSS transitions only)
- Icon library (keep inline SVG)
- Web fonts (keep system stack)
- Component library extraction (keep in-app for now)
- Go service changes
- Supabase schema changes