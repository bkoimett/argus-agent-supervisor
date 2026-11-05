import { useState, useEffect, useCallback, useMemo } from 'react'
import { NewSessionForm } from './NewSessionForm'
import SessionChat from './SessionChat'

// Types
interface Task {
  id: string
  session_id: string
  prompt: string
  status: string
  dependencies: string[]
  scope: string
  started_at: string
  updated_at: string
  ended_at: string | null
  exit_code: number | null
  error_message: string | null
  approval_id: string | null
  recovery_count: number
}

interface Incident {
  id: string
  session_id: string
  task_id: string | null
  type: string
  severity: string
  status: string
  evidence: string
  meta_diagnosis: string | null
  meta_recovery: string | null
  detected_at: string
  diagnosed_at: string | null
  proposed_at: string | null
  approved_at: string | null
  recovered_at: string | null
  verified_at: string | null
  recovery_attempts: number
}

interface Approval {
  id: string
  session_id: string
  task_id: string | null
  incident_id: string | null
  type: string
  status: string
  title: string
  description: string
  payload: string
  scope: string
  requested_at: string
  expires_at: string
  responded_at: string | null
  decision: boolean | null
  decided_by: string | null
}

interface Event {
  session_id: string
  type: string
  timestamp: string
  payload: Record<string, unknown>
}

interface DashboardData {
  id: string
  project_dir: string
  agent: string
  status: string
  pid: number | null
  tasks: Task[]
  incidents: Incident[]
  approvals: Approval[]
  recent_events: Event[]
  recovery_actions: unknown[]
  supervision_enabled?: boolean
}

async function setSessionSupervision(sessionId: string, enabled: boolean): Promise<boolean> {
  const res = await fetch(`${API_BASE}/api/sessions/${sessionId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ supervision_enabled: enabled }),
  })
  if (!res.ok) throw new Error('Failed to update supervision')
  const d = await res.json()
  return !!d.supervision_enabled
}

const API_BASE = 'http://localhost:8080'

async function fetchDashboard(): Promise<DashboardData[]> {
  const res = await fetch(`${API_BASE}/api/dashboard`)
  if (!res.ok) throw new Error('Failed to fetch dashboard')
  return res.json()
}

async function createSession(projectDir: string, prompt: string): Promise<{ session_id: string; session: DashboardData }> {
  const res = await fetch(`${API_BASE}/api/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ project_dir: projectDir, prompt }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || 'Failed to create session')
  }
  return res.json()
}

async function apiHealth(): Promise<{ status: string; started_at: number } | null> {
  try {
    const res = await fetch(`${API_BASE}/health`, { signal: AbortSignal.timeout(2000) })
    if (!res.ok) return null
    return res.json()
  } catch {
    return null
  }
}

async function stopSession(sessionId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/sessions/${sessionId}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error('Failed to stop session')
}

async function restoreSession(sessionId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/sessions/${sessionId}/restore`, {
    method: 'POST',
  })
  if (!res.ok) throw new Error('Failed to restore session')
}

async function fetchMetaConfig(): Promise<{ baseUrl: string; model: string; hasKey: boolean; configured: boolean } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/meta/config`)
    if (!res.ok) return null
    const d = await res.json()
    // Backend uses snake_case.
    const baseUrl = d.base_url ?? ''
    const model = d.model ?? ''
    const hasKey = !!d.has_key
    const configured = typeof d.configured === 'boolean' ? d.configured : (baseUrl !== '' && model !== '' && hasKey)
    return { baseUrl, model, hasKey, configured }
  } catch {
    return null
  }
}

type MetaResult = { success: boolean; error?: string; requestId?: string; baseUrl?: string; model?: string; hasKey?: boolean; configured?: boolean }

async function parseMetaResult(res: Response): Promise<MetaResult> {
  let d: Record<string, unknown> = {}
  try { d = await res.json() } catch { /* non-JSON error body */ }
  const requestId = (d.request_id as string) || res.headers.get('X-Request-Id') || undefined
  if (!res.ok || d.success === false) {
    const detail = (d.error as string) || `Request failed (HTTP ${res.status})`
    return { success: false, error: requestId ? `${detail} [request ${requestId}]` : detail, requestId }
  }
  return { success: true, requestId, baseUrl: d.base_url as string, model: d.model as string, hasKey: d.has_key as boolean, configured: d.configured as boolean }
}

