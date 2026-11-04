/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        background: '#1e1e1e',
        surface: '#252526',
        surfaceHover: '#2d2d2d',
        border: '#333',
        text: '#d4d4d4',
        textMuted: '#888',
        accent: '#0e639c',
        accentHover: '#1177bb',
        success: '#4ec9b0',
        warning: '#dcdcaa',
        error: '#f44747',
        info: '#9cdcfe',
      },
      fontFamily: {
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
        sans: ['system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
      },
    },
  },
  plugins: [],
}