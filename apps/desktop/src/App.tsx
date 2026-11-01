import { useState, useEffect, useCallback } from 'react'

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

async function createSession(projectDir: string, prompt: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ project_dir: projectDir, prompt }),
  })
  if (!res.ok) throw new Error('Failed to create session')
}

async function stopSession(sessionId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/sessions/${sessionId}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error('Failed to stop session')
}

async function respondApproval(approvalId: string, approved: boolean): Promise<void> {
  const res = await fetch(`${API_BASE}/api/approvals/${approvalId}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ approved, decided_by: 'desktop' }),
  })
  if (!res.ok) throw new Error('Failed to respond to approval')
}

async function diagnoseIncident(incidentId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/diagnosis`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ incident_id: incidentId }),
  })
  if (!res.ok) throw new Error('Failed to diagnose incident')
}

async function requestRecoveryApproval(recoveryId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/recovery/approval`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ recovery_id: recoveryId }),
  })
  if (!res.ok) throw new Error('Failed to request recovery approval')
}

async function executeRecovery(recoveryId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/recovery/${recoveryId}`, {
    method: 'POST',
  })
  if (!res.ok) throw new Error('Failed to execute recovery')
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

function formatTime(dateStr: string | null): string {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  return date.toLocaleTimeString()
}

function SessionCard({ session, onRefresh }: { session: DashboardData; onRefresh: () => void }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="bg-surface border border-border rounded-lg overflow-hidden">
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
          <button
            onClick={() => setExpanded(!expanded)}
            className="px-3 py-1 text-sm bg-surfaceHover hover:bg-border border border-border rounded"
          >
            {expanded ? 'Collapse' : 'Expand'}
          </button>
          <button
            onClick={() => stopSession(session.id).then(onRefresh)}
            disabled={session.status === 'COMPLETED' || session.status === 'STOPPED' || session.status === 'ERROR'}
            className="px-3 py-1 text-sm bg-red-500/20 hover:bg-red-500/30 text-red-400 border border-red-500/30 rounded disabled:opacity-50"
          >
            Stop
          </button>
        </div>
      </div>

      {/* Expanded Content */}
      {expanded && (
        <div className="p-4 space-y-4 border-t border-border">
          {/* Tasks */}
          {session.tasks.length > 0 && (
            <div>
              <h4 className="font-medium mb-2">Tasks</h4>
              <div className="space-y-2">
                {session.tasks.map((task) => (
                  <div key={task.id} className="p-3 bg-background rounded border border-border">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <div className="font-mono text-sm text-textMuted">{task.prompt}</div>
                        <div className="flex items-center gap-2 mt-1">
                          <span className={`px-2 py-0.5 rounded text-xs ${statusClass(task.status)}`}>
                            {task.status}
                          </span>
                          <span className="font-mono text-xs text-textMuted">Scope: {task.scope}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Incidents */}
          {session.incidents.length > 0 && (
            <div>
              <h4 className="font-medium mb-2">Incidents</h4>
              <div className="space-y-2">
                {session.incidents.map((inc) => (
                  <div key={inc.id} className="p-3 bg-background rounded border border-border">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{inc.type}</span>
                          <span className={`px-2 py-0.5 rounded text-xs ${statusClass(inc.status)}`}>
                            {inc.status}
                          </span>
                          <span className={`px-2 py-0.5 rounded text-xs bg-gray-500/20 text-gray-400`}>
                            {inc.severity}
                          </span>
                        </div>
                        <div className="font-mono text-xs text-textMuted mt-1 max-h-32 overflow-auto">
                          {inc.evidence}
                        </div>
                        {inc.meta_diagnosis && (
                          <div className="mt-2 p-2 bg-blue-500/10 rounded border border-blue-500/20 text-sm">
                            <strong>Diagnosis:</strong> {inc.meta_diagnosis}
                          </div>
                        )}
                      </div>
                      <div className="flex flex-col gap-1">
                        {inc.status === 'PROPOSED' && (
                          <>
                            <button
                              onClick={() => diagnoseIncident(inc.id).then(onRefresh)}
                              className="px-3 py-1 text-sm bg-blue-500/20 hover:bg-blue-500/30 text-blue-400 border border-blue-500/30 rounded"
                            >
                              Diagnose
                            </button>
                          </>
                        )}
                        {inc.status === 'APPROVED' && (
                          <button
                            onClick={() => executeRecovery(inc.id).then(onRefresh)}
                            className="px-3 py-1 text-sm bg-green-500/20 hover:bg-green-500/30 text-green-400 border border-green-500/30 rounded"
                          >
                            Execute Recovery
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Approvals */}
          {session.approvals.length > 0 && (
            <div>
              <h4 className="font-medium mb-2">Pending Approvals</h4>
              <div className="space-y-2">
                {session.approvals.map((appr) => (
                  <div key={appr.id} className="p-3 bg-background rounded border border-border">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <div className="font-medium">{appr.title}</div>
                        <div className="text-sm text-textMuted mt-1">{appr.description}</div>
                        <div className="font-mono text-xs text-textMuted mt-1 max-h-24 overflow-auto">
                          {appr.payload}
                        </div>
                      </div>
                      <div className="flex gap-2">
                        <button
                          onClick={() => respondApproval(appr.id, true).then(onRefresh)}
                          className="px-3 py-1 text-sm bg-green-500/20 hover:bg-green-500/30 text-green-400 border border-green-500/30 rounded"
                        >
                          Approve
                        </button>
                        <button
                          onClick={() => respondApproval(appr.id, false).then(onRefresh)}
                          className="px-3 py-1 text-sm bg-red-500/20 hover:bg-red-500/30 text-red-400 border border-red-500/30 rounded"
                        >
                          Reject
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Recovery Actions */}
          {session.recovery_actions && session.recovery_actions.length > 0 && (
            <div>
              <h4 className="font-medium mb-2">Recovery Actions</h4>
              <div className="space-y-2">
                {session.recovery_actions.map((ra: any) => (
                  <div key={ra.id} className="p-3 bg-background rounded border border-border">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{ra.type}</span>
                          <span className={`px-2 py-0.5 rounded text-xs ${statusClass(ra.status)}`}>
                            {ra.status}
                          </span>
                        </div>
                        <div className="text-sm text-textMuted mt-1">{ra.description}</div>
                        <div className="font-mono text-xs text-textMuted mt-1">
                          <code>{ra.command} {ra.args?.join(' ') || ''}</code>
                        </div>
                        {ra.result && (
                          <div className="mt-2 p-2 bg-background rounded border border-border text-sm">
                            <div>Exit: {ra.result.exit_code} | {ra.result.status}</div>
                            <div className="font-mono text-xs max-h-24 overflow-auto mt-1">{ra.result.stdout}</div>
                          </div>
                        )}
                      </div>
                      <div className="flex flex-col gap-1">
                        {ra.status === 'PROPOSED' && (
                          <button
                            onClick={() => requestRecoveryApproval(ra.id).then(onRefresh)}
                            className="px-3 py-1 text-sm bg-blue-500/20 hover:bg-blue-500/30 text-blue-400 border border-blue-500/30 rounded"
                          >
                            Request Approval
                          </button>
                        )}
                        {ra.status === 'APPROVED' && (
                          <button
                            onClick={() => executeRecovery(ra.id).then(onRefresh)}
                            className="px-3 py-1 text-sm bg-green-500/20 hover:bg-green-500/30 text-green-400 border border-green-500/30 rounded"
                          >
                            Execute
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Recent Events */}
          {session.recent_events.length > 0 && (
            <div>
              <h4 className="font-medium mb-2">Recent Events</h4>
              <div className="font-mono text-xs max-h-64 overflow-auto bg-background rounded border border-border p-2">
                {session.recent_events.slice().reverse().map((evt, idx) => (
                  <div key={idx} className="py-1 border-b border-border/50 last:border-0">
                    <span className="text-textMuted">{formatTime(evt.timestamp)} </span>
                    <span className="text-info">{evt.type}</span>
                    <pre className="inline text-textMuted ml-2">{JSON.stringify(evt.payload).slice(0, 200)}</pre>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function NewSessionForm({ onSubmit, onClose }: { onSubmit: (dir: string, prompt: string) => void; onClose: () => void }) {
  const [dir, setDir] = useState('')
  const [prompt, setPrompt] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (dir && prompt) {
      onSubmit(dir, prompt)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-md mx-4">
        <h2 className="text-lg font-medium mb-4">New Session</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Project Directory</label>
            <input
              type="text"
              value={dir}
              onChange={(e) => setDir(e.target.value)}
              placeholder="/path/to/project"
              className="w-full bg-surface border border-border rounded text-text"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Prompt</label>
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="Describe the task..."
              rows={3}
              className="w-full bg-surface border border-border rounded text-text"
              required
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-4 py-2 bg-surfaceHover hover:bg-border border border-border rounded">
              Cancel
            </button>
            <button type="submit" className="px-4 py-2 bg-accent hover:bg-accentHover text-white rounded">
              Create Session
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default function App() {
  const [sessions, setSessions] = useState<DashboardData[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showNewSession, setShowNewSession] = useState(false)

  const loadDashboard = useCallback(async () => {
    try {
      setLoading(true)
      const data = await fetchDashboard()
      setSessions(data)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadDashboard()
    const interval = setInterval(loadDashboard, 5000)
    return () => clearInterval(interval)
  }, [loadDashboard])

  const handleNewSession = async (dir: string, prompt: string) => {
    try {
      await createSession(dir, prompt)
      setShowNewSession(false)
      loadDashboard()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create session')
    }
  }

  if (loading && sessions.length === 0) {
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
          <h1 className="text-2xl font-medium">Argus Dashboard</h1>
          <button
            onClick={() => setShowNewSession(true)}
            className="px-4 py-2 bg-accent hover:bg-accentHover text-white rounded font-medium"
          >
            New Session
          </button>
        </div>

        {/* Error Banner */}
        {error && (
          <div className="mb-4 p-4 bg-red-500/20 border border-red-500/30 rounded text-red-400 flex items-center justify-between">
            <span>{error}</span>
            <button onClick={() => setError(null)} className="text-red-400 hover:text-red-300">×</button>
          </div>
        )}

        {/* Sessions Grid */}
        {(sessions ?? []).length === 0 ? (
          <div className="text-center py-12 text-textMuted">
            <p className="text-lg">No sessions running</p>
            <p className="text-sm mt-1">Click "New Session" to start an OpenCode agent</p>
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {sessions.map((session) => (
              <SessionCard key={session.id} session={session} onRefresh={loadDashboard} />
            ))}
          </div>
        )}

        {/* New Session Modal */}
        {showNewSession && (
          <NewSessionForm onSubmit={handleNewSession} onClose={() => setShowNewSession(false)} />
        )}
      </div>
    </div>
  )
}