async function testMetaConnection(baseUrl: string, apiKey: string, model: string): Promise<MetaResult> {
  try {
    const res = await fetch(`${API_BASE}/api/meta/test-connection`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ base_url: baseUrl, api_key: apiKey, model }),
    })
    return parseMetaResult(res)
  } catch (err) {
    return { success: false, error: err instanceof Error ? err.message : 'Connection failed' }
  }
}

async function saveMetaConfig(baseUrl: string, apiKey: string, model: string): Promise<MetaResult> {
  try {
    const res = await fetch(`${API_BASE}/api/meta/config`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ base_url: baseUrl, api_key: apiKey, model }),
    })
    return parseMetaResult(res)
  } catch (err) {
    return { success: false, error: err instanceof Error ? err.message : 'Save failed' }
  }
}

function statusClass(status: string): string {
  const s = status.toLowerCase()
  switch (s) {
    case 'running': return 'bg-green-500/20 text-green-400'
    case 'completed': return 'bg-green-500/20 text-green-400'
    case 'starting': return 'bg-blue-500/20 text-blue-400'
    case 'waiting': return 'bg-yellow-500/20 text-yellow-400'
    case 'parked': return 'bg-amber-500/20 text-amber-400'
    case 'paused': return 'bg-gray-500/20 text-gray-400'
    case 'error': return 'bg-red-500/20 text-red-400'
    case 'crashed': return 'bg-red-500/20 text-red-400'
    case 'disconnected': return 'bg-gray-500/20 text-gray-400'
    case 'stopped': return 'bg-gray-500/20 text-gray-400'
    case 'detached': return 'bg-gray-500/20 text-gray-400'
    case 'pending': return 'bg-blue-500/20 text-blue-400'
    case 'recovering': return 'bg-purple-500/20 text-purple-400'
    case 'verified': return 'bg-green-500/20 text-green-400'
    case 'failed': return 'bg-red-500/20 text-red-400'
    case 'rejected': return 'bg-gray-500/20 text-gray-400'
    case 'approved': return 'bg-green-500/20 text-green-400'
    case 'proposed': return 'bg-blue-500/20 text-blue-400'
    case 'diagnosing': return 'bg-purple-500/20 text-purple-400'
    case 'detected': return 'bg-yellow-500/20 text-yellow-400'
    default: return 'bg-gray-500/20 text-gray-400'
  }
}


// GusControls is the per-session GUS control set, used both on session cards
// and in the opened session view, so GUS access does not disappear when a
// session is opened. Panel visibility is local UI state persisted per
// session; supervision is real backend state persisted per session.
function GusControls({
  session,
  metaConfigured,
  onOpenSettings,
  onRefresh,
}: {
  session: DashboardData
  metaConfigured: boolean | null
  onOpenSettings: () => void
  onRefresh: () => void
}) {
  const gusStorageKey = `argus_gus_visible_${session.id}`
  const [gusVisible, setGusVisible] = useState<boolean>(() => {
    try { return localStorage.getItem(gusStorageKey) === 'true' } catch { return false }
  })
  const [supervision, setSupervision] = useState<boolean>(!!session.supervision_enabled)
  const [supervisionError, setSupervisionError] = useState<string | null>(null)
  useEffect(() => { setSupervision(!!session.supervision_enabled) }, [session.supervision_enabled])
  const toggleGus = (e: React.MouseEvent) => {
    e.stopPropagation()
    setGusVisible((v) => {
      try { localStorage.setItem(gusStorageKey, String(!v)) } catch { /* storage unavailable */ }
      return !v
    })
  }
  const toggleSupervision = async (e: React.ChangeEvent<HTMLInputElement>) => {
    e.stopPropagation()
    const next = e.target.checked
    setSupervision(next)
    setSupervisionError(null)
    try {
      const confirmed = await setSessionSupervision(session.id, next)
      setSupervision(confirmed)
      onRefresh()
    } catch {
      setSupervision(!next)
      setSupervisionError('Could not update supervision. Is the Go service running?')
    }
  }
  return (
      <div className="p-4 pt-0" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-3 flex-wrap">
          <button
            onClick={toggleGus}
            aria-expanded={gusVisible}
            className="px-3 py-1 text-sm bg-surfaceHover hover:bg-border border border-border rounded focus:outline-none focus:ring-2 focus:ring-accent"
          >
            {gusVisible ? 'Hide GUS' : 'Show GUS'}
          </button>
          <label className="flex items-center gap-2 text-sm cursor-pointer">
            <input
              type="checkbox"
              checked={supervision}
              onChange={toggleSupervision}
              disabled={!metaConfigured}
              aria-label={`GUS supervision for session in ${session.project_dir}`}
              className="w-4 h-4 rounded border-border bg-surface text-accent focus:ring-accent disabled:opacity-50"
            />
            <span>Supervision</span>
          </label>
          {!metaConfigured && <span className="text-xs text-textMuted">Supervision unavailable until a model is configured.</span>}
          {supervisionError && <span role="alert" className="text-xs text-red-400">{supervisionError}</span>}
        </div>
        {gusVisible && (
          <div className="mt-3 p-3 bg-surface/50 border border-border/50 rounded text-sm" aria-label="GUS assistant panel">
            {!metaConfigured ? (
              <>
                <p className="text-textMuted">GUS is not configured, so the assistant cannot diagnose this session or propose recovery actions. No assistant response will be generated.</p>
                <button onClick={onOpenSettings} className="mt-2 px-3 py-1 text-sm bg-surfaceHover hover:bg-border border border-border rounded focus:outline-none focus:ring-2 focus:ring-accent">Configure assistant in Settings</button>
              </>
            ) : supervision ? (
              <p className="text-textMuted">Supervision is on for this session. GUS will diagnose incidents and propose recovery actions for approval. {(session.incidents ?? []).length} incident{(session.incidents ?? []).length === 1 ? '' : 's'} recorded for this session.</p>
            ) : (
              <p className="text-textMuted">Supervision is off for this session. Turn it on above to let GUS diagnose incidents and propose recovery actions.</p>
            )}
          </div>
        )}
      </div>
  )
}

