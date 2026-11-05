import { useState, useEffect, useCallback, useRef } from 'react'
import { createClient, RealtimeChannel } from '@supabase/supabase-js'

// Types
interface Session {
  id: string
  project_id: string
  status: string
  started_at: string
  ended_at: string | null
  pid: number | null
  agent: string
  project_dir: string
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
}

interface ChatEvent {
  id: string
  session_id: string
  type: string
  timestamp: string
  payload: Record<string, unknown>
}

// Supabase client (configured via environment or manual entry)
const SUPABASE_URL = (import.meta as any).env?.VITE_SUPABASE_URL || ''
const SUPABASE_ANON_KEY = (import.meta as any).env?.VITE_SUPABASE_ANON_KEY || ''

let supabase: ReturnType<typeof createClient> | null = null

export function getSupabase() {
  if (!supabase && SUPABASE_URL && SUPABASE_ANON_KEY) {
    supabase = createClient(SUPABASE_URL, SUPABASE_ANON_KEY)
  }
  return supabase
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
    default: return 'bg-gray-500/20 text-gray-400'
  }
}

function formatTime(dateStr: string | null): string {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  return date.toLocaleTimeString()
}

export async function fetchSessionEvents(sessionId: string, sb?: any): Promise<ChatEvent[]> {
  const supabase = sb ?? getSupabase()
  if (!supabase) return []
  const { data, error } = await (supabase as any)
    .from('events')
    .select('*')
    .eq('session_id', sessionId)
    .order('created_at', { ascending: true })
  if (error || !data) return []
  return data as ChatEvent[]
}

export async function sendCommandToSession(sessionId: string, type: string, payload: Record<string, unknown>, sb?: any): Promise<boolean> {
  const supabase = sb ?? getSupabase()
  if (!supabase) return false
  const { error } = await (supabase as any).from('commands').insert({
    session_id: sessionId,
    type,
    payload,
    status: 'PENDING',
    created_at: new Date().toISOString(),
  })
  return !error
}

// Permission approve/deny is contract-only plumbing and has NOT been verified
// end-to-end against a live OpenCode permission request. The UI controls are
// therefore rendered disabled with an explanation until a real request can be
// triggered and answered. See docs/PROGRESS.md for details.
const PERMISSIONS_UNVERIFIED = 'Permission approve/deny is not yet verified end-to-end. Controls are disabled until a live permission request can be triggered and answered.'

