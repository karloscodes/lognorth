// The : command line, vim style. The layout owns the line and the commands
// that work on every screen; a page adds the ones that act on what it shows,
// like :mute on an issue.
import { useEffect } from 'react'
import { router } from '@inertiajs/react'
import type { Application } from '@/lib/shared'
import { THEMES, isTheme, setTheme } from '@/lib/theme'

export interface CommandContext {
  appId?: number
  applications?: Application[]
  openHelp: () => void
}

// A command returns nothing, or a line for the status bar. A line that starts
// with "E" is an error, as in vim.
type Result = string | void

interface Command {
  help: string
  args?: (ctx: CommandContext) => string[] // what Tab offers after the name
  run: (arg: string, ctx: CommandContext) => Result
}

const pageCommands = new Map<string, (arg: string) => Result>()

// usePageCommand adds a command while the page is on screen.
export function usePageCommand(name: string, run: (arg: string) => Result) {
  useEffect(() => {
    pageCommands.set(name, run)
    return () => {
      pageCommands.delete(name)
    }
  })
}

// Where switching apps lands: the same section, if the app has it.
export function sectionPath(appId: number): string {
  const path = window.location.pathname
  if (path.includes('/log')) return `/apps/${appId}/log`
  if (path.includes('/endpoints')) return `/apps/${appId}/endpoints`
  return `/apps/${appId}/issues`
}

// The app a command acts on: the open one, or the only one there is.
function currentApp(ctx: CommandContext): number | undefined {
  if (ctx.appId) return ctx.appId
  if (ctx.applications?.length === 1) return ctx.applications[0].id
  return undefined
}

const NO_APP = 'E: open an app first, e.g. :app checkout-prod'

function inApp(ctx: CommandContext, section: string): Result {
  const app = currentApp(ctx)
  if (!app) return NO_APP
  router.visit(`/apps/${app}/${section}`)
}

// logsWith opens the logs with some filters changed. On the logs page it
// keeps the filters already set, so :since after :search narrows the search.
function logsWith(ctx: CommandContext, set: Record<string, string>): Result {
  const app = currentApp(ctx)
  if (!app) return NO_APP
  const base = `/apps/${app}/log`
  const params = new URLSearchParams(window.location.pathname === base ? window.location.search : '')
  params.delete('offset')
  for (const [k, v] of Object.entries(set)) params.set(k, v)
  router.visit(`${base}?${params}`)
}

// "2h" as the start_time the logs page reads: a time, or "all".
function startTime(arg: string): string | undefined {
  if (arg === 'all') return 'all'
  const m = /^(\d+)(m|h|d)$/.exec(arg)
  if (!m || Number(m[1]) === 0) return undefined
  const unit = { m: 60_000, h: 3_600_000, d: 86_400_000 }[m[2] as 'm' | 'h' | 'd']
  return new Date(Date.now() - Number(m[1]) * unit).toISOString()
}

export const COMMANDS: Record<string, Command> = {
  app: {
    help: 'switch app: :app checkout-prod',
    args: (ctx) => ctx.applications?.map((a) => a.name) ?? [],
    run: (arg, ctx) => {
      const want = arg.toLowerCase()
      const app = ctx.applications?.find((a) => a.name.toLowerCase() === want) ?? ctx.applications?.find((a) => a.name.toLowerCase().startsWith(want))
      if (!arg || !app) return `E: no app named ${arg || '(empty)'}`
      router.visit(sectionPath(app.id))
    },
  },
  issues: { help: 'issues of this app', run: (_, ctx) => inApp(ctx, 'issues') },
  logs: { help: 'logs of this app', run: (_, ctx) => inApp(ctx, 'log') },
  endpoints: { help: 'endpoints and uptime', run: (_, ctx) => inApp(ctx, 'endpoints') },
  search: {
    help: 'search the logs: :search 503',
    run: (arg, ctx) => (arg ? logsWith(ctx, { search: arg }) : 'E: search for what? e.g. :search 503'),
  },
  since: {
    help: 'logs since 15m, 2h, 7d, or all',
    args: () => ['15m', '1h', '2h', '6h', '24h', '7d', '30d', 'all'],
    run: (arg, ctx) => {
      const start = startTime(arg)
      return start ? logsWith(ctx, { start_time: start }) : 'E: try :since 15m, 2h, 7d, or all'
    },
  },
  theme: {
    help: 'color theme: :theme gruvbox',
    args: () => THEMES.map((t) => t.id),
    run: (arg) => {
      if (!arg) return `themes: ${THEMES.map((t) => t.id).join(' ')}`
      const want = arg.toLowerCase()
      if (!isTheme(want)) return `E: no theme ${arg}. Try ${THEMES.map((t) => t.id).join(', ')}`
      setTheme(want)
      return `theme ${want}`
    },
  },
  apps: { help: 'all apps', run: () => router.visit('/apps') },
  settings: { help: 'settings', run: () => router.visit('/settings') },
  help: { help: 'these keys', run: (_, ctx) => ctx.openHelp() },
  q: { help: 'home', run: () => router.visit('/') },
}

const ALIASES: Record<string, string> = { quit: 'q', 'q!': 'q', home: 'q', h: 'help', log: 'logs' }

// Shown in help. :mute is a page command, so it is listed by hand.
export const COMMAND_HELP: [string, string][] = [
  [':app', COMMANDS.app.help],
  [':search', COMMANDS.search.help],
  [':since', COMMANDS.since.help],
  [':mute', 'mute the issue on screen'],
  [':theme', COMMANDS.theme.help],
  [':logs', 'switch tab, also :issues :endpoints'],
  [':q', 'home'],
  ['tab', 'complete'],
]

export function runCommand(line: string, ctx: CommandContext): Result {
  const text = line.trim().replace(/^:/, '')
  if (!text) return
  const [word, ...rest] = text.split(/\s+/)
  const name = ALIASES[word] ?? word
  const arg = rest.join(' ')

  const page = pageCommands.get(name)
  if (page) return page(arg)
  const command = COMMANDS[name]
  if (command) return command.run(arg, ctx)
  if (name === 'mute') return 'E: :mute works on an issue or the issue list'
  return `E492: Not an editor command: ${text}`
}

// completions lists what Tab can put in place of the word being typed: a
// command name, or the argument of the command before it.
export function completions(line: string, ctx: CommandContext): { base: string; options: string[] } {
  const m = /^(\S+)(\s+)(.*)$/.exec(line)
  if (!m) {
    const names = [...new Set([...Object.keys(COMMANDS), ...pageCommands.keys()])].sort()
    return { base: '', options: names.filter((n) => n.startsWith(line)) }
  }
  const name = ALIASES[m[1]] ?? m[1]
  const typed = m[3].toLowerCase()
  const args = COMMANDS[name]?.args?.(ctx) ?? []
  return { base: m[1] + ' ', options: args.filter((a) => a.toLowerCase().startsWith(typed)) }
}

// The lines run this session, for ↑ and ↓.
export const history: string[] = []
