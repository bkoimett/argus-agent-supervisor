import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import App from './App'

const mockFetch = vi.fn()
;(globalThis as any).fetch = mockFetch

const session = {
  id: 'ses_1', project_dir: '/tmp/proj', agent: 'opencode', status: 'RUNNING', pid: 1,
  supervision_enabled: true, tasks: [], incidents: [], approvals: [], recent_events: [], recovery_actions: [],
}

function mockBackend(meta: Record<string, unknown>) {
  mockFetch.mockImplementation((url: string) => {
    const u = String(url)
    const json = (body: unknown, ok = true) => Promise.resolve({ ok, status: ok ? 200 : 500, headers: new Headers(), json: async () => body } as Response)
    if (u.includes('/api/meta/config')) return json(meta)
    if (u.includes('/api/dashboard')) return json([session])
    if (u.includes('/health')) return json({ status: 'ok', started_at: 1 })
    return json({})
  })
}

describe('App GUS visibility and configuration status', () => {
  beforeEach(() => { mockFetch.mockReset(); localStorage.clear() })

  it('keeps GUS visible with an unconfigured state and per-session Show GUS control', async () => {
    mockBackend({ base_url: '', model: '', has_key: false, configured: false })
    render(<App />)
    expect(await screen.findByText(/Not configured\. The assistant cannot diagnose/)).toBeInTheDocument()
    const toggle = await screen.findByRole('button', { name: 'Show GUS' })
    fireEvent.click(toggle)
    expect(await screen.findByText(/GUS is not configured, so the assistant cannot diagnose this session/)).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Configure assistant in Settings' }).length).toBeGreaterThan(0)
    // Supervision toggle exists but is disabled while unconfigured.
    expect(screen.getByLabelText(/GUS supervision for session/)).toBeDisabled()
    // Visibility persists per session.
    expect(localStorage.getItem('argus_gus_visible_ses_1')).toBe('true')
  })

  it('shows configured state consistently on the dashboard', async () => {
    mockBackend({ base_url: 'https://api.example/v1', model: 'm1', has_key: true, configured: true })
    render(<App />)
    expect(await screen.findByText(/Configured with model m1 at https:\/\/api\.example\/v1/)).toBeInTheDocument()
    await waitFor(() => expect(screen.getByLabelText(/GUS supervision for session/)).not.toBeDisabled())
  })
})