export function SessionList({ sessions, approvals, onRespondApproval, onSessionSelect }: {
  sessions: Session[]
  approvals: Approval[]
  onRespondApproval: (id: string, approved: boolean) => void
  onSessionSelect?: (session: Session) => void
}) {
  if (sessions.length === 0) {
    return (
      <div className="text-center py-12 text-textMuted">
        <p className="text-lg">No active sessions</p>
        <p className="text-sm mt-1">Sessions will appear here when started on desktop</p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {sessions.map((session) => (
        <div
          key={session.id}
          className="bg-surface border border-border rounded-xl p-4 cursor-pointer hover:border-accent/50 transition-border"
          onClick={() => onSessionSelect?.(session)}
        >
          <div className="flex items-start justify-between gap-3 mb-3">
            <div className="flex-1 min-w-0">
              <div className="font-mono text-sm text-textMuted truncate">{session.project_dir}</div>
              <div className="font-medium mt-1">{session.agent}</div>
            </div>
            <span className={`px-3 py-1 rounded-full text-sm font-medium ${statusClass(session.status)} flex-shrink-0`}>
              {session.status}
            </span>
          </div>

          <div className="text-sm text-textMuted">
            Started: {formatTime(session.started_at)}
            {session.pid && <span className="mx-2">•</span>}
            {session.pid && <span>PID: {session.pid}</span>}
          </div>

          {/* Approvals for this session */}
          {approvals.filter(a => a.session_id === session.id).length > 0 && (
            <div className="mt-3 p-3 bg-background rounded-lg border border-border opacity-50 pointer-events-none">
              <div className="text-[10px] text-amber-500 mb-2">{PERMISSIONS_UNVERIFIED}</div>
              {approvals.filter(a => a.session_id === session.id).map((approval) => (
                <div key={approval.id} className="mt-2 p-2 bg-background rounded-lg border border-border">
                  <div className="font-medium mb-1">{approval.title}</div>
                  <div className="text-sm text-textMuted mb-1">{approval.description}</div>
                  <div className="font-mono text-xs text-textMuted mb-2 max-h-20 overflow-auto">{approval.payload}</div>
                  <div className="flex gap-2">
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        onRespondApproval(approval.id, true)
                      }}
                      disabled
                      className="flex-1 py-2 bg-green-500/10 text-green-500/40 border border-green-500/20 rounded font-medium"
                    >
                      Approve
                    </button>
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        onRespondApproval(approval.id, false)
                      }}
                      disabled
                      className="flex-1 py-2 bg-red-500/10 text-red-500/40 border border-red-500/20 rounded font-medium"
                    >
                      Reject
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

function SettingsScreen({ onClose }: { onClose: () => void }) {
  const [supabaseUrl, setSupabaseUrl] = useState(() => localStorage.getItem('argus_supabase_url') || '')
  const [supabaseKey, setSupabaseKey] = useState(() => localStorage.getItem('argus_supabase_key') || '')
  const [deviceName, setDeviceName] = useState(() => localStorage.getItem('argus_device_name') || '')
  const [notifPrefs, setNotifPrefs] = useState<Record<string, boolean>>(() => {
    const stored = localStorage.getItem('argus_notif_prefs')
    if (stored) {
      try {
        return JSON.parse(stored)
      } catch {
        // fall through to defaults
      }
    }
    return {
      needs_input: true,
      permission: true,
      completed: true,
      crashed: true,
      error: true,
      quota_warning: true,
      disconnected: true,
    }
  })

  // Persist notification preferences on change
  useEffect(() => {
    localStorage.setItem('argus_notif_prefs', JSON.stringify(notifPrefs))
  }, [notifPrefs])

  const saveSettings = () => {
    localStorage.setItem('argus_supabase_url', supabaseUrl)
    localStorage.setItem('argus_supabase_key', supabaseKey)
    localStorage.setItem('argus_device_name', deviceName)
    localStorage.setItem('argus_notif_prefs', JSON.stringify(notifPrefs))
    // Reinitialize supabase client
    supabase = null
    onClose()
  }

  return (
    <div className="min-h-screen p-4">
      <div className="max-w-md mx-auto">
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-xl font-medium">Settings</h1>
          <button onClick={onClose} className="p-2 text-textMuted hover:text-text">×</button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Device Name</label>
            <input
              type="text"
              value={deviceName}
              onChange={(e) => setDeviceName(e.target.value)}
              placeholder="My Phone"
              className="w-full"
            />
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Supabase URL</label>
            <input
              type="url"
              value={supabaseUrl}
              onChange={(e) => setSupabaseUrl(e.target.value)}
              placeholder="https://your-project.supabase.co"
              className="w-full"
            />
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Supabase Anon Key</label>
            <input
              type="password"
              value={supabaseKey}
              onChange={(e) => setSupabaseKey(e.target.value)}
              placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
              className="w-full"
            />
          </div>

          <div className="pt-4 border-t border-border">
            <h3 className="text-sm font-medium mb-3">Notification Preferences</h3>
            <div className="space-y-2">
              {['needs_input', 'permission', 'completed', 'crashed', 'error', 'quota_warning', 'disconnected'].map(
                (key) => (
                  <div key={key} className="flex items-center justify-between">
                    <label className="text-sm text-text flex-1 cursor-pointer">
                      {key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
                      <input
                        type="checkbox"
                        checked={notifPrefs[key]}
                        onChange={(e) =>
                          setNotifPrefs((prev: Record<string, boolean>) => ({
                            ...prev,
                            [key]: e.target.checked,
                          }))
                        }
                        className="rounded border-border text-accent focus:ring-accent ml-2"
                      />
                    </label>
                  </div>
                )
              )}
            </div>
          </div>
          <div className="pt-4 border-t border-border">
            <button
              onClick={saveSettings}
              className="w-full py-3 bg-accent hover:bg-accentHover text-white rounded-lg font-medium"
            >
              Save Settings
            </button>
          </div>

          
        </div>
      </div>
    </div>
  )
}

function SessionDetailScreen({
  session,
  onBack,
}: {
  session: Session
  onBack: () => void
}) {
  const [messages, setMessages] = useState<Array<{
    role: 'user' | 'assistant'
    text: string
    time: string
  }>>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const eventsRef = useRef<ChatEvent[]>([])

  const refresh = useCallback(async () => {
    const events = await fetchSessionEvents(session.id)
    eventsRef.current = events

    const msgs: Array<{ role: 'user' | 'assistant'; text: string; time: string }> = []
    for (const ev of events) {
      const payload = ev.payload || {}
      if (ev.type === 'user_message') {
        msgs.push({ role: 'user', text: String(payload.text || ''), time: formatTime(ev.timestamp) })
      } else if (ev.type === 'assistant_message') {
        msgs.push({ role: 'assistant', text: String(payload.text || ''), time: formatTime(ev.timestamp) })
      }
    }
    setMessages(msgs)
  }, [session.id])

  useEffect(() => {
    refresh()
    const iv = setInterval(refresh, 3000)
    return () => clearInterval(iv)
  }, [refresh])

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    const text = input.trim()
    if (!text || sending) return
    setSending(true)
    setSendError(null)
    const ok = await sendCommandToSession(session.id, 'send_message', { message: text })
    setSending(false)
    if (ok) {
      setInput('')
      await refresh()
    } else {
      setSendError('Failed to send message. Check your connection and try again.')
    }
  }

  return (
    <div className="flex flex-col h-full bg-background border border-border rounded-lg overflow-hidden">
      {/* Session header */}
      <div className="flex items-center justify-between p-4 border-b border-border bg-surface">
        <div className="flex items-center gap-3">
          <button onClick={onBack} className="p-2 text-textMuted hover:text-text">
            ← Back
          </button>
          <div className="flex flex-col">
            <div className="font-medium text-text">{session.agent}</div>
            <div className="font-mono text-xs text-textMuted truncate max-w-xs">{session.project_dir}</div>
          </div>
          <span
            className={`px-2 py-1 rounded text-xs font-medium ${
              session.status === 'WORKING' || session.status === 'RUNNING'
                ? 'bg-green-500/20 text-green-400'
                : session.status === 'STOPPED' || session.status === 'DETACHED'
                  ? 'bg-gray-500/20 text-gray-400'
                  : session.status === 'ERROR' || session.status === 'CRASHED'
                    ? 'bg-red-500/20 text-red-400'
                    : 'bg-blue-500/20 text-blue-400'
            }`}
          >
            {session.status}
          </span>
        </div>
      </div>

      {/* Conversation timeline */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {sendError && (
          <div className="p-3 bg-red-500/10 border border-red-500/20 rounded text-sm text-red-400">
            {sendError}
          </div>
        )}
        {messages.length === 0 && !sendError && (
          <div className="text-textMuted text-sm text-center mt-10">
            No messages yet. Start the conversation below.
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} className="flex gap-3 max-w-[85%]">
            {m.role === 'user' ? (
              <div className="flex flex-col gap-1 ml-auto">
                <div className="bg-blue-500/20 text-blue-400 p-3 rounded-lg rounded-tr-none rounded-br-none text-sm break-words">
                  {m.text}
                </div>
                <div className="font-mono text-[10px] text-textMuted text-right">{m.time}</div>
              </div>
            ) : (
              <div className="flex flex-col gap-1">
                <div className="bg-surfaceHover border border-border p-3 rounded-lg rounded-tl-none rounded-bl-none text-sm break-words">
                  {m.text}
                </div>
                <div className="font-mono text-[10px] text-textMuted">{m.time}</div>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Composer */}
      <form onSubmit={handleSend} className="p-4 border-t border-border bg-surface">
        {sendError && (
          <div className="mb-2 p-2 bg-red-500/10 border border-red-500/20 rounded text-sm text-red-400">
            {sendError}
          </div>
        )}
        <div className="flex gap-2">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Type a follow-up message…"
            rows={1}
            className="flex-1 min-h-[44px] bg-background border border-border rounded p-2 text-sm resize-none focus:outline-none focus:border-accent"
            disabled={sending}
          />
          <button
            type="submit"
            disabled={sending || !input.trim()}
            className="px-4 py-2 bg-accent hover:bg-accentHover text-white rounded font-medium disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Send
          </button>
        </div>
        {sending && <div className="mt-2 text-[10px] text-textMuted">Sending…</div>}
      </form>
    </div>
  )
}

export default function App() {
  const [view, setView] = useState<'sessions' | 'session-detail' | 'settings'>('sessions')
  const [sessions, setSessions] = useState<Session[]>([])
  const [approvals, setApprovals] = useState<Approval[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const paired = true
  const [selectedSession, setSelectedSession] = useState<Session | null>(null)
  const channelRef = useRef<RealtimeChannel | null>(null)

  const loadData = useCallback(async () => {
    const sb = getSupabase()
    if (!sb) {
      setError('Supabase not configured. Open Settings to configure.')
      setLoading(false)
      return
    }

    try {
      const [sessionsRes, approvalsRes] = await Promise.all([
        sb.from('sessions').select('*').order('started_at', { ascending: false }),
        sb.from('approvals').select('*').eq('status', 'PENDING').order('requested_at', { ascending: false }),
      ])

      if (sessionsRes.error) throw sessionsRes.error
      if (approvalsRes.error) throw approvalsRes.error

      setSessions(sessionsRes.data || [])
      setApprovals(approvalsRes.data || [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }, [])

  const handleRespondApproval = async (approvalId: string, approved: boolean) => {
    const sb = getSupabase()
    if (!sb) return

    try {
      const updates = {
        status: approved ? 'APPROVED' : 'REJECTED',
        decision: approved,
        decided_by: 'mobile',
        responded_at: new Date().toISOString(),
      }
      const { error } = await (sb
        .from('approvals') as any)
        .update(updates)
        .eq('id', approvalId)

      if (error) throw error
      loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to respond to approval')
    }
  }

  // Setup realtime subscriptions
  useEffect(() => {
    const sb = getSupabase()
    if (!sb || !paired) return

    const channel = sb
      .channel('argus-changes')
      .on('postgres_changes', { event: '*', schema: 'public', table: 'sessions' }, () => loadData())
      .on('postgres_changes', { event: '*', schema: 'public', table: 'approvals' }, () => loadData())
      .subscribe()

    channelRef.current = channel

    loadData()
    const interval = setInterval(loadData, 30000)

    return () => {
      clearInterval(interval)
      if (channelRef.current) {
        sb.removeChannel(channelRef.current)
      }
    }
  }, [loadData])

  

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="bg-surface border-b border-border sticky top-0 z-10">
        <div className="max-w-2xl mx-auto px-4 py-3 flex items-center justify-between">
          <h1 className="font-medium">Argus Remote</h1>
          <div className="flex items-center gap-2">
            <button
              onClick={() => { loadData(); setView('sessions'); }}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium ${view === 'sessions' ? 'bg-accent text-white' : 'bg-surfaceHover text-text'}`}
            >
              Sessions
            </button>
            <button
              onClick={() => setView('settings')}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium ${view === 'settings' ? 'bg-accent text-white' : 'bg-surfaceHover text-text'}`}
            >
              Settings
            </button>
          </div>
        </div>
      </header>

      {/* Content */}
      <main className="max-w-2xl mx-auto p-4 pb-20">
        {error && (
          <div className="mb-4 p-3 bg-red-500/20 border border-red-500/30 rounded text-red-400 text-sm flex items-center justify-between">
            <span>{error}</span>
            <button onClick={() => setError(null)} className="text-red-400 hover:text-red-300">×</button>
          </div>
        )}

        {view === 'session-detail' && selectedSession && (
          <div className="max-w-4xl mx-auto">
            <button
              onClick={() => { setSelectedSession(null); setView('sessions'); }}
              className="mb-4 inline-flex items-center gap-2 text-sm text-textMuted hover:text-text"
            >
              ← Back to sessions
            </button>
            <SessionDetailScreen session={selectedSession} onBack={() => { setSelectedSession(null); setView('sessions'); }} />
          </div>
        )}
        {view === 'sessions' && (
          <>
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-lg font-medium">Active Sessions</h2>
              <button
                onClick={loadData}
                disabled={loading}
                className="px-3 py-1.5 text-sm bg-surfaceHover hover:bg-border border border-border rounded disabled:opacity-50"
              >
                {loading ? 'Refreshing...' : 'Refresh'}
              </button>
            </div>
            <SessionList
              sessions={sessions}
              approvals={approvals}
              onRespondApproval={handleRespondApproval}
              onSessionSelect={setSelectedSession}
            />
          </>
        )}

        {view === 'settings' && (
          <SettingsScreen onClose={() => setView('sessions')} />
        )}
      </main>
    </div>
  )
}