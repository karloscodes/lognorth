import React, { useEffect } from 'react'
import { formatDistanceToNow, format, eachDayOfInterval, subDays, differenceInDays } from 'date-fns'
import { Link, router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { TraceTree } from '@/components/trace-tree'
import { Mono, parseContext, statusColor } from '@/lib/shared'
import type { Application, IssueDetail as IssueDetailType, OccurrencePoint, LogEntry, RelatedLocation } from '@/lib/shared'
import { pageKey } from '@/lib/keys'
import { usePageCommand } from '@/lib/commands'
import { StackTrace } from '@/components/stack-trace'

interface PathCount {
  path: string
  count: number
}

interface Props {
  issue: IssueDetailType
  occurrences: OccurrencePoint[] | null
  traces: Record<string, LogEntry[]> | null
  alsoHappensAt?: RelatedLocation[] | null
  paths?: PathCount[] | null
  appId: number
  appName: string
  applications?: Application[]
}

export default function IssueDetail({ issue, occurrences, traces, alsoHappensAt, paths, appId, appName, applications }: Props) {
  const issuesPath = `/apps/${appId}/issues`
  const toggleMute = () => router.post(`/issues/${issue.hash}/${issue.muted ? 'unmute' : 'mute'}`, {}, { preserveScroll: true })
  usePageCommand('mute', () => toggleMute())

  // esc goes back to the list, m mutes.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!pageKey(e)) return
      if (e.key === 'Escape') router.visit(issuesPath)
      if (e.key === 'm') toggleMute()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const entries = issue.entries || []
  const recentEntries = entries.slice(0, 20)
  const logPath = `/apps/${appId}/log`

  // Structured error identity, from the issue or the newest entry's context.
  const firstCtx = recentEntries.length > 0 ? parseContext(recentEntries[0].context) : null
  const errorClass = issue.error_class || firstCtx?.error_class || firstCtx?.error_type
  const errorFile = issue.error_file || firstCtx?.error_file
  const errorLine = issue.error_line || firstCtx?.error_line
  const errorCaller = firstCtx?.error_caller
  const stackTrace = firstCtx?.stack_trace

  // The most recent entry that has a trace, for the request context.
  const latestTraced = recentEntries.find((e) => e.trace_id && traces?.[e.trace_id])
  const latestTraceEntries = latestTraced && traces ? traces[latestTraced.trace_id!] : null

  const wherePaths = paths ?? []

  return (
    <Layout currentPath="/issues" appId={appId} appName={appName} applications={applications}>
      <p className="text-xs text-dim">
        <Link href={issuesPath} className="hover:text-foreground">
          issues
        </Link>{' '}
        / {errorClass || 'Error'}
        <span className="ml-3">esc back</span>
      </p>

      <div className="mt-4 grid gap-6 md:grid-cols-[1fr_auto] md:items-start">
        <div className="min-w-0">
          <div className="flex gap-2 text-[11px] font-bold">
            <span className="bg-error px-[7px] text-foreground">ERROR</span>
            {issue.active && <span className="border border-error px-[7px] font-medium text-error">active</span>}
            {issue.muted && <span className="border border-border px-[7px] font-medium text-subtle">muted</span>}
          </div>
          <p className="mt-3 max-w-[60ch] break-words text-[21px] font-bold leading-snug text-foreground">{issue.error}</p>
          {(errorClass || errorFile) && (
            <p className="mt-2.5 text-[13px] text-dim">
              {errorClass && <span className="font-bold text-foreground">{errorClass}</span>}
              {errorFile && (
                <>
                  {errorClass ? ' at ' : ''}
                  <span className="text-subtle">
                    {errorFile}
                    {errorLine ? `:${errorLine}` : ''}
                  </span>
                </>
              )}
              {errorCaller && (
                <>
                  {' '}in <span className="text-subtle">{errorCaller}</span>
                </>
              )}
            </p>
          )}
          <AlsoHappensAt locations={alsoHappensAt} appId={appId} />
        </div>
        <button onClick={toggleMute} className="key justify-self-start">
          <kbd>m</kbd> {issue.muted ? 'unmute' : 'mute'}
        </button>
      </div>

      <div className="mt-7 grid grid-cols-2 border border-border md:grid-cols-4 [&>div:nth-child(n+3)]:max-md:border-t [&>div:nth-child(odd)]:max-md:border-l-0 [&>div]:border-l [&>div]:border-line [&>div:first-child]:border-l-0">
        <Stat label="times" value={<Mono className="text-[30px] font-bold leading-none text-error">{issue.count}×</Mono>} />
        <Stat label="first seen" value={formatDistanceToNow(new Date(issue.first_seen), { addSuffix: true })} />
        <Stat label="last seen" value={formatDistanceToNow(new Date(issue.last_seen), { addSuffix: true })} />
        <Stat label="where it happened" value={wherePaths.length ? `${wherePaths.length} endpoint${wherePaths.length !== 1 ? 's' : ''}` : 'no requests'} />
      </div>

      {stackTrace && <StackTrace trace={stackTrace} errorFile={errorFile} />}

      <FrequencyHeatmap occurrences={occurrences} firstSeen={issue.first_seen} />

      {wherePaths.length > 0 && (
        <div className="pane mt-7 px-4 pb-4 pt-6">
          <span className="pane-title">where it happened</span>
          <div className="flex flex-wrap gap-x-7 gap-y-2 text-[13px]">
            {wherePaths.map((p) => (
              <a key={p.path} href={`${logPath}?search=${encodeURIComponent(p.path)}&is_error=true`} className="text-foreground hover:underline">
                {p.path}
                <Mono className="ml-1.5 text-dim">{p.count}</Mono>
              </a>
            ))}
          </div>
        </div>
      )}

      {/* Request context: the log lines around the newest traced error. */}
      {latestTraced && latestTraceEntries && latestTraceEntries.length > 1 && latestTraceEntries.some((e) => e.id === latestTraced.id) && (
        <div className="mt-7">
          <TraceTree title="Request Context" entries={latestTraceEntries} currentId={latestTraced.id} logPath={logPath} grouped traceId={latestTraced.trace_id} />
        </div>
      )}

      {recentEntries.length > 0 && (
        <div className="pane mt-7 pb-1.5 pt-3.5">
          <span className="pane-title">last {recentEntries.length} times</span>
          {recentEntries.map((entry) => (
            <Occurrence key={entry.id} entry={entry} href={`${logPath}/${entry.id}`} />
          ))}
        </div>
      )}
    </Layout>
  )
}

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="px-[18px] py-3.5">
      <p className="text-xs text-dim">{label}</p>
      <div className="mt-0.5 text-[15px] text-foreground">{value}</div>
    </div>
  )
}

