import { useTheme } from '../contexts/ThemeContext'

export function useDarkMode() {
  const { isDark, toggleTheme } = useTheme()
  return { isDark, toggle: toggleTheme }
}