function SessionCard({
  session,
  onRefresh,
  onSelect,
  metaConfigured,
  onOpenSettings,
}: {
  session: DashboardData
  onRefresh: () => void
  onSelect?: () => void
  metaConfigured: boolean | null
  onOpenSettings: () => void
}) {
  const handleRestore = () => {
    restoreSession(session.id).then(onRefresh)
  }

  const handleStop = () => {
    stopSession(session.id).then(onRefresh)
  }

  return (
    <div className="bg-surface border border-border rounded-lg overflow-hidden cursor-pointer hover:border-accent/50 transition-border" onClick={onSelect}>
      {/* Session Header */}
      <div className="p-4 border-b border-border flex flex-wrap items-center gap-4">
        <div className="flex-1 min-w-[200px]">
          <div className="font-mono text-sm text-textMuted">{session.project_dir}</div>
          <div className="font-medium">{session.agent}</div>
        </div>
        <div className="flex items-center gap-2">
          <span className={`px-2 py-1 rounded text-xs font-medium ${statusClass(session.status)}`}>
            {session.status}
          </span>
          {session.pid && (
            <span className="font-mono text-xs text-textMuted">PID: {session.pid}</span>
          )}
        </div>
        <div className="flex gap-2">
          {session.status === 'DISCONNECTED' || session.status === 'STOPPED' || session.status === 'DETACHED' ? (
            <>
              <button
                onClick={(e) => {
                  e.stopPropagation()
                  handleRestore()
                }}
                className="px-3 py-1 text-sm bg-blue-500/20 hover:bg-blue-500/30 text-blue-400 border border-blue-500/30 rounded"
              >
                Restore
              </button>
              {session.status === 'DISCONNECTED' && (
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    handleStop()
                  }}
                  className="px-3 py-1 text-sm bg-gray-500/20 hover:bg-gray-500/30 text-gray-400 border border-gray-500/30 rounded"
                >
                  Dismiss
                </button>
              )}
              {session.status === 'DETACHED' && (
                <p className="text-xs text-textMuted mt-1">
                  Argus stopped tracking this session but cannot confirm the OpenCode process ended. It may still be running.
                </p>
              )}
            </>
          ) : (
            <button
              onClick={(e) => {
                e.stopPropagation()
                handleStop()
              }}
              disabled={session.status === 'COMPLETED' || session.status === 'STOPPED' || session.status === 'ERROR' || session.status === 'DETACHED'}
              className="px-3 py-1 text-sm bg-red-500/20 hover:bg-red-500/30 text-red-400 border border-red-500/30 rounded disabled:opacity-50"
            >
              Stop
            </button>
          )}
        </div>
      </div>
      <GusControls session={session} metaConfigured={metaConfigured} onOpenSettings={onOpenSettings} onRefresh={onRefresh} />
    </div>
  )
}

