// Uptime pings, as the server sends them (internal/uptime).

export interface UptimeState {
  // up, failing (a ping failed, not yet 3 in a row), down, pending (no ping
  // yet), off (checks turned off), none (no URL).
  status: 'up' | 'failing' | 'down' | 'pending' | 'off' | 'none'
  url?: string
  latency_ms?: number
  checked_at?: string
  down_since?: string
  uptime_24h?: number
}

export interface UptimeCheck {
  checked_at: string
  up: boolean
  status: number
  latency_ms: number
  error?: string
}

export interface UptimeBar {
  start: string
  checks: number
  fails: number
  latency_ms?: number // average of the successful pings
  failures?: { label: string; count: number }[] // "503" or the error, most frequent first
}

// "HTTP 503" for a status, the error text otherwise.
export const failureText = (label: string) => (/^\d+$/.test(label) ? `HTTP ${label}` : label)

export interface UptimeReport extends UptimeState {
  window: string
  bars: UptimeBar[]
  last_failure?: UptimeCheck
}

// "12m", "3h", "2d": short enough for the status bar.
export function shortDuration(fromISO: string, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - new Date(fromISO).getTime()) / 1000))
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

export const percent = (pct: number) => `${pct >= 99.995 ? '100' : pct.toFixed(2)}%`

// A square dot and a short label for an app's uptime.
export function uptimeLabel(state?: UptimeState): { dot: string; text: string; bad: boolean } {
  switch (state?.status) {
    case 'up':
      return { dot: 'bg-accent', text: `up ${state.latency_ms ?? 0}ms`, bad: false }
    case 'failing':
      return { dot: 'border border-error', text: 'failing', bad: true }
    case 'down':
      return { dot: 'bg-error', text: state.down_since ? `down ${shortDuration(state.down_since)}` : 'down', bad: true }
    case 'pending':
      return { dot: 'border border-dashed border-subtle', text: 'first ping soon', bad: false }
    case 'off':
      return { dot: 'border border-border', text: 'uptime off', bad: false }
    default:
      return { dot: 'border border-dashed border-border', text: 'no url', bad: false }
  }
}
