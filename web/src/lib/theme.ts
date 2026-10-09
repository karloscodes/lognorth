// Themes are CSS-variable swaps: each one redefines the tokens in index.css
// under [data-theme]. North is the default and has no attribute. The choice
// lives in this browser only, like an editor's color scheme.

export const THEMES = [
  { id: 'north', label: 'North', note: 'stone and lime, the default' },
  { id: 'light', label: 'Light', note: 'for daylight' },
  { id: 'tokyo-night', label: 'Tokyo Night', note: 'night blue' },
  { id: 'catppuccin', label: 'Catppuccin', note: 'Mocha' },
  { id: 'gruvbox', label: 'Gruvbox', note: 'warm retro' },
  { id: 'nord', label: 'Nord', note: 'arctic gray' },
  { id: 'dracula', label: 'Dracula', note: 'purple dark' },
] as const

export type Theme = (typeof THEMES)[number]['id']

const STORAGE_KEY = 'lognorth-theme'

export function isTheme(v: string | null): v is Theme {
  return THEMES.some((t) => t.id === v)
}

export function getTheme(): Theme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (isTheme(stored)) return stored
  } catch {
    // No storage (private window): North.
  }
  return 'north'
}

export function applyTheme(theme: Theme) {
  if (theme === 'north') document.documentElement.removeAttribute('data-theme')
  else document.documentElement.setAttribute('data-theme', theme)
}

export function setTheme(theme: Theme) {
  applyTheme(theme)
  try {
    localStorage.setItem(STORAGE_KEY, theme)
  } catch {
    // The theme still applies to this page.
  }
}
