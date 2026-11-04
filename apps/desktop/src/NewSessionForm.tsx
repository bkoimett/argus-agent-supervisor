import { useCallback, useEffect, useRef, useState } from 'react'
import { open } from '@tauri-apps/plugin-dialog'
import { isTauri } from '@tauri-apps/api/core'
import React from 'react'

interface NewSessionFormProps {
  onSubmit: (dir: string, prompt: string) => void
  onClose: () => void
  // Server-reported error (e.g. create-session failure) shown inline in the
  // form so it never flashes behind the modal or leaves the user guessing.
  serverError?: string | null
}

export const NewSessionForm = React.memo(
  function NewSessionForm({ onSubmit, onClose, serverError }: NewSessionFormProps) {
    const [dir, setDir] = useState('')
    const [prompt, setPrompt] = useState('')
    const [error, setError] = useState<string | null>(null)
    const [isDesktop, setIsDesktop] = useState(false)
    const dirRef = useRef('')
    const promptRef = useRef('')

    useEffect(() => {
      setIsDesktop(isTauri())
    }, [])

    const handleBrowse = useCallback(async () => {
      if (!isDesktop) {
        setError('Browse is available in the desktop app. Please enter the path manually.')
        return
      }
      try {
        const selected = await open({ directory: true, multiple: false })
        if (selected && typeof selected === 'string' && selected.length > 0) {
          setDir(selected)
          dirRef.current = selected
          setError(null)
        } else if (Array.isArray(selected) && selected.length > 0 && typeof selected[0] === 'string' && selected[0].length > 0) {
          setDir(selected[0])
          dirRef.current = selected[0]
          setError(null)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to open directory picker')
      }
    }, [isDesktop])

    const handleDirChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value
      setDir(value)
      dirRef.current = value
      setError(null)
    }, [])

    const handlePromptChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const value = e.target.value
      setPrompt(value)
      promptRef.current = value
      setError(null)
    }, [])

    const handleSubmit = useCallback(
      (e: React.FormEvent) => {
        e.preventDefault()
        setError(null)
        const dirValue = dir.trim()
        const promptValue = prompt.trim()
        if (!dirValue) {
          setError('Project directory is required')
          return
        }
        if (!promptValue) {
          setError('Prompt is required')
          return
        }
        onSubmit(dirValue, promptValue)
      },
      [dir, prompt, onSubmit]
    )

    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
        <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-md mx-4">
          <h2 className="text-lg font-medium mb-4">New Session</h2>
          <form onSubmit={handleSubmit} className="space-y-4" onReset={(e) => e.preventDefault()}>
            {(error || serverError) && (
              <div className="p-3 bg-red-500/20 border border-red-500/30 rounded text-red-400 text-sm">
                {error || serverError}
              </div>
            )}
            <div>
              <label className="block text-sm font-medium mb-1">Project Directory</label>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={dir}
                  onChange={handleDirChange}
                  placeholder="/path/to/project"
                  className="flex-1 bg-surface border border-border rounded text-text"
                  required
                />
                <button
                  type="button"
                  onClick={handleBrowse}
                  disabled={!isDesktop}
                  className="px-4 py-2 bg-surfaceHover hover:bg-border border border-border rounded text-text whitespace-nowrap disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Browse…
                </button>
              </div>
              {!isDesktop && (
                <p className="text-xs text-textMuted mt-1">Running in browser — enter path manually</p>
              )}
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Prompt</label>
              <textarea
                value={prompt}
                onChange={handlePromptChange}
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
  },
)