// "also at line 45 (12×), and in other.rb:10 (3×)": the same error elsewhere.
function AlsoHappensAt({ locations, appId }: { locations?: RelatedLocation[] | null; appId: number }) {
  if (!locations?.length) return null
  const sameFile = locations.filter((l) => l.same_file)
  const diffFile = locations.filter((l) => !l.same_file)
  const link = (loc: RelatedLocation, label: string) => (
    <React.Fragment key={loc.hash}>
      <a href={`/apps/${appId}/issues/${loc.hash}`} className="underline hover:text-foreground">
        {label}
      </a>{' '}
      ({loc.count}×)
    </React.Fragment>
  )
  const join = (nodes: React.ReactNode[]) => nodes.map((n, i) => <React.Fragment key={i}>{i > 0 && ', '}{n}</React.Fragment>)
  return (
    <p className="mt-1 text-[13px] text-dim">
      {sameFile.length > 0 && (
        <>
          also at line{sameFile.length > 1 ? 's' : ''} {join(sameFile.slice(0, 2).map((l) => link(l, String(l.line))))}
          {sameFile.length > 2 && ` (+${sameFile.length - 2} more)`}
        </>
      )}
      {diffFile.length > 0 && (
        <>
          {sameFile.length > 0 ? ', and in ' : 'same error in '}
          {join(diffFile.slice(0, 2).map((l) => link(l, `${l.file}${l.line > 0 ? `:${l.line}` : ''}`)))}
          {diffFile.length > 2 && ` (+${diffFile.length - 2} more)`}
        </>
      )}
    </p>
  )
}

