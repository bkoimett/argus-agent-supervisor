import { createContext, useContext, useEffect, useState, ReactNode } from 'react'

type Theme = 'light' | 'dark' | 'system'

interface ThemeContextType {
  theme: Theme
  resolvedTheme: 'light' | 'dark'
  setTheme: (theme: Theme) => void
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined)

function getMediaQuery() {
  if (typeof window !== 'undefined' && window.matchMedia) {
    return window.matchMedia('(prefers-color-scheme: dark)')
  }
  return {
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
    media: '',
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  } as MediaQueryList
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>('system')
  const [resolvedTheme, setResolvedTheme] = useState<'light' | 'dark'>('dark')
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
    const stored = localStorage.getItem('argus_theme') as Theme | null
    if (stored) {
      setThemeState(stored)
    }
  }, [])

  useEffect(() => {
    if (!mounted) return

    const root = document.documentElement
    const mediaQuery = getMediaQuery()

    const updateResolvedTheme = () => {
      let resolved: 'light' | 'dark'
      if (theme === 'system') {
        resolved = mediaQuery.matches ? 'dark' : 'light'
      } else {
        resolved = theme
      }
      setResolvedTheme(resolved)
      root.setAttribute('data-theme', resolved)
    }

    updateResolvedTheme()
    mediaQuery.addEventListener('change', updateResolvedTheme)
    return () => mediaQuery.removeEventListener('change', updateResolvedTheme)
  }, [theme, mounted])

  const setTheme = (newTheme: Theme) => {
    setThemeState(newTheme)
    localStorage.setItem('argus_theme', newTheme)
  }

  const value = { theme, resolvedTheme, setTheme }

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  )
}

export function useTheme() {
  const context = useContext(ThemeContext)
  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider')
  }
  return context
}