const LIVE_STATES = new Set(['STARTING', 'RUNNING', 'WAITING', 'PARKED', 'PAUSED'])
const DISCONNECTED_STATES = new Set(['DISCONNECTED', 'DETACHED'])
const PAST_STATES = new Set(['COMPLETED', 'ERROR', 'CRASHED', 'STOPPED'])

function sessionKind(status: string): 'live' | 'disconnected' | 'past' {
  const s = status.toUpperCase()
  if (LIVE_STATES.has(s)) return 'live'
  if (DISCONNECTED_STATES.has(s)) return 'disconnected'
  if (PAST_STATES.has(s)) return 'past'
  // Unknown status: Argus cannot confirm the OpenCode process stopped,
  // so treat it honestly as disconnected rather than past/stopped.
  return 'disconnected'
}

function SessionSection({
  title,
  sessions,
  onRefresh,
  onSelect,
  note,
  metaConfigured,
  onOpenSettings,
}: {
  title: string
  sessions: DashboardData[]
  onRefresh: () => void
  onSelect: (session: DashboardData) => void
  note?: string
  metaConfigured: boolean | null
  onOpenSettings: () => void
}) {
  if (sessions.length === 0) return null

  return (
    <section className="mb-8">
      <div className="flex items-center gap-2 mb-3">
        <h2 className="text-sm font-medium text-textMuted uppercase tracking-wider">{title}</h2>
        {note && (
          <span className="text-xs text-textMuted bg-surface/50 px-2 py-0.5 rounded border border-border/50">
            {note}
          </span>
        )}
      </div>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {sessions.map((session) => (
          <SessionCard
            key={session.id}
            session={session}
            onRefresh={onRefresh}
            onSelect={() => onSelect(session)}
            metaConfigured={metaConfigured}
            onOpenSettings={onOpenSettings}
          />
        ))}
      </div>
    </section>
  )
}

