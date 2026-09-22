// Keyboard model, herdr style. ctrl+b opens navigate mode (the layout owns it).
// Outside it, keys go to what is on screen: pages add their own j/k, m, e, /.

// True while navigate mode or the app picker owns the keyboard.
export function navigating(): boolean {
  const c = document.documentElement.classList
  return c.contains('prefix') || c.contains('picking')
}

// True when the key was typed into a form field or with a modifier held.
export function typing(e: KeyboardEvent): boolean {
  const el = e.target as HTMLElement
  if (e.metaKey || e.ctrlKey || e.altKey) return true
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)
}

// Page-level shortcut guard: skip when typing or while navigate mode is open.
export function pageKey(e: KeyboardEvent): boolean {
  return !typing(e) && !navigating()
}
