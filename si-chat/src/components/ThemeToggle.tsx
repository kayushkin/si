import { useDarkMode } from '../hooks/useDarkMode'

export default function ThemeToggle() {
  const { isDark, toggle } = useDarkMode()

  return (
    <button
      onClick={toggle}
      className="theme-toggle"
      aria-label={`Switch to ${isDark ? 'light' : 'dark'} mode`}
      title={`Switch to ${isDark ? 'light' : 'dark'} mode`}
    >
      {isDark ? '☀️' : '🌙'}
    </button>
  )
}
