import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import App, {
  fetchSessionEvents,
  sendCommandToSession,
  SessionList,
} from './App'
import { render, screen, fireEvent } from '@testing-library/react'

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

function mockSupabase(chain: any) {
  return {
    from: vi.fn(() => chain),
  }
}

describe('fetchSessionEvents', () => {
  it('returns events from Supabase', async () => {
    const events = [
      {
        id: 'evt_1',
        session_id: 'ses_1',
        type: 'user_message',
        timestamp: '2026-11-02T10:00:00Z',
        payload: { text: 'hello' },
      },
    ]

    const chain = {
      select: vi.fn(() => ({
        eq: vi.fn(() => ({
          order: vi.fn(() => ({
            then: (resolve: (val: any) => void) =>
              resolve({ data: events, error: null }),
          })),
        })),
      })),
    }
    const sb = mockSupabase(chain)

    const result = await fetchSessionEvents('ses_1', sb)
    expect(result).toHaveLength(1)
    expect(result[0].type).toBe('user_message')
  })

  it('returns empty array on error', async () => {
    const chain = {
      select: vi.fn(() => ({
        eq: vi.fn(() => ({
          order: vi.fn(() => ({
            then: (resolve: (val: any) => void) =>
              resolve({ data: null, error: { message: 'fail' } }),
          })),
        })),
      })),
    }
    const sb = mockSupabase(chain)

    const result = await fetchSessionEvents('ses_1', sb)
    expect(result).toHaveLength(0)
  })

  it('returns empty array when no Supabase client', async () => {
    const result = await fetchSessionEvents('ses_1', null)
    expect(result).toHaveLength(0)
  })
})

describe('sendCommandToSession', () => {
  it('creates a command and returns true on success', async () => {
    const chain = {
      insert: vi.fn(() => ({
        then: (resolve: (val: any) => void) =>
          resolve({ data: [], error: null }),
      })),
    }
    const sb = mockSupabase(chain)

    const ok = await sendCommandToSession('ses_1', 'send_message', {
      message: 'test',
    }, sb)
    expect(ok).toBe(true)
    expect(sb.from).toHaveBeenCalledWith('commands')
  })

  it('returns false on failure', async () => {
    const chain = {
      insert: vi.fn(() => ({
        then: (resolve: (val: any) => void) =>
          resolve({ data: null, error: { message: 'fail' } }),
      })),
    }
    const sb = mockSupabase(chain)

    const ok = await sendCommandToSession('ses_1', 'send_message', {
      message: 'test',
    }, sb)
    expect(ok).toBe(false)
  })

  it('returns false when no Supabase client', async () => {
    const ok = await sendCommandToSession('ses_1', 'send_message', {
      message: 'test',
    }, null)
    expect(ok).toBe(false)
  })
})

describe('SettingsScreen notification preferences', () => {
  const keyPrefs = ['needs_input', 'permission', 'completed', 'crashed', 'error', 'quota_warning', 'disconnected']

  beforeEach(() => {
    localStorage.setItem('argus_paired', 'true')
    localStorage.removeItem('argus_notif_prefs')
  })

  afterEach(() => {
    localStorage.removeItem('argus_paired')
    localStorage.removeItem('argus_notif_prefs')
  })

  it('toggles notification preferences and persists to localStorage', async () => {
    const { unmount } = render(<App />)

    // Click Settings tab
    const settingsBtn = screen.getByRole('button', { name: /settings/i })
    fireEvent.click(settingsBtn)

    // Toggle 'Completed' off
    const completedCheckbox = screen.getByRole('checkbox', { name: /completed/i })
    fireEvent.click(completedCheckbox)
    expect(completedCheckbox).not.toBeChecked()

    // Toggle 'Error' off
    const errorCheckbox = screen.getByRole('checkbox', { name: /error/i })
    fireEvent.click(errorCheckbox)
    expect(errorCheckbox).not.toBeChecked()

    // Verify localStorage
    const stored = JSON.parse(localStorage.getItem('argus_notif_prefs') || '{}')
    expect(stored.completed).toBe(false)
    expect(stored.error).toBe(false)
    expect(stored.needs_input).toBe(true) // untouched

    unmount()
  })

  it('renders all 7 notification preferences', async () => {
    const { unmount } = render(<App />)

    const settingsBtn = screen.getByRole('button', { name: /settings/i })
    fireEvent.click(settingsBtn)

    for (const key of keyPrefs) {
      const labelText = key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
      expect(screen.getByRole('checkbox', { name: labelText })).toBeInTheDocument()
    }

    unmount()
  })
})

describe('SessionList permission controls', () => {
  const session: any = {
    id: 'ses_1',
    project_id: 'proj_1',
    status: 'RUNNING',
    started_at: '2026-11-02T10:00:00Z',
    ended_at: null,
    pid: 1234,
    agent: 'opencode',
    project_dir: '/tmp/demo',
  }

  const approval: any = {
    id: 'appr_1',
    session_id: 'ses_1',
    task_id: null,
    incident_id: null,
    type: 'PERMISSION',
    status: 'PENDING',
    title: 'Permission request',
    description: 'Agent requests file access',
    payload: '{}',
    scope: 'ses_1',
    requested_at: '2026-11-02T10:00:00Z',
    expires_at: '2026-11-02T10:10:00Z',
  }

  it('renders Approve/Reject buttons as disabled', () => {
    const onRespondApproval = vi.fn()
    render(
      <SessionList
        sessions={[session]}
        approvals={[approval]}
        onRespondApproval={onRespondApproval}
      />
    )

    const approveBtn = screen.getByRole('button', { name: 'Approve' })
    const rejectBtn = screen.getByRole('button', { name: 'Reject' })
    expect(approveBtn).toBeDisabled()
    expect(rejectBtn).toBeDisabled()
  })

  it('renders the not-available-yet explanation', () => {
    const onRespondApproval = vi.fn()
    render(
      <SessionList
        sessions={[session]}
        approvals={[approval]}
        onRespondApproval={onRespondApproval}
      />
    )

    expect(
      screen.getByText(/Permission approve\/deny is not yet verified end-to-end/)
    ).toBeInTheDocument()
  })

  it('does not call onRespondApproval when disabled buttons are clicked', () => {
    const onRespondApproval = vi.fn()
    render(
      <SessionList
        sessions={[session]}
        approvals={[approval]}
        onRespondApproval={onRespondApproval}
      />
    )

    const approveBtn = screen.getByRole('button', { name: 'Approve' })
    approveBtn.click()
    expect(onRespondApproval).not.toHaveBeenCalled()
  })

  it('renders nothing when there are no approvals', () => {
    const onRespondApproval = vi.fn()
    render(
      <SessionList
        sessions={[session]}
        approvals={[]}
        onRespondApproval={onRespondApproval}
      />
    )

    expect(screen.queryByRole('button', { name: 'Approve' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Reject' })).not.toBeInTheDocument()
  })
})
