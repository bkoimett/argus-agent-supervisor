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

async function fetchMetaConfig(): Promise<{ baseUrl: string; model: string; hasKey: boolean } | null> {
  try {
    const res = await fetch(`${API_BASE}/api/meta/config`)
    if (!res.ok) return null
    return res.json()
  } catch {
    return null
  }
}

async function testMetaConnection(baseUrl: string, apiKey: string, model: string): Promise<{ success: boolean; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/meta/test-connection`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ baseUrl, apiKey, model }),
    })
    if (!res.ok) return { success: false, error: 'Request failed' }
    return res.json()
  } catch (err) {
    return { success: false, error: err instanceof Error ? err.message : 'Connection failed' }
  }
}

async function saveMetaConfig(baseUrl: string, apiKey: string, model: string): Promise<{ success: boolean; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/meta/config`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ baseUrl, apiKey, model }),
    })
    if (!res.ok) return { success: false, error: 'Request failed' }
    return res.json()
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

function SessionCard({
  session,
  onRefresh,
  onSelect,
}: {
  session: DashboardData
  onRefresh: () => void
  onSelect?: () => void
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
}: {
  title: string
  sessions: DashboardData[]
  onRefresh: () => void
  onSelect: (session: DashboardData) => void
  note?: string
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
  const [metaConfigError, setMetaConfigError] = useState<string | null>(null)
  const [metaConfigSuccess, setMetaConfigSuccess] = useState<string | null>(null)
  const [metaTesting, setMetaTesting] = useState(false)

  // Fetch meta config on mount
  useEffect(() => {
    fetchMetaConfig().then((config) => {
      if (config) {
        setMetaConfig({ baseUrl: config.baseUrl, apiKey: '', model: config.model })
      }
    })
  }, [])

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
                        note="Argus is actively managing these sessions. Status updates as OpenCode emits events."
                      />
                      <SessionSection
                        title="Disconnected"
                        sessions={disconnectedSessions}
                        onRefresh={loadDashboard}
                        onSelect={(s) => setSelectedSession(s)}
                        note="Argus lost contact with, or stopped tracking, these sessions. It cannot confirm the OpenCode process ended, so they may still be running."
                      />
                      <SessionSection
                        title="Past"
                        sessions={pastSessions}
                        onRefresh={loadDashboard}
                        onSelect={(s) => setSelectedSession(s)}
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
                <button onClick={() => setShowSettings(false)} className="text-textMuted hover:text-text">×</button>
              </div>

              <div className="space-y-6">
                {/* Supervision Status */}
                <div>
                  <h3 className="text-sm font-medium mb-3">Assistant Supervision</h3>
                  <p className="text-sm text-textMuted mb-4">
                    Enable Argus assistant to diagnose incidents and propose recovery actions.
                    Requires a configured Meta-compatible API (OpenAI, local LLM, etc.).
                  </p>
                  <div className="space-y-2">
                    <label className="flex items-center gap-3 cursor-pointer">
                      <input
                        type="checkbox"
                        defaultChecked={true}
                        className="w-4 h-4 rounded border-border bg-surface text-accent focus:ring-accent"
                      />
                      <span className="text-sm">Enable supervision for new sessions (can be toggled per session)</span>
                    </label>
                  </div>
                </div>

                {/* Model Configuration */}
                <div className="border-t border-border pt-6">
                  <h3 className="text-sm font-medium mb-3">Model Configuration</h3>
                  <p className="text-sm text-textMuted mb-4">
                    Configure the Meta-compatible API for assistant supervision.
                    Supports OpenAI API, local LLMs (Ollama, LM Studio), and other OpenAI-compatible endpoints.
                  </p>

                  {(metaConfigError || metaConfigSuccess) && (
                    <div className={`p-3 rounded text-sm ${
                      metaConfigError ? 'bg-red-500/20 border border-red-500/30 text-red-400' :
                      'bg-green-500/10 border border-green-500/20 text-green-400'
                    }`}>
                      {metaConfigError || metaConfigSuccess}
                    </div>
                  )}

                  <div className="space-y-4">
                    <div>
                      <label className="block text-sm font-medium mb-1">API Base URL</label>
                      <input
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
                      <label className="block text-sm font-medium mb-1">API Key</label>
                      <input
                        type="password"
                        value={metaConfig.apiKey}
                        onChange={(e) => setMetaConfig({ ...metaConfig, apiKey: e.target.value })}
                        placeholder="sk-... or leave blank to use environment variable"
                        className="w-full bg-surface border border-border rounded px-3 py-2 text-text"
                      />
                      <p className="text-xs text-textMuted mt-1">
                        Your API key. Not stored in the browser after saving. Falls back to META_API_KEY env var.
                      </p>
                    </div>

                    <div>
                      <label className="block text-sm font-medium mb-1">Model Name</label>
                      <input
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
                        setMetaConfigError(null)
                        setMetaConfigSuccess(null)
                        const result = await testMetaConnection(metaConfig.baseUrl, metaConfig.apiKey, metaConfig.model)
                        setMetaTesting(false)
                        if (result.success) {
                          setMetaConfigSuccess('Connection successful!')
                        } else {
                          setMetaConfigError(result.error || 'Connection failed')
                        }
                      }}
                      disabled={metaTesting || !metaConfig.baseUrl || !metaConfig.model}
                      className="flex-1 px-4 py-2 bg-blue-500/20 hover:bg-blue-500/30 text-blue-400 border border-blue-500/30 rounded disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {metaTesting ? 'Testing...' : 'Test Connection'}
                    </button>
                    <button
                      onClick={async () => {
                        setMetaConfigError(null)
                        setMetaConfigSuccess(null)
                        const result = await saveMetaConfig(metaConfig.baseUrl, metaConfig.apiKey, metaConfig.model)
                        if (result.success) {
                          setMetaConfigSuccess('Configuration saved!')
                        } else {
                          setMetaConfigError(result.error || 'Save failed')
                        }
                      }}
                      disabled={!metaConfig.baseUrl || !metaConfig.model}
                      className="flex-1 px-4 py-2 bg-accent hover:bg-accentHover text-white rounded disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      Save
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