export default function App() {
  const [sessions, setSessions] = useState<DashboardData[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [apiConnected, setApiConnected] = useState<boolean | null>(null)
  const [showNewSession, setShowNewSession] = useState(false)
  const [selectedSession, setSelectedSession] = useState<DashboardData | null>(null)
  const [createError, setCreateError] = useState<string | null>(null)
  const [createSuccess, setCreateSuccess] = useState<string | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [metaConfig, setMetaConfig] = useState<{
    baseUrl: string
    apiKey: string
    model: string
  }>({ baseUrl: '', apiKey: '', model: '' })
  const [metaSaveError, setMetaSaveError] = useState<string | null>(null)
  const [metaSaveSuccess, setMetaSaveSuccess] = useState<string | null>(null)
  const [metaTestError, setMetaTestError] = useState<string | null>(null)
  const [metaTestSuccess, setMetaTestSuccess] = useState<string | null>(null)
  const [metaTesting, setMetaTesting] = useState(false)
  const [metaSaving, setMetaSaving] = useState(false)
  const [metaConfigured, setMetaConfigured] = useState<boolean | null>(null)
  const [metaHasKey, setMetaHasKey] = useState(false)
  const [savedMeta, setSavedMeta] = useState<{ baseUrl: string; model: string } | null>(null)
  const metaDirty = savedMeta !== null && (savedMeta.baseUrl !== metaConfig.baseUrl.trim().replace(/\/$/, '') || savedMeta.model !== metaConfig.model.trim())

  // Single status source: the backend /api/meta/config response. Refreshed
  // on mount and periodically, so the dashboard panel and Settings never
  // disagree, and a slow-starting service does not leave a stale
  // "not configured" state. Form fields are only populated from the server
  // when the user has no unsaved edits.
  const metaDirtyRef = useCallback((cfg: { baseUrl: string; model: string }, saved: { baseUrl: string; model: string } | null) =>
    saved !== null && (saved.baseUrl !== cfg.baseUrl.trim().replace(/\/$/, '') || saved.model !== cfg.model.trim()), [])
  const refreshMetaConfig = useCallback(async (populateForm: boolean) => {
    const config = await fetchMetaConfig()
    if (!config) return // keep last known state; do not flap to "not configured"
    setMetaConfigured(config.configured)
    setMetaHasKey(config.hasKey)
    setSavedMeta({ baseUrl: config.baseUrl, model: config.model })
    if (populateForm) {
      setMetaConfig((prev) => (metaDirtyRef(prev, savedMeta) ? prev : { baseUrl: config.baseUrl, apiKey: prev.apiKey, model: config.model }))
    }
  }, [metaDirtyRef, savedMeta])
  useEffect(() => { refreshMetaConfig(true) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const iv = setInterval(() => refreshMetaConfig(false), 10000)
    return () => clearInterval(iv)
  }, [refreshMetaConfig])

  const liveSessions = useMemo(
    () => (sessions ?? []).filter((s) => sessionKind(s.status) === 'live'),
    [sessions]
  )
  const disconnectedSessions = useMemo(
    () => (sessions ?? []).filter((s) => sessionKind(s.status) === 'disconnected'),
    [sessions]
  )
  const pastSessions = useMemo(
    () => (sessions ?? []).filter((s) => sessionKind(s.status) === 'past'),
    [sessions]
  )

  const loadDashboard = useCallback(async () => {
    let ok = false
    try {
      setLoading(true)
      const data = await fetchDashboard()
      setSessions(data)
      setError(null)
      ok = true
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
      setApiConnected(ok)
    }
  }, [])

  useEffect(() => {
    loadDashboard()
    const interval = setInterval(loadDashboard, 5000)
    return () => clearInterval(interval)
  }, [loadDashboard])

  useEffect(() => {
    // Periodically confirm the Go service is reachable so the UI never
    // silently presents stale data as a fresh connection.
    const check = () => {
      apiHealth().then((h) => setApiConnected(h !== null))
    }
    check()
    const iv = setInterval(check, 10000)
    return () => clearInterval(iv)
  }, [])

  const handleNewSession = useCallback(async (dir: string, prompt: string) => {
    setCreateError(null)
    setCreateSuccess(null)
    try {
      await createSession(dir, prompt)
      setShowNewSession(false)
      setCreateSuccess('Session created.')
      setTimeout(() => setCreateSuccess(null), 3000)
      loadDashboard()
    } catch (err) {
      // Keep the modal open and surface the error inline in the form.
      setCreateError(err instanceof Error ? err.message : 'Failed to create session')
    }
  }, [loadDashboard])

  const handleCloseNewSession = useCallback(() => {
    setShowNewSession(false)
    setCreateError(null)
    setCreateSuccess(null)
  }, [])

  if (loading && sessions.length === 0 && !showNewSession) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-4 border-accent border-t-transparent mx-auto mb-4"></div>
          <p className="text-textMuted">Loading dashboard...</p>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen p-6">
      <div className="max-w-7xl mx-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-medium">Argus Dashboard</h1>
            {apiConnected === null ? (
              <span className="text-xs text-textMuted animate-pulse">connecting…</span>
            ) : apiConnected ? (
              <span className="flex items-center gap-1 text-xs text-green-400">
                <span className="w-1.5 h-1.5 bg-green-500 rounded-full"></span>
                connected
              </span>
            ) : (
              <span className="flex items-center gap-1 text-xs text-red-400">
                <span className="w-1.5 h-1.5 bg-red-500 rounded-full"></span>
                Go service unreachable
              </span>
            )}
          </div>
          <button
            onClick={() => setShowNewSession(true)}
            className="px-4 py-2 bg-accent hover:bg-accentHover text-white rounded font-medium"
          >
            New Session
          </button>
          <button
            onClick={() => setShowSettings(true)}
            className="px-4 py-2 bg-surfaceHover hover:bg-border border border-border rounded font-medium ml-2"
          >
            Settings
          </button>
        </div>

        {/* Error Banner */}
        {error && (
          <div className="mb-4 p-4 bg-red-500/20 border border-red-500/30 rounded text-red-400 flex items-center justify-between">
            <span>{error}</span>
            <button onClick={() => setError(null)} className="text-red-400 hover:text-red-300">×</button>
          </div>
        )}

        {/* Create-session success */}
        {createSuccess && (
          <div className="mb-4 p-4 bg-green-500/10 border border-green-500/20 rounded text-green-400 flex items-center justify-between">
            <span>{createSuccess}</span>
            <button onClick={() => setCreateSuccess(null)} className="text-green-400 hover:text-green-300">×</button>
          </div>
        )}

        {/* Argus assistant (GUS): always visible, even when unconfigured */}
        <section aria-label="Argus assistant" className="mb-6 p-4 bg-surface border border-border rounded">
          <div className="flex items-center justify-between gap-4 flex-wrap">
            <div>
              <h2 className="text-sm font-medium">Argus assistant</h2>
              {metaConfigured === null ? (
                <p className="text-sm text-textMuted mt-1">Checking assistant configuration...</p>
              ) : metaConfigured ? (
                <p className="text-sm text-textMuted mt-1">
                  Configured{metaConfig.model ? ` with model ${metaConfig.model}` : ''}{metaConfig.baseUrl ? ` at ${metaConfig.baseUrl}` : ''}. Use Test Connection in Settings to verify the provider responds.
                </p>
              ) : (
                <p className="text-sm text-textMuted mt-1">
                  Not configured. The assistant cannot diagnose incidents or propose recovery actions until a Meta-compatible model is set up. No assistant response will be generated.
                </p>
              )}
            </div>
            <button
              onClick={() => setShowSettings(true)}
              className="px-4 py-2 bg-surfaceHover hover:bg-border border border-border rounded font-medium focus:outline-none focus:ring-2 focus:ring-accent"
            >
              {metaConfigured ? 'Assistant settings' : 'Configure assistant in Settings'}
            </button>
          </div>
        </section>

        {/* Sessions by lifecycle: live, disconnected, past */}
        {!showNewSession && (
          <>
            {selectedSession ? (
              <div className="max-w-4xl mx-auto">
                <button
                  onClick={() => setSelectedSession(null)}
                  className="mb-4 inline-flex items-center gap-2 text-sm text-textMuted hover:text-text"
                >
                  ← Back to sessions
                </button>
                <GusControls session={selectedSession} metaConfigured={metaConfigured} onOpenSettings={() => setShowSettings(true)} onRefresh={loadDashboard} />
                <SessionChat
                  sessionId={selectedSession.id}
                  projectDir={selectedSession.project_dir}
                  agent={selectedSession.agent}
                  status={selectedSession.status}
                  initialPrompt={selectedSession.tasks?.[0]?.prompt}
                  onStatusChange={(s) => {
                    setSelectedSession((prev) =>
                      prev ? { ...prev, status: s } : null
                    )
                  }}
                  onClose={() => setSelectedSession(null)}
                />
              </div>
            ) : (
              <>
                {loading && sessions.length === 0 ? (
                  <div className="text-center py-12 text-textMuted">
                    <p className="text-lg">Loading dashboard...</p>
                  </div>
                ) : (
                  (sessions ?? []).length === 0 ? (
                    <div className="text-center py-12 text-textMuted">
                      <p className="text-lg">No sessions</p>
                      <p className="text-sm mt-1">Click "New Session" to start an OpenCode agent</p>
                    </div>
                  ) : (
                    <>
                      <SessionSection
                        title="Live"
                        sessions={liveSessions}
                        onRefresh={loadDashboard}
                        onSelect={(s) => setSelectedSession(s)}
                        metaConfigured={metaConfigured}
                        onOpenSettings={() => setShowSettings(true)}
                        note="Argus is actively managing these sessions. Status updates as OpenCode emits events."
                      />
                      <SessionSection
                        title="Disconnected"
                        sessions={disconnectedSessions}
                        onRefresh={loadDashboard}
                        onSelect={(s) => setSelectedSession(s)}
                        metaConfigured={metaConfigured}
                        onOpenSettings={() => setShowSettings(true)}
                        note="Argus lost contact with, or stopped tracking, these sessions. It cannot confirm the OpenCode process ended, so they may still be running."
                      />
                      <SessionSection
                        title="Past"
                        sessions={pastSessions}
                        onRefresh={loadDashboard}
                        onSelect={(s) => setSelectedSession(s)}
                        metaConfigured={metaConfigured}
                        onOpenSettings={() => setShowSettings(true)}
                      />
                    </>
                  )
                )}
              </>
            )}
          </>
        )}

        {/* New Session Modal */}
        {showNewSession && (
          <NewSessionForm onSubmit={handleNewSession} onClose={handleCloseNewSession} serverError={createError} />
        )}

        {/* Settings Modal */}
        {showSettings && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-md mx-4 max-h-[90vh] overflow-y-auto">
              <div className="flex items-center justify-between mb-6">
                <h2 className="text-lg font-medium">Argus Assistant Settings</h2>
                <button aria-label="Close settings" onClick={() => setShowSettings(false)} className="text-textMuted hover:text-text focus:outline-none focus:ring-2 focus:ring-accent rounded px-1">×</button>
              </div>

              <div className="space-y-6">
                {/* Supervision Status */}
                <div>
                  <h3 className="text-sm font-medium mb-3">Assistant Supervision</h3>
                  <p className="text-sm text-textMuted mb-4">
                    Enable Argus assistant to diagnose incidents and propose recovery actions.
                    Requires a configured Meta-compatible API (OpenAI, local LLM, etc.).
                  </p>
                  <p className="text-sm text-textMuted">
                    Supervision is controlled per session with the Supervision toggle on each session card. New sessions start with supervision on, but it only takes effect once a model is configured below.
                  </p>
                </div>

                {/* Model Configuration */}
                <div className="border-t border-border pt-6">
                  <h3 className="text-sm font-medium mb-3">Model Configuration</h3>
                  <p className="text-sm text-textMuted mb-4">
                    Configure the Meta-compatible API for assistant supervision.
                    Supports OpenAI API, local LLMs (Ollama, LM Studio), and other OpenAI-compatible endpoints.
                  </p>

                  {metaConfigured === false && (
                    <div role="status" className="p-3 rounded text-sm bg-amber-500/10 border border-amber-500/30 text-amber-300">
                      Not configured. Enter a base URL, model, and API key if your provider requires one, then Save.
                    </div>
                  )}
                  {metaSaveError && (
                    <div role="alert" className="p-3 rounded text-sm bg-red-500/20 border border-red-500/30 text-red-400">{metaSaveError}</div>
                  )}
                  {metaSaveSuccess && (
                    <div role="status" className="p-3 rounded text-sm bg-green-500/10 border border-green-500/20 text-green-400">{metaSaveSuccess}</div>
                  )}
                  {metaTestError && (
                    <div role="alert" className="p-3 rounded text-sm bg-red-500/20 border border-red-500/30 text-red-400">Test connection: {metaTestError}</div>
                  )}
                  {metaTestSuccess && (
                    <div role="status" className="p-3 rounded text-sm bg-blue-500/10 border border-blue-500/20 text-blue-300">{metaTestSuccess}</div>
                  )}
                  {metaDirty && !metaSaveSuccess && (
                    <p className="text-xs text-amber-300">Unsaved changes. Save to persist them across a service restart.</p>
                  )}

                  <div className="space-y-4">
                    <div>
                      <label htmlFor="meta-base-url" className="block text-sm font-medium mb-1">API Base URL</label>
                      <input
                        id="meta-base-url"
                        type="text"
                        value={metaConfig.baseUrl}
                        onChange={(e) => setMetaConfig({ ...metaConfig, baseUrl: e.target.value })}
                        placeholder="https://api.openai.com/v1 or http://localhost:11434/v1"
                        className="w-full bg-surface border border-border rounded px-3 py-2 text-text"
                      />
                      <p className="text-xs text-textMuted mt-1">
                        The base URL for the OpenAI-compatible API. Include /v1 suffix if required.
                      </p>
                    </div>

                    <div>
                      <label htmlFor="meta-api-key" className="block text-sm font-medium mb-1">API Key</label>
                      <input
                        id="meta-api-key"
                        type="password"
                        value={metaConfig.apiKey}
                        onChange={(e) => setMetaConfig({ ...metaConfig, apiKey: e.target.value })}
                        placeholder={metaHasKey ? 'Saved key in place. Enter a new key to replace it' : 'sk-...'}
                        className="w-full bg-surface border border-border rounded px-3 py-2 text-text"
                      />
                      <p className="text-xs text-textMuted mt-1">
                        {metaHasKey
                          ? 'API key configured (saved on the service, never shown here). Leave blank to keep the saved key, or enter a new key to replace it.'
                          : 'Your API key. Stored only on the Go service, never in the browser.'}
                      </p>
                      {metaHasKey && !metaConfig.apiKey && metaConfigured && (
                        <p className="text-xs text-green-400 mt-1">API key configured</p>
                      )}
                      {metaHasKey && !metaConfig.apiKey && metaConfigured === false && (
                        <p className="text-xs text-amber-300 mt-1">An API key is saved on the service, but the assistant is not fully configured yet. Check the base URL and model, then Save.</p>
                      )}
                    </div>

                    <div>
                      <label htmlFor="meta-model" className="block text-sm font-medium mb-1">Model Name</label>
                      <input
                        id="meta-model"
                        type="text"
                        value={metaConfig.model}
                        onChange={(e) => setMetaConfig({ ...metaConfig, model: e.target.value })}
                        placeholder="gpt-4o-mini, llama3.1, etc."
                        className="w-full bg-surface border border-border rounded px-3 py-2 text-text"
                      />
                      <p className="text-xs text-textMuted mt-1">
                        The model identifier to use for supervision.
                      </p>
                    </div>
                  </div>

                  <div className="flex gap-2 pt-2">
                    <button
                      onClick={async () => {
                        setMetaTesting(true)
                        setMetaTestError(null)
                        setMetaTestSuccess(null)
                        const result = await testMetaConnection(metaConfig.baseUrl, metaConfig.apiKey, metaConfig.model)
                        setMetaTesting(false)
                        if (result.success) {
                          setMetaTestSuccess(`Provider accepted the API key and model ${result.model ?? metaConfig.model} at ${result.baseUrl ?? metaConfig.baseUrl}. This does not save the configuration.`)
                        } else {
                          setMetaTestError(result.error || 'Connection failed')
                        }
                      }}
                      disabled={metaTesting || !metaConfig.baseUrl || !metaConfig.model} aria-busy={metaTesting}
                      className="flex-1 px-4 py-2 bg-blue-500/20 hover:bg-blue-500/30 text-blue-400 border border-blue-500/30 rounded disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {metaTesting ? 'Testing...' : 'Test Connection'}
                    </button>
                    <button
                      onClick={async () => {
                        setMetaSaving(true)
                        setMetaSaveError(null)
                        setMetaSaveSuccess(null)
                        const submitted = { ...metaConfig }
                        const result = await saveMetaConfig(submitted.baseUrl, submitted.apiKey, submitted.model)
                        setMetaSaving(false)
                        if (result.success) {
                          // Backend persisted and reloaded the config. Update
                          // only status/saved snapshot; keep the form fields
                          // as submitted (no flicker), clearing just the key
                          // input value, which is never kept in the browser.
                          const baseUrl = result.baseUrl ?? submitted.baseUrl
                          const model = result.model ?? submitted.model
                          setSavedMeta({ baseUrl, model })
                          setMetaHasKey(result.hasKey ?? metaHasKey)
                          setMetaConfigured(result.configured ?? false)
                          setMetaConfig((prev) => ({ ...prev, apiKey: '' }))
                          void refreshMetaConfig(false)
                          if (result.configured) {
                            setMetaSaveSuccess(`Saved and verified after reload: ${model} at ${baseUrl}. It will persist across a service restart.`)
                          } else {
                            setMetaSaveSuccess(`Saved base URL and model (${model} at ${baseUrl}), but no API key is configured yet, so the assistant is not fully configured.`)
                          }
                        } else {
                          setMetaSaveError(result.error || 'Save failed')
                        }
                      }}
                      disabled={metaSaving || !metaConfig.baseUrl || !metaConfig.model}
                      className="flex-1 px-4 py-2 bg-accent hover:bg-accentHover text-white rounded disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none focus:ring-2 focus:ring-accent"
                    >
                      {metaSaving ? 'Saving...' : 'Save'}
                    </button>
                  </div>
                </div>

                {/* Sync Status */}
                <div className="border-t border-border pt-6">
                  <h3 className="text-sm font-medium mb-3">Supabase Sync</h3>
                  <p className="text-sm text-textMuted mb-2">
                    Session data syncs to Supabase when configured. Uses service role key on the backend only.
                  </p>
                  <div className="p-3 bg-surface/50 border border-border/50 rounded text-sm">
                    <code>SUPABASE_URL</code> and <code>SUPABASE_SERVICE_KEY</code> must be set in the Go service environment.
                    The anon key is not used for backend sync operations.
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}