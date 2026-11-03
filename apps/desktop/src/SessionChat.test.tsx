import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import SessionChat from './SessionChat'

// Mock fetch globally
const mockFetch = vi.fn()
;(globalThis as any).fetch = mockFetch

const defaultProps = {
  sessionId: 'ses_test_123',
  projectDir: '/tmp/test-project',
  agent: 'opencode',
  status: 'WORKING',
  onStatusChange: vi.fn(),
  onClose: vi.fn(),
}

function mockEventsResponse(events: any[]) {
  mockFetch.mockImplementation((url: string) => {
    if (String(url).includes('/api/events')) {
      return Promise.resolve({
        ok: true,
        json: async () => events,
      } as Response)
    }
    if (String(url).includes('/api/sessions/') && String(url).includes('/message')) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          id: 'msg_1',
          sessionID: 'ses_test_123',
          role: 'assistant',
          text: 'Assistant response',
          finish: 'stop',
          parts: [],
          tokens: { total: 10, input: 6, output: 3, reasoning: 1 },
        }),
      } as Response)
    }
    return Promise.resolve({
      ok: false,
      status: 500,
    } as Response)
  })
}

function mockSendError() {
  mockFetch.mockImplementation(() => {
    return Promise.resolve({
      ok: false,
      status: 500,
    } as Response)
  })
}


beforeEach(() => {
  vi.clearAllMocks()
  mockEventsResponse([])
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('SessionChat', () => {
  it('renders the session header with agent and project dir', () => {
    render(<SessionChat {...defaultProps} />)
    expect(screen.getByText('opencode')).toBeInTheDocument()
    expect(screen.getByText('/tmp/test-project')).toBeInTheDocument()
    expect(screen.getByText('WORKING')).toBeInTheDocument()
  })

  it('has Interrupt current work and End session buttons', () => {
    render(<SessionChat {...defaultProps} />)
    expect(screen.getByRole('button', { name: 'Interrupt current work' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'End session' })).toBeInTheDocument()
  })

  it('sends a follow-up message and displays the assistant response', async () => {
    const events = [
      {
        id: 'evt_1',
        session_id: 'ses_test_123',
        type: 'user_message',
        timestamp: Date.now().toString(),
        payload: { text: 'hello world' },
      },
      {
        id: 'evt_2',
        session_id: 'ses_test_123',
        type: 'assistant_message',
        timestamp: Date.now().toString(),
        payload: {
          text: 'Assistant response',
          messageID: 'msg_1',
          finish: 'stop',
        },
      },
    ]
    mockEventsResponse(events)

    render(<SessionChat {...defaultProps} />)

    const input = screen.getByPlaceholderText(/type a follow-up message/i)
    fireEvent.change(input, { target: { value: 'hello world' } })
    fireEvent.submit(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => {
      expect(screen.getByText('hello world')).toBeInTheDocument()
    })

    await waitFor(() => {
      expect(screen.getByText('Assistant response')).toBeInTheDocument()
    })
  })

  it('prevents duplicate sends while a request is in flight', async () => {
    let resolveSend: (val: any) => void = () => {}
    mockFetch.mockImplementation((url: string) => {
      if (String(url).includes('/api/events')) {
        return Promise.resolve({
          ok: true,
          json: async () => [],
        } as Response)
      }
      return new Promise((resolve) => {
        resolveSend = (val) => resolve(val)
      })
    })

    render(<SessionChat {...defaultProps} />)

    const input = screen.getByPlaceholderText(/type a follow-up message/i)
    const sendButton = screen.getByRole('button', { name: 'Send' })

    fireEvent.change(input, { target: { value: 'first message' } })
    fireEvent.submit(sendButton)

    // Button should be disabled while in flight
    expect(sendButton).toBeDisabled()

    // Resolve the send request
    resolveSend({
      ok: true,
      json: async () => ({ id: 'msg_1', text: 'ok', sessionID: 'ses_test_123' }),
    } as Response)

    await waitFor(() => {
      expect(input).toHaveValue('')
    })
  })

  it('shows error and preserves input when send fails', async () => {
    mockSendError()

    render(<SessionChat {...defaultProps} />)

    const input = screen.getByPlaceholderText(/type a follow-up message/i)
    const sendButton = screen.getByRole('button', { name: 'Send' })

    fireEvent.change(input, { target: { value: 'failing message' } })
    fireEvent.submit(sendButton)

    await waitFor(() => {
      expect(screen.getByText(/failed to send message/i)).toBeInTheDocument()
    })

    // Input should be preserved for retry
    expect(input).toHaveValue('failing message')

    // Button should be re-enabled after failure
    expect(sendButton).not.toBeDisabled()
  })

  it('retries successfully after a failure', async () => {
    let callCount = 0
    mockFetch.mockImplementation((url: string) => {
      if (String(url).includes('/api/events')) {
        return Promise.resolve({
          ok: true,
          json: async () => [],
        } as Response)
      }
      callCount++
      if (callCount === 1) {
        // First send fails
        return Promise.resolve({
          ok: false,
          status: 500,
        } as Response)
      }
      // Second send succeeds
      return Promise.resolve({
        ok: true,
        json: async () => ({ id: 'msg_2', text: 'ok', sessionID: 'ses_test_123' }),
      } as Response)
    })

    render(<SessionChat {...defaultProps} />)

    const input = screen.getByPlaceholderText(/type a follow-up message/i)
    const sendButton = screen.getByRole('button', { name: 'Send' })

    fireEvent.change(input, { target: { value: 'retry message' } })
    fireEvent.submit(sendButton)

    await waitFor(() => {
      expect(screen.getByText(/failed to send message/i)).toBeInTheDocument()
    })

    // Retry
    fireEvent.submit(sendButton)

    await waitFor(() => {
      expect(screen.queryByText(/failed to send message/i)).not.toBeInTheDocument()
    })

    expect(input).toHaveValue('')
  })

  it('disables Send button when input is empty', () => {
    render(<SessionChat {...defaultProps} />)

    const sendButton = screen.getByRole('button', { name: 'Send' })
    expect(sendButton).toBeDisabled()
  })

  it('shows activity block when tool events exist', async () => {
    const events = [
      {
        id: 'evt_1',
        session_id: 'ses_test_123',
        type: 'tool_use',
        timestamp: Date.now().toString(),
        payload: { tool: 'read', status: 'completed', input: { filePath: '/tmp/test.go' } },
      },
    ]
    mockEventsResponse(events)

    render(<SessionChat {...defaultProps} />)

    await waitFor(() => {
      expect(screen.getByText('Tool activity & events')).toBeInTheDocument()
    })
    expect(screen.getByText('1')).toBeInTheDocument() // activity count
  })
})
