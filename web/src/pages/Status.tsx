import { useEffect, useState } from 'react'
import { FIELD, Notice } from '@/components/auth-shell'
import { PageHeader } from '@/lib/shared'
import { percent, shortDuration, uptimeLabel, type UptimeState } from '@/lib/uptime'

// A public status page, as internal/status sends it. It wears the app's
// chrome: the header, PageHeader, panes of rows, and the status bar.
interface Day {
  date: string // "2026-10-11", UTC
  checks: number
  fails: number
  down_minutes: number
}

interface StatusApp extends UptimeState {
  id: number
  name: string
  uptime?: number // percent of the window's pings that were up
  days: Day[]
  since?: string // the first day with pings, "2026-03-04"
  all_time?: number // percent of all pings since then that were up
}

interface Comment {
  id: number
  body: string
  created_at: string
}

interface Incident {
  app_id: number
  app_name: string
  start: string // sent back as-is: it names the outage
  end: string
  ongoing?: boolean
  comments: Comment[]
}

interface Props {
  report: { page: { name: string; slug: string; domain: string }; apps: StatusApp[]; incidents: Incident[] }
  signedIn?: boolean
  onOwnDomain?: boolean // served at the page's own domain, not at /status/<slug>
  flash?: { type?: string; message?: string }
}

const MONTHS = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec']

// "jan 15" from "2026-01-15".
const dayLabel = (date: string) => `${MONTHS[Number(date.slice(5, 7)) - 1]} ${Number(date.slice(8, 10))}`

// "jan 15 11:01" in UTC, like the days.
function utc(iso: string): string {
  const d = new Date(iso)
  const hh = String(d.getUTCHours()).padStart(2, '0')
  const mm = String(d.getUTCMinutes()).padStart(2, '0')
  return `${MONTHS[d.getUTCMonth()]} ${d.getUTCDate()} ${hh}:${mm}`
}

// "14:09", with the day too when the outage ended on another day.
function until(start: string, end: string): string {
  const [from, to] = [utc(start).split(' '), utc(end).split(' ')]
  const sameDay = from[0] === to[0] && from[1] === to[1]
  return sameDay ? to[2] : to.join(' ')
}

// "7m", "1h 12m", "2d 3h".
function minutes(total: number): string {
  if (total < 60) return `${total}m`
  if (total < 1440) return `${Math.floor(total / 60)}h${total % 60 ? ` ${total % 60}m` : ''}`
  return `${Math.floor(total / 1440)}d${Math.floor((total % 1440) / 60) ? ` ${Math.floor((total % 1440) / 60)}h` : ''}`
}

const span = (start: string, end: string) => minutes(Math.max(1, Math.round((new Date(end).getTime() - new Date(start).getTime()) / 60000)))

// The face, the headline, and the status bar's word for all apps at once.
function overall(apps: StatusApp[]): { face: string; tone: string; title: string; short: string; bad: boolean } {
  const down = apps.filter((a) => a.status === 'down')
  if (apps.length === 0) return { face: '(-_-)', tone: 'text-dim', title: 'Quiet so far.', short: 'no apps yet', bad: false }
  if (down.length === 1) return { face: '(x_x)', tone: 'text-error', title: `${down[0].name} is down.`, short: `${down[0].name} down`, bad: true }
  if (down.length > 1) return { face: '(x_x)', tone: 'text-error', title: `${down.length} apps are down.`, short: `${down.length} down`, bad: true }
  if (apps.some((a) => a.status === 'failing')) return { face: '(o_o)', tone: 'text-foreground', title: 'A ping just failed. Watching.', short: 'failing', bad: true }
  return { face: '(^_^)', tone: 'text-accent', title: 'Everything is up.', short: 'all up', bad: false }
}

