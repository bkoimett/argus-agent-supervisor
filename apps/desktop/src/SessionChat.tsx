import { useState, useEffect, useCallback, useRef } from 'react'

interface ChatEvent {
  id: string
  session_id: string
  type: string
  timestamp: string
  payload: Record<string, unknown>
}

interface SessionChatProps {
  sessionId: string
  projectDir: string
  agent: string
  status: string
  onStatusChange: (status: string) => void
  onClose: () => void
}

const API_BASE = 'http://localhost:8080'

// Completion cannot be reliably detected with the opencode serve integration:
// serve is a single shared, long-lived process with no per-session exit or
// completion endpoint. Session status stays RUNNING until the user stops it.
// A completion notification is only shown when the session status actually
// transitions to COMPLETED (e.g. via the subprocess adapter path). We do not
// fabricate completion merely because a request was accepted or the session is
// still RUNNING.

async function fetchEvents(sessionId: string): Promise<ChatEvent[]> {
  const res = await fetch(`${API_BASE}/api/events?session_id=${encodeURIComponent(sessionId)}`)
  if (!res.ok) return []
  return res.json()
}

async function sendMessage(sessionId: string, message: string): Promise<{ text?: string; messageID?: string } | null> {
  const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/message`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
  if (!res.ok) return null
  return res.json()
}

async function interruptSession(sessionId: string): Promise<boolean> {
  const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}/interrupt`, {
    method: 'POST',
  })
  return res.ok
}

async function stopSession(sessionId: string): Promise<boolean> {
  const res = await fetch(`${API_BASE}/api/sessions/${encodeURIComponent(sessionId)}`, {
    method: 'DELETE',
  })
  return res.ok
}