function Occurrence({ entry, href }: { entry: LogEntry; href: string }) {
  const ctx = parseContext(entry.context)
  return (
    <a
      href={href}
      className="grid grid-cols-[6ch_5ch_1fr_16ch_9ch_14ch] gap-3.5 whitespace-nowrap border-t border-line px-[18px] py-1.5 text-[13px] first-of-type:border-t-0 hover:bg-raise max-md:grid-cols-[5ch_4ch_1fr_9ch]"
    >
      <span className="text-dim">{ctx.job ? 'JOB' : ctx.method}</span>
      <span className={ctx.status ? statusColor(ctx.status) : 'text-dim'}>{ctx.status}</span>
      <span className="truncate text-foreground">{ctx.job || ctx.path || entry.message}</span>
      <span className="text-dim max-md:hidden">{format(new Date(entry.timestamp), 'MMM d HH:mm:ss').toLowerCase()}</span>
      <Mono className="text-right text-subtle">{entry.duration_ms.toLocaleString()}ms</Mono>
      <span className="truncate text-dim max-md:hidden">{entry.trace_id ? `trace:${entry.trace_id.slice(0, 8)}` : ''}</span>
    </a>
  )
}

// --- Frequency heatmap ---
// Full width, right edge = today, filling backwards. 12 two-hour rows per day.

const TIME_BUCKETS = [
  { label: '8am', start: 8, end: 9 },
  { label: '10am', start: 10, end: 11 },
  { label: '12pm', start: 12, end: 13 },
  { label: '2pm', start: 14, end: 15 },
  { label: '4pm', start: 16, end: 17 },
  { label: '6pm', start: 18, end: 19 },
  { label: '8pm', start: 20, end: 21 },
  { label: '10pm', start: 22, end: 23 },
  { label: '12am', start: 0, end: 1 },
  { label: '2am', start: 2, end: 3 },
  { label: '4am', start: 4, end: 5 },
  { label: '6am', start: 6, end: 7 },
]

const LABEL_WIDTH = 40
const CELL = 11 // px, with a 2px gap
const EMPTY = 'hsl(var(--raise))'
const errorShade = (alpha: number) => `hsl(var(--error) / ${alpha})`