// The face blinks every few seconds while everything is up.
function useBlink(on: boolean): boolean {
  const [closed, setClosed] = useState(false)
  useEffect(() => {
    if (!on || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    let open: number | undefined
    const timer = window.setInterval(() => {
      setClosed(true)
      open = window.setTimeout(() => setClosed(false), 160)
    }, 4000)
    return () => {
      window.clearInterval(timer)
      window.clearTimeout(open)
    }
  }, [on])
  return closed
}

export default function Status({ report, signedIn = false, onOwnDomain = false, flash }: Props) {
  const { page, apps, incidents } = report
  const head = overall(apps)
  const blink = useBlink(head.face === '(^_^)')
  const lastPing = apps.map((a) => a.checked_at).filter(Boolean).sort().pop()
  const days = apps[0]?.days.length ?? 90

  useEffect(() => {
    document.title = `${page.name} status`
  }, [page.name])

  return (
    <div className="min-h-screen bg-background pb-0 md:pb-7">
      {/* The app's header: the page's name as a wordmark, and its sections. */}
      <header className="sticky top-0 z-50 border-b border-line bg-background">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-5 px-4 md:px-8">
          <a href={onOwnDomain ? '/' : `/status/${page.slug}`} className="min-w-0 truncate text-[17px] font-bold tracking-tight">
            <span className="text-accent">[</span>
            {page.name}
            <span className="text-accent">]</span>
          </a>
          <span className="hidden text-[13px] text-dim sm:inline">status</span>
          <span className="flex-1" />
          <nav className="flex items-center gap-4 text-[13px] text-subtle" aria-label="Sections">
            {apps.length > 0 && (
              <>
                <a href="#uptime" className="hover:text-foreground">
                  uptime
                </a>
                <a href="#incidents" className="hover:text-foreground">
                  incidents
                </a>
              </>
            )}
            {signedIn && (
              <a href="/settings/status" className="hover:text-foreground">
                settings
              </a>
            )}
          </nav>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-10 md:px-8">
        <PageHeader
          title={
            <>
              <span className={`whitespace-pre ${head.tone}`} aria-hidden="true">
                {blink ? '(-_-)' : head.face}
              </span>{' '}
              {head.title}
            </>
          }
          subtitle={
            apps.length === 0 ? (
              signedIn ? (
                <>
                  No app on this page yet.{' '}
                  <a href="/apps" className="text-foreground hover:underline">
                    Pick its apps on the Apps page →
                  </a>
                </>
              ) : (
                'Nothing to show yet.'
              )
            ) : (
              <>
                <span className="tabular-nums">{apps.length}</span> app{apps.length !== 1 ? 's' : ''} · pinged every minute
                {lastPing && <> · last ping {shortDuration(lastPing)} ago</>}
              </>
            )
          }
        />

        {flash?.message && (flash.type === 'error' || flash.type === 'success') && <Notice kind={flash.type}>{flash.message}</Notice>}

        {apps.length > 0 && (
          <>
            <div id="uptime" className="pane mb-7 scroll-mt-20 pb-0 pt-3.5">
              <span className="pane-title">uptime</span>
              <span className="pane-right">last {days} days</span>
              {apps.map((app) => (
                <AppRow key={app.id} app={app} />
              ))}
            </div>

            <div id="incidents" className="pane scroll-mt-20 pb-0 pt-3.5">
              <span className="pane-title">incidents</span>
              <span className="pane-right">last 30 days · utc</span>
              {incidents.length === 0 ? (
                <p className="px-[18px] py-3.5 text-[13px] text-subtle">No incidents. Enjoy your day.</p>
              ) : (
                incidents.map((incident) => <IncidentRow key={`${incident.app_id}-${incident.start}`} incident={incident} slug={page.slug} signedIn={signedIn} />)
              )}
            </div>
          </>
        )}
      </main>

      <footer className="pb-6 text-center text-xs text-dim md:pb-12">
        <a href="https://lognorth.com/uptime/" className="hover:text-foreground">
          uptime by LogNorth
        </a>
        {!signedIn && !onOwnDomain && (
          <>
            {' · '}
            <a href="/login" className="hover:text-foreground">
              sign in to post notes
            </a>
          </>
        )}
      </footer>

      {/* The app's status bar: the mode, the page, and the state now. Hidden on phones. */}
      <div className="status-bar fixed inset-x-0 bottom-0 z-40 hidden h-7 border-t border-line bg-raise text-xs text-subtle md:flex">
        <span className="flex items-center bg-border px-3 font-bold uppercase text-foreground">status</span>
        <span className="flex items-center border-r border-line px-3 text-foreground">{page.name}</span>
        <span className={`flex items-center gap-1.5 px-3 ${head.bad ? 'font-bold text-error' : ''}`}>
          <i className={`h-[7px] w-[7px] ${head.bad ? 'bg-error' : apps.length ? 'bg-accent' : 'border border-dashed border-border'}`} />
          {head.short}
        </span>
        <span className="flex-1" />
        {lastPing && <span className="flex items-center border-l border-line px-3">last ping {shortDuration(lastPing)} ago</span>}
        <span className="flex items-center border-l border-line px-3">every minute</span>
      </div>
    </div>
  )
}

// Phones show the last 30 days; wider screens all of them.
const PHONE_DAYS = 30

// One app: its state, its uptime, and a bar per day. A row of the uptime
// pane, as an app is a row of the Apps page.
function AppRow({ app }: { app: StatusApp }) {
  // The hovered day, and where its bar sits across the strip (0 to 1).
  const [hovered, setHovered] = useState<{ i: number; at: number } | null>(null)
  const checked = app.status !== 'none' && app.status !== 'off'
  const label = uptimeLabel(app)

  const hover = (i: number) => (e: React.SyntheticEvent<HTMLElement>) => {
    const bar = e.currentTarget
    const strip = bar.parentElement!
    setHovered({ i, at: (bar.offsetLeft + bar.offsetWidth / 2) / strip.offsetWidth })
  }

  return (
    <div className="border-t border-line px-[18px] py-4 text-[13px] first-of-type:border-t-0">
      <div className="mb-3 flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <span className="text-[15px] font-bold text-foreground">{app.name}</span>
        <span className="flex items-center gap-1.5 text-xs">
          {checked ? (
            <>
              <i className={`h-[7px] w-[7px] ${label.dot}`} />
              <span className={label.bad ? 'font-bold text-error' : 'text-subtle'}>{label.text}</span>
              {app.status === 'down' && app.down_since && <span className="text-dim">· since {utc(app.down_since)}</span>}
            </>
          ) : (
            <>
              <i className="h-[7px] w-[7px] border border-dashed border-border" />
              <span className="text-subtle">not checked right now</span>
            </>
          )}
        </span>
        <span className="flex-1" />
        <UptimeFigures app={app} />
      </div>

      <div className="relative" onMouseLeave={() => setHovered(null)}>
        {hovered && <DayTooltip day={app.days[hovered.i]} at={hovered.at} />}
        <div className="relative flex h-7 items-stretch gap-px">
          {app.days.map((day, i) => (
            <i
              key={day.date}
              onMouseEnter={hover(i)}
              onClick={hover(i)}
              aria-label={`${dayLabel(day.date)}: ${dayText(day)}`}
              className={`relative flex-1 cursor-crosshair ${dayColor(day, hovered?.i === i)} ${i < app.days.length - PHONE_DAYS ? 'hidden sm:block' : ''}`}
            >
              {day.down_minutes === 0 && day.fails > 0 && <b className="absolute inset-x-0 bottom-0 h-1/3 bg-error" />}
            </i>
          ))}
        </div>
      </div>
      <div className="mt-1.5 flex justify-between text-[10px] text-dim">
        <span>
          <span className="sm:hidden">{Math.min(PHONE_DAYS, app.days.length)}</span>
          <span className="hidden sm:inline">{app.days.length}</span> days ago
        </span>
        <span>today</span>
      </div>
    </div>
  )
}

// "99.991%": all-time uptime takes 3 decimals, as a year of pings measures
// it to a ten-thousandth of a percent.
const percent3 = (pct: number) => `${pct >= 99.9995 ? '100' : pct.toFixed(3)}%`

// "mar 4", with the year when it is not this one.
function sinceLabel(date: string): string {
  const year = date.slice(0, 4)
  return year === String(new Date().getUTCFullYear()) ? dayLabel(date) : `${dayLabel(date)} ${year}`
}

// The uptime of the bars, and since the first ping. When the first ping is
// inside the bars, the two are the same number: show it once, with "since".
function UptimeFigures({ app }: { app: StatusApp }) {
  if (app.all_time === undefined || !app.since) return null
  const longer = app.days.length > 0 && app.since < app.days[0].date
  return (
    <span className="text-xs text-dim">
      {longer && app.uptime !== undefined && (
        <>
          <span className="tabular-nums text-foreground">{percent(app.uptime)}</span> {app.days.length}d ·{' '}
        </>
      )}
      <span className="tabular-nums text-foreground">{percent3(app.all_time)}</span> since {sinceLabel(app.since)}
    </span>
  )
}

// An up day is the accent, the color of up across LogNorth, and a day with an
// outage is the error red. A day with a failed ping or two but no outage
// stays up, with a red base (in the bar). A day without pings is a hairline.
// The hovered day turns to the foreground.
function dayColor(day: Day, hovered: boolean): string {
  if (day.checks === 0) return 'bg-line'
  if (day.down_minutes > 0) return 'bg-error'
  return hovered ? 'bg-foreground' : 'bg-accent'
}

function dayText(day: Day): string {
  if (day.checks === 0) return 'no pings'
  const pings = `${day.checks.toLocaleString()} ping${day.checks !== 1 ? 's' : ''}`
  if (day.fails === 0) return `${pings} · all up`
  const down = day.down_minutes > 0 ? ` · down ${minutes(day.down_minutes)}` : ''
  return `${pings} · ${day.fails} failed${down}`
}

function DayTooltip({ day, at }: { day: Day; at: number }) {
  // Near the edges the box grows inward instead of centering on the bar.
  const pos = at < 0.15 ? { left: 0 } : at > 0.85 ? { right: 0 } : { left: `${at * 100}%`, transform: 'translateX(-50%)' }
  return (
    <div className="pane absolute bottom-[calc(100%+10px)] z-10 w-max max-w-[300px] bg-background px-3 pb-2 pt-3.5 text-xs" style={pos}>
      <span className="pane-title text-foreground">{dayLabel(day.date)}</span>
      <p className={day.down_minutes > 0 ? 'font-bold text-error' : 'text-subtle'}>{dayText(day)}</p>
    </div>
  )
}

// One outage: when, which app, how long, and the owner's notes under it. A
// row of the incidents pane, with the time in a fixed column.
function IncidentRow({ incident, slug, signedIn }: { incident: Incident; slug: string; signedIn: boolean }) {
  const confirmDelete = (e: React.FormEvent) => {
    if (!confirm('Delete this note? Visitors will stop seeing it.')) e.preventDefault()
  }

  return (
    <div className="border-t border-line px-[18px] py-3.5 text-[13px] first-of-type:border-t-0">
      <div className="grid grid-cols-[1fr_auto] items-baseline gap-x-4 gap-y-1 sm:grid-cols-[110px_1fr_auto]">
        <span className="tabular-nums text-subtle">{utc(incident.start)}</span>
        <p className="order-last col-span-2 min-w-0 sm:order-none sm:col-span-1">
          <b className="text-foreground">{incident.app_name}</b> <span className="text-text">{incident.ongoing ? "isn't answering." : 'stopped answering.'}</span>{' '}
          <span className="text-dim">{incident.ongoing ? `${span(incident.start, incident.end)} so far.` : `Back at ${until(incident.start, incident.end)}.`}</span>
        </p>
        {incident.ongoing ? <b className="text-error">ongoing</b> : <span className="tabular-nums text-error">down {span(incident.start, incident.end)}</span>}
      </div>

      {(incident.comments.length > 0 || signedIn) && (
        <div className="sm:ml-[126px]">
          {incident.comments.length > 0 && (
            <ul className="mt-2.5 space-y-2 border-l border-border pl-3">
              {incident.comments.map((c) => (
                <li key={c.id} className="group">
                  <p className="whitespace-pre-wrap break-words text-text">{c.body}</p>
                  <p className="mt-0.5 flex items-center gap-3 text-[11px] text-dim">
                    {utc(c.created_at)}
                    {signedIn && (
                      <form action={`/status/comments/${c.id}/delete`} method="POST" onSubmit={confirmDelete}>
                        <input type="hidden" name="page" value={slug} />
                        <button type="submit" className="opacity-0 hover:text-error focus:opacity-100 group-hover:opacity-100">
                          delete
                        </button>
                      </form>
                    )}
                  </p>
                </li>
              ))}
            </ul>
          )}
          {signedIn && <NoteForm incident={incident} slug={slug} />}
        </div>
      )}
    </div>
  )
}

// The owner tells visitors what happened. Collapsed until asked for.
function NoteForm({ incident, slug }: { incident: Incident; slug: string }) {
  const [open, setOpen] = useState(false)
  const [body, setBody] = useState('')
  const id = `note-${incident.app_id}-${incident.start}`

  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)} className="key mt-2.5">
        + add a note
      </button>
    )
  }

  return (
    <form action="/status/comments" method="POST" className="mt-2.5 flex flex-col gap-2">
      <input type="hidden" name="app_id" value={incident.app_id} />
      <input type="hidden" name="start" value={incident.start} />
      <input type="hidden" name="page" value={slug} />
      <label htmlFor={id} className="sr-only">
        Note for visitors
      </label>
      <textarea
        id={id}
        name="body"
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => e.key === 'Escape' && setOpen(false)}
        maxLength={1000}
        rows={3}
        autoFocus
        placeholder="What happened, and what you did. Visitors see this."
        className={`${FIELD} resize-y`}
      />
      <div className="flex items-center gap-3">
        <button type="submit" disabled={!body.trim()} className="bg-accent px-4 py-1.5 text-[13px] font-bold text-accent-foreground hover:bg-foreground disabled:opacity-40">
          Post note
        </button>
        <button type="button" onClick={() => setOpen(false)} className="text-xs text-dim hover:text-foreground">
          cancel
        </button>
      </div>
    </form>
  )
}