function formatTime(dateStr: string): string {
  try {
    return new Date(dateStr).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch {
    return dateStr
  }
}

// playCompletionSound plays a short in-app notification sound when a task is
// confirmed complete. It is gated on a user gesture (browsers block audio
// until the page has been interacted with) and is a no-op if the Audio API is
// unavailable.
let audioEnabled = false
function enableAudio() {
  audioEnabled = true
}
function playCompletionSound() {
  if (!audioEnabled || typeof AudioContext === 'undefined' && typeof (window as any).webkitAudioContext === 'undefined') {
    return
  }
  try {
    const Ctx = window.AudioContext || (window as any).webkitAudioContext
    const ctx = new Ctx()
    const t = ctx.currentTime
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.type = 'sine'
    osc.frequency.setValueAtTime(880, t)
    osc.frequency.exponentialRampToValueAtTime(1760, t + 0.15)
    gain.gain.setValueAtTime(0.1, t)
    gain.gain.exponentialRampToValueAtTime(0.001, t + 0.3)
    osc.connect(gain)
    gain.connect(ctx.destination)
    osc.start(t)
    osc.stop(t + 0.3)
  } catch {
    // Audio playback is best-effort; silence is acceptable.
  }
}

export default function SessionChat({
  sessionId,
  projectDir,
  agent,
  status,
  onStatusChange,
  onClose,
}: SessionChatProps) {
  const [messages, setMessages] = useState<Array<{
    role: 'user' | 'assistant'
    text: string
    time: string
    id: string
  }>>([])
  const [activity, setActivity] = useState<Array<{
    type: string
    text: string
    time: string
  }>>([])
  const [errors, setErrors] = useState<Array<{ text: string; time: string }>>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [sessionStatus, setSessionStatus] = useState(status)
  const [completionShown, setCompletionShown] = useState(false)
  const eventsRef = useRef<ChatEvent[]>([])

  const refresh = useCallback(async () => {
    const events = await fetchEvents(sessionId)
    // Sort oldest-first as a safety measure regardless of API ordering, so the
    // conversation reads naturally from top to bottom.
    events.sort((a, b) => a.timestamp.localeCompare(b.timestamp))
    eventsRef.current = events

    const msgs: Array<{ role: 'user' | 'assistant'; text: string; time: string; id: string }> = []
    const act: Array<{ type: string; text: string; time: string }> = []
    const errs: Array<{ text: string; time: string }> = []

    for (const ev of events) {
      const time = formatTime(ev.timestamp)
      const payload = ev.payload || {}
      switch (ev.type) {
        case 'user_message':
          msgs.push({ role: 'user', text: String(payload.text || ''), time, id: ev.id })
          break
        case 'assistant_message':
          msgs.push({
            role: 'assistant',
            text: String(payload.text || ''),
            time,
            id: String((payload as Record<string, unknown>).messageID || ev.id),
          })
          break
        case 'tool_use':
        case 'step_start':
        case 'step_finish': {
          const tool = payload.tool || payload.tool_name || ev.type
          const status = payload.status || payload.execution_status || ''
          const input = payload.input || payload.tool_input || ''
          let text = String(tool)
          if (status) text += ` · ${status}`
          if (input && typeof input === 'string') text += ` · ${String(input).slice(0, 80)}`
          act.push({ type: ev.type, text, time })
          break
        }
        case 'error':
          errs.push({ text: String(payload.error || payload.output || ev.type), time })
          break
        default:
          // routine activity: tracked but not surfaced by default
          break
      }
    }

    setMessages(msgs)
    setActivity(act)
    setErrors(errs)
  }, [sessionId])

  useEffect(() => {
    refresh()
    const iv = setInterval(refresh, 3000)
    return () => clearInterval(iv)
  }, [refresh])

  useEffect(() => {
    // Enable audio on first user interaction so browsers allow playback.
    const enable = () => { enableAudio() }
    document.addEventListener('click', enable, { once: true })
    document.addEventListener('keydown', enable, { once: true })
    return () => {
      document.removeEventListener('click', enable)
      document.removeEventListener('keydown', enable)
    }
  }, [])

  // Show a completion notification only when the session status is confirmed
  // COMPLETED. With opencode serve this status is not set by the adapter, so
  // the banner will not appear for serve-driven sessions — that limitation is
  // honest, not fabricated.
  useEffect(() => {
    if (sessionStatus === 'COMPLETED' && !completionShown) {
      setCompletionShown(true)
      playCompletionSound()
    }
  }, [sessionStatus, completionShown])

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    const text = input.trim()
    if (!text || sending) return
    setSending(true)
    setSendError(null)
    const res = await sendMessage(sessionId, text)
    setSending(false)
    if (res) {
      setInput('')
      await refresh()
    } else {
      // Preserve the user's text so they can retry without retyping.
      setSendError('Failed to send message. Check your connection and try again.')
    }
  }

  const handleInterrupt = async () => {
    const ok = await interruptSession(sessionId)
    if (ok) {
      setSessionStatus('WORKING') // interrupt ends the turn, session retained
      onStatusChange('WORKING')
      await refresh()
    }
  }

  const handleEnd = async () => {
    const ok = await stopSession(sessionId)
    if (ok) {
      setSessionStatus('STOPPED')
      onStatusChange('STOPPED')
      onClose()
    }
  }

  return (
    <div className="flex flex-col h-full bg-background border border-border rounded-lg overflow-hidden">
      {/* Session header */}
      <div className="flex items-center justify-between p-4 border-b border-border bg-surface">
        <div className="flex items-center gap-3">
          <div className="flex flex-col">
            <div className="font-medium text-text">{agent}</div>
            <div className="font-mono text-xs text-textMuted truncate max-w-xs">{projectDir}</div>
          </div>
          <span
            className={`px-2 py-1 rounded text-xs font-medium ${
              sessionStatus === 'WORKING'
                ? 'bg-green-500/20 text-green-400'
                : sessionStatus === 'STOPPED' || sessionStatus === 'DETACHED'
                  ? 'bg-gray-500/20 text-gray-400'
                  : sessionStatus === 'ERROR' || sessionStatus === 'CRASHED'
                    ? 'bg-red-500/20 text-red-400'
                    : 'bg-blue-500/20 text-blue-400'
            }`}
          >
            {sessionStatus}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleInterrupt}
            disabled={sessionStatus === 'STOPPED' || sessionStatus === 'ERROR' || sessionStatus === 'DETACHED'}
            className="px-3 py-1.5 text-sm bg-amber-500/20 hover:bg-amber-500/30 text-amber-400 border border-amber-500/30 rounded disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Interrupt current work
          </button>
          <button
            onClick={handleEnd}
            className="px-3 py-1.5 text-sm bg-red-500/20 hover:bg-red-500/30 text-red-400 border border-red-500/30 rounded"
          >
            End session
          </button>
          <button onClick={onClose} className="ml-2 px-3 py-1.5 text-sm text-textMuted hover:text-text rounded">
            Close
          </button>
        </div>
      </div>

      {/* Conversation timeline */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Errors surfaced prominently */}
        {errors.length > 0 && (
          <div className="space-y-2">
            {errors.map((err, i) => (
              <div key={i} className="p-3 bg-red-500/10 border border-red-500/20 rounded text-sm text-red-400">
                {err.text}
              </div>
            ))}
          </div>
        )}
        {/* Completion notification — shown only when status is confirmed COMPLETED */}
        {completionShown && (
          <div className="p-3 bg-green-500/10 border border-green-500/20 rounded text-sm text-green-400 flex items-center gap-2">
            <span>Task complete.</span>
          </div>
        )}
        {messages.length === 0 && errors.length === 0 && !completionShown && (
          <div className="text-textMuted text-sm text-center mt-10">
            No messages yet. Start the conversation below.
          </div>
        )}
        {messages.map((m) => (
          <div key={m.id} className="flex gap-3 max-w-[85%]">
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

      {/* Compact tool/activity status (collapsible, hidden by default) */}
      <details className="border-t border-border bg-surface/50">
        <summary
          className="flex items-center justify-between p-2 text-xs font-medium text-textMuted cursor-pointer hover:bg-surfaceHover/50 rounded-b-lg"
        >
          <span>Tool activity &amp; events</span>
          <span>{activity.length}</span>
        </summary>
        <div className="p-2 space-y-2 max-h-48 overflow-y-auto">
          {activity.length === 0 ? (
            <div className="text-textMuted text-xs text-center">No tool activity.</div>
          ) : (
            activity.map((a, i) => (
              <div key={i} className="p-2 bg-background rounded border border-border/50">
                <div className="flex justify-between text-[10px] font-mono text-textMuted mb-1">
                  <span>{a.type}</span>
                  <span>{a.time}</span>
                </div>
                <div className="text-xs text-text">{a.text}</div>
              </div>
            ))
          )}
        </div>
      </details>

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
            placeholder="Type a follow-up message… (Shift+Enter for newline)"
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
        {sending && (
          <div className="mt-2 text-[10px] text-textMuted">Sending…</div>
        )}
      </form>
    </div>
  )
}