function FrequencyHeatmap({ occurrences, firstSeen }: { occurrences: OccurrencePoint[] | null; firstSeen: string }) {
  const containerRef = React.useRef<HTMLDivElement>(null)
  const [numCols, setNumCols] = React.useState(0)
  const [hovered, setHovered] = React.useState<{ day: Date; bucket: (typeof TIME_BUCKETS)[number]; count: number } | null>(null)

  // Measure before paint so the grid fills the full width.
  React.useLayoutEffect(() => {
    const measure = () => {
      if (!containerRef.current) return
      const available = containerRef.current.clientWidth - LABEL_WIDTH - 40
      setNumCols(Math.max(7, Math.floor(available / (CELL + 2))))
    }
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [])

  // Buckets arrive as RFC3339 UTC hours; key them by local "YYYY-MM-DD HH" so
  // the axes follow the viewer's timezone.
  const countMap = new Map<string, number>()
  for (const p of occurrences ?? []) {
    const d = new Date(p.hour)
    if (isNaN(d.getTime())) continue
    const key = `${format(d, 'yyyy-MM-dd')} ${String(d.getHours()).padStart(2, '0')}`
    countMap.set(key, (countMap.get(key) || 0) + p.count)
  }
  const bucketCount = (dayStr: string, bucket: (typeof TIME_BUCKETS)[number]) => {
    let total = 0
    for (let h = bucket.start; h <= bucket.end; h++) total += countMap.get(`${dayStr} ${String(h).padStart(2, '0')}`) || 0
    return total
  }

  const end = new Date()
  let dataStart = subDays(end, numCols - 1)
  const first = new Date(firstSeen)
  if (!isNaN(first.getTime()) && first < dataStart) dataStart = first
  const totalDays = Math.max(numCols, differenceInDays(end, dataStart) + 1)
  const days = eachDayOfInterval({ start: subDays(end, totalDays - 1), end })

  let maxBucket = 1
  for (const day of days) {
    const dayStr = format(day, 'yyyy-MM-dd')
    for (const bucket of TIME_BUCKETS) maxBucket = Math.max(maxBucket, bucketCount(dayStr, bucket))
  }
  const needsScroll = days.length > numCols
  const weeks = Math.max(1, Math.round(days.length / 7))

  return (
    <div className="pane mt-7 px-4 pb-4 pt-6" ref={containerRef}>
      <span className="pane-title">
        frequency · {weeks} week{weeks !== 1 ? 's' : ''}
      </span>
      <span className="pane-right">
        less
        {[0, 0.35, 0.65, 1].map((a) => (
          <i key={a} className="inline-block h-2.5 w-1" style={{ background: a === 0 ? EMPTY : errorShade(a) }} />
        ))}
        more
      </span>
      {numCols === 0 ? (
        <div className="h-[170px]" />
      ) : (
        <>
          <div className="flex">
            <div className="shrink-0 pr-2" style={{ width: LABEL_WIDTH }}>
              {TIME_BUCKETS.map((bucket, i) => (
                <div key={bucket.label} className="flex items-center justify-end text-[10px] text-dim" style={{ height: CELL + 2 }}>
                  {i % 2 === 0 ? bucket.label : ''}
                </div>
              ))}
            </div>
            <div
              className="flex-1 overflow-x-auto overflow-y-hidden scrollbar-hide"
              ref={(el) => {
                if (el && needsScroll) el.scrollLeft = el.scrollWidth
              }}
            >
              <div style={{ display: 'grid', gridTemplateColumns: `repeat(${days.length}, 1fr)`, gap: 2, minWidth: needsScroll ? days.length * (CELL + 2) : undefined }}>
                {TIME_BUCKETS.flatMap((bucket) =>
                  days.map((day, i) => {
                    const count = bucketCount(format(day, 'yyyy-MM-dd'), bucket)
                    const isHovered = hovered?.day === day && hovered?.bucket === bucket
                    return (
                      <div
                        key={`${bucket.label}-${i}`}
                        className={`cursor-crosshair ${hovered && !isHovered ? 'opacity-40' : ''}`}
                        style={{ aspectRatio: '1', background: count > 0 ? errorShade(Math.max(0.35, count / maxBucket)) : EMPTY }}
                        onMouseEnter={() => setHovered({ day, bucket, count })}
                        onMouseLeave={() => setHovered(null)}
                      />
                    )
                  }),
                )}
              </div>
              <div className="relative mt-1.5 h-3.5" style={{ minWidth: needsScroll ? days.length * (CELL + 2) : undefined }}>
                {days.map((day, i) => {
                  const spacing = Math.max(10, Math.floor(days.length / 8))
                  if (i !== 0 && i % spacing !== 0) return null
                  return (
                    <span key={i} className="absolute whitespace-nowrap text-[10px] text-dim" style={{ left: `${(i / days.length) * 100}%` }}>
                      {format(day, 'MMM d').toLowerCase()}
                    </span>
                  )
                })}
              </div>
            </div>
          </div>
          <p className="mt-2 h-4 text-center text-xs text-subtle">
            {hovered && (
              <>
                {format(hovered.day, 'EEE, MMM d')} {hovered.bucket.label} ·{' '}
                <span className="text-foreground">
                  {hovered.count} error{hovered.count !== 1 ? 's' : ''}
                </span>
              </>
            )}
          </p>
        </>
      )}
    </div>
  )
}
