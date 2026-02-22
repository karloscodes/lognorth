import React from 'react'
import { formatDistanceToNow, format, eachDayOfInterval, subDays, differenceInDays } from 'date-fns'
import { router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { Mono, Code, parseContext, getConfig, methodColor, statusColor } from '@/lib/shared'
import type { Application, IssueDetail as IssueDetailType, OccurrencePoint, LogEntry } from '@/lib/shared'

interface Props {
  issue: IssueDetailType
  occurrences: OccurrencePoint[] | null
  traces: Record<string, LogEntry[]> | null
  appId: number
  appName: string
  applications?: Application[]
}

export default function IssueDetail({ issue, occurrences, traces, appId, appName, applications }: Props) {
  const toggleMute = () => {
    const url = issue.muted ? `/issues/${issue.hash}/unmute` : `/issues/${issue.hash}/mute`
    router.post(url, {}, { preserveScroll: true })
  }

  const entries = issue.entries || []
  const recentEntries = entries.slice(0, 20)
  const logPath = `/apps/${appId}/log`

  // Extract error_type/error_location from first entry context
  const firstCtx = recentEntries.length > 0 ? parseContext(recentEntries[0].context) : null
  const errorType = firstCtx?.error_type
  const errorLocation = firstCtx?.error_location

  // Latest trace context: find the most recent entry with a trace
  const latestTraced = recentEntries.find((e) => e.trace_id && traces?.[e.trace_id])
  const latestTraceEntries = latestTraced && traces ? traces[latestTraced.trace_id!] : null

  return (
    <Layout currentPath="/issues" appId={appId} appName={appName} applications={applications}>
      {/* Header Card */}
      <div className="border-2 border-black/20 overflow-hidden mb-8">
        <div className="p-6">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-3 mb-3">
                <span className="bg-[#e11d48] text-white px-2 py-0.5 text-xs font-black uppercase">Error</span>
                {issue.active && (
                  <span className="bg-[#e11d48]/10 text-[#e11d48] px-2 py-0.5 text-xs font-black">Active</span>
                )}
                {issue.muted && (
                  <span className="bg-black/10 text-black px-2 py-0.5 text-xs font-black">Muted</span>
                )}
              </div>
              <Code className="text-lg leading-snug block text-black font-bold break-all">{issue.error}</Code>
              {errorType && (
                <div className="mt-2 text-sm font-mono">
                  <span className="text-black font-semibold">{errorType}</span>
                  {errorLocation && <span className="text-black/60"> at {errorLocation}</span>}
                </div>
              )}
            </div>
            <button
              onClick={toggleMute}
              className="shrink-0 px-4 py-2 text-sm font-bold border-2 border-black/20 hover:border-black text-black hover:bg-black hover:text-white transition-colors"
            >
              {issue.muted ? 'Unmute' : 'Mute'}
            </button>
          </div>
        </div>

        <div className="grid grid-cols-3 border-t-2 border-black/10 divide-x-2 divide-black/10 bg-black/[0.02]">
          <div className="px-5 py-3">
            <div className="text-xs text-black/70 font-bold mb-1">Occurrences</div>
            <Mono className="text-2xl font-black text-[#e11d48]">{issue.count}</Mono>
          </div>
          <div className="px-5 py-3">
            <div className="text-xs text-black/70 font-bold mb-1">First seen</div>
            <Mono className="text-sm text-black font-semibold">{formatDistanceToNow(parseSqliteDate(issue.first_seen), { addSuffix: true })}</Mono>
          </div>
          <div className="px-5 py-3">
            <div className="text-xs text-black/70 font-bold mb-1">Last seen</div>
            <Mono className="text-sm text-black font-semibold">{formatDistanceToNow(parseSqliteDate(issue.last_seen), { addSuffix: true })}</Mono>
          </div>
        </div>
      </div>

      {/* Frequency Heatmap */}
      <FrequencyHeatmap occurrences={occurrences} firstSeen={issue.first_seen} />

      {/* In Context — surrounding log lines from the same trace */}
      {latestTraced && latestTraceEntries && (
        <InContextSection
          errorEntry={latestTraced}
          traceEntries={latestTraceEntries}
          logPath={logPath}
        />
      )}

      {/* Recent Occurrences Timeline */}
      {recentEntries.length > 0 && (
        <div className="mb-8">
          <h3 className="text-sm font-black text-black uppercase tracking-wider mb-4">
            Recent Occurrences
          </h3>
          <div className="relative pl-6">
            <div className="absolute left-[7px] top-2 bottom-2 w-px bg-black/20" />
            {recentEntries.map((entry, idx) => (
              <TimelineEntry key={entry.id} entry={entry} appId={appId} isFirst={idx === 0} />
            ))}
          </div>
        </div>
      )}
    </Layout>
  )
}

// --- In Context Section ---
// Shows what happened before/after this error in the same request trace
function InContextSection({ errorEntry, traceEntries, logPath }: { errorEntry: LogEntry; traceEntries: LogEntry[]; logPath: string }) {
  const errorIdx = traceEntries.findIndex((e) => e.id === errorEntry.id)
  if (errorIdx < 0) return null

  const errorCtx = parseContext(errorEntry.context)
  const before = traceEntries.slice(Math.max(0, errorIdx - 3), errorIdx)
  const after = traceEntries.slice(errorIdx + 1, errorIdx + 4)

  return (
    <div className="mb-8">
      <h3 className="text-sm font-black text-black uppercase tracking-wider mb-4">
        In Context
      </h3>
      <div className="border-2 border-black/20 overflow-hidden">

        {/* Log entries before */}
        {before.length > 0 && (
          <>
            <div className="px-4 py-1.5 border-b border-black/10">
              <span className="text-[10px] font-mono font-black text-black/70 uppercase">Before this error</span>
            </div>
            {before.map((e) => (
              <TraceRow key={e.id} entry={e} logPath={logPath} />
            ))}
          </>
        )}

        {/* Current occurrence — prominent */}
        <div className="bg-[#e11d48]/10 border-y-2 border-[#e11d48]/40 px-4 py-4">
          <a href={`${logPath}/${errorEntry.id}`} className="block hover:opacity-80 transition-opacity">
            <div className="flex items-center gap-2 mb-2">
              <span className="bg-[#e11d48] text-white px-1.5 py-0.5 text-[10px] font-black">ERROR</span>
              <Mono className="text-xs text-black/70 font-semibold">{format(parseSqliteDate(errorEntry.timestamp), 'MMM d, HH:mm:ss')}</Mono>
              <Mono className="text-xs text-black/60">{errorEntry.duration_ms}ms</Mono>
            </div>
            <Code className="text-sm text-[#e11d48] font-bold block break-all">{errorCtx.error || errorEntry.message}</Code>
            {errorCtx.method && errorCtx.path && (
              <div className="flex items-center gap-2 mt-1.5 text-xs font-mono text-black/70">
                <span className="font-bold">{errorCtx.method}</span>
                <span>{errorCtx.status}</span>
                <span>{errorCtx.path}</span>
              </div>
            )}
            {errorCtx.job && (
              <div className="mt-1.5 text-xs font-mono text-black/70">
                Job: <span className="font-bold">{errorCtx.job}</span>
              </div>
            )}
          </a>
        </div>

        {/* Log entries after */}
        {after.length > 0 && (
          <>
            <div className="px-4 py-1.5 border-b border-black/10">
              <span className="text-[10px] font-mono font-black text-black/70 uppercase">After this error</span>
            </div>
            {after.map((e) => (
              <TraceRow key={e.id} entry={e} logPath={logPath} />
            ))}
          </>
        )}

        {/* No surrounding entries */}
        {before.length === 0 && after.length === 0 && (
          <div className="px-4 py-2.5 border-t border-black/10">
            <span className="text-xs font-mono text-black/60">No other log entries in this trace</span>
          </div>
        )}

        {/* View full trace */}
        <a
          href={`${logPath}?trace=${errorEntry.trace_id}`}
          className="flex items-center justify-between px-4 py-2.5 border-t-2 border-black/20 text-xs font-mono font-bold text-black/80 hover:text-black transition-colors"
        >
          <span>{traceEntries.length} {traceEntries.length === 1 ? 'entry' : 'entries'} in trace</span>
          <span>View full trace →</span>
        </a>
      </div>
    </div>
  )
}

function TraceRow({ entry, logPath }: { entry: LogEntry; logPath: string }) {
  const ctx = parseContext(entry.context)
  const config = getConfig(entry.is_error)
  const Icon = config.icon
  const isJob = !!ctx.job

  return (
    <a
      href={`${logPath}/${entry.id}`}
      className="flex items-center gap-3 px-4 py-2.5 hover:bg-black/[0.03] transition-colors border-b border-black/10 last:border-b-0"
    >
      <div className={`p-1 ${config.colors}`}>
        <Icon className="w-3 h-3" />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          {isJob ? (
            <span className="font-mono text-sm font-bold text-black">{ctx.job}</span>
          ) : (
            <>
              {ctx.method && (
                <span className={`font-mono text-sm font-bold ${methodColor(ctx.method)}`}>{ctx.method}</span>
              )}
              {ctx.status && (
                <span className={`font-mono text-sm font-bold ${statusColor(ctx.status)}`}>{ctx.status}</span>
              )}
              <Code className="text-sm text-black/80 truncate">{ctx.path || entry.message}</Code>
            </>
          )}
        </div>
      </div>
      <Mono className="text-xs text-black/60 shrink-0">{entry.duration_ms}ms</Mono>
      <Mono className="text-xs text-black/60 shrink-0 w-16 text-right">{format(parseSqliteDate(entry.timestamp), 'HH:mm:ss')}</Mono>
    </a>
  )
}

// --- Timeline Entry (occurrences) ---
function TimelineEntry({ entry, appId, isFirst }: { entry: LogEntry; appId: number; isFirst: boolean }) {
  const ctx = parseContext(entry.context)
  const isJob = !!ctx.job
  const logPath = `/apps/${appId}/log`

  return (
    <a href={`${logPath}/${entry.id}`} className="relative flex items-start gap-3 pb-5 last:pb-0 group block">
      <div className={`absolute left-[-17px] top-1.5 w-[9px] h-[9px] border-2 ${
        isFirst ? 'bg-[#e11d48] border-[#e11d48]' : 'bg-white border-black/40 group-hover:border-black/60'
      }`} />
      <div className="flex-1 min-w-0 py-0.5">
        <div className="flex items-center gap-2">
          {isJob ? (
            <span className="font-mono text-sm font-black text-black">{ctx.job}</span>
          ) : (
            <>
              {ctx.method && (
                <span className={`font-mono text-sm font-black ${methodColor(ctx.method)}`}>{ctx.method}</span>
              )}
              {ctx.status && (
                <span className={`font-mono text-sm font-black ${statusColor(ctx.status)}`}>{ctx.status}</span>
              )}
              <Code className="text-sm text-black/80 font-semibold truncate group-hover:text-black">{ctx.path || entry.message}</Code>
            </>
          )}
        </div>
        <div className="flex items-center gap-2 mt-0.5 text-xs text-black/60 font-mono">
          <span className="font-semibold">{format(parseSqliteDate(entry.timestamp), 'MMM d, HH:mm:ss')}</span>
          <span>·</span>
          <span>{entry.duration_ms}ms</span>
          {entry.trace_id && (
            <>
              <span>·</span>
              <span className="text-black/40">trace:{entry.trace_id.slice(0, 8)}</span>
            </>
          )}
        </div>
      </div>
    </a>
  )
}

// --- Frequency Heatmap ---
// Full width, right edge = today, fills backwards. Data placed at correct position relative to now.
function parseSqliteDate(s: string): Date {
  if (!s) return new Date()
  return new Date(s.replace(' ', 'T'))
}

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

function FrequencyHeatmap({ occurrences, firstSeen }: { occurrences: OccurrencePoint[] | null; firstSeen: string }) {
  const containerRef = React.useRef<HTMLDivElement>(null)
  const [numCols, setNumCols] = React.useState(60)

  // Measure container to fill full width
  React.useEffect(() => {
    const measure = () => {
      if (!containerRef.current) return
      const availableWidth = containerRef.current.clientWidth - LABEL_WIDTH - 32 // 32 = padding
      const colWidth = 13 // cell + gap
      setNumCols(Math.max(7, Math.floor(availableWidth / colWidth)))
    }
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [])

  const countMap = new Map<string, number>()
  if (occurrences) {
    for (const p of occurrences) {
      countMap.set(p.hour, p.count)
    }
  }

  const bucketCount = (dayStr: string, bucket: typeof TIME_BUCKETS[number]) => {
    let total = 0
    for (let h = bucket.start; h <= bucket.end; h++) {
      total += countMap.get(`${dayStr} ${String(h).padStart(2, '0')}`) || 0
    }
    return total
  }

  // Right edge = today. Always show numCols days. If data is older, scroll.
  const now = new Date()
  const end = now

  let dataStart = subDays(now, numCols - 1)
  if (firstSeen) {
    const d = parseSqliteDate(firstSeen)
    if (!isNaN(d.getTime()) && d < dataStart) {
      dataStart = d
    }
  }

  const totalDays = Math.max(numCols, differenceInDays(end, dataStart) + 1)
  const start = subDays(end, totalDays - 1)
  const days = eachDayOfInterval({ start, end })

  let maxBucket = 1
  for (const day of days) {
    const dayStr = format(day, 'yyyy-MM-dd')
    for (const bucket of TIME_BUCKETS) {
      const c = bucketCount(dayStr, bucket)
      if (c > maxBucket) maxBucket = c
    }
  }

  const needsScroll = days.length > numCols

  return (
    <div className="mb-8" ref={containerRef}>
      <h3 className="text-sm font-black text-black uppercase tracking-wider mb-4">
        Frequency
      </h3>
      <div className="border-2 border-black/10 p-4">
        <div className="flex">
          {/* Row labels */}
          <div className="shrink-0 pr-2" style={{ width: LABEL_WIDTH }}>
            {TIME_BUCKETS.map((bucket, i) => (
              <div
                key={bucket.label}
                className="text-[9px] font-mono text-black/70 font-bold text-right flex items-center justify-end"
                style={{ height: 13 }}
              >
                {i % 2 === 0 ? bucket.label : ''}
              </div>
            ))}
          </div>

          {/* Grid — uses CSS grid to fill available width */}
          <div
            className="overflow-x-auto flex-1"
            ref={(el) => {
              if (el && needsScroll) el.scrollLeft = el.scrollWidth
            }}
          >
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: `repeat(${days.length}, 1fr)`,
                gap: 2,
                minWidth: needsScroll ? days.length * 13 : undefined,
              }}
            >
              {/* Grid cells — row by row, 12 rows × N columns */}
              {TIME_BUCKETS.flatMap((bucket) =>
                days.map((day, i) => {
                  const dayStr = format(day, 'yyyy-MM-dd')
                  const count = bucketCount(dayStr, bucket)
                  const intensity = count > 0 ? Math.max(0.25, count / maxBucket) : 0
                  return (
                    <div
                      key={`${bucket.label}-${i}`}
                      style={{
                        aspectRatio: '1',
                        borderRadius: 2,
                        backgroundColor: count > 0 ? `rgba(225, 29, 72, ${intensity})` : 'rgba(0,0,0,0.06)',
                      }}
                      title={`${format(day, 'EEE, MMM d')} ${bucket.label} — ${count} error${count !== 1 ? 's' : ''}`}
                    />
                  )
                })
              )}
            </div>

            {/* Day labels — spaced every ~10 columns, positioned to not clip */}
            <div style={{ position: 'relative', height: 14, marginTop: 4, minWidth: needsScroll ? days.length * 13 : undefined }}>
              {days.map((day, i) => {
                // Show a label roughly every 10 columns
                const spacing = Math.max(10, Math.floor(days.length / 8))
                if (i !== 0 && i % spacing !== 0) return null
                // Position as percentage of total width
                const left = `${(i / days.length) * 100}%`
                return (
                  <span
                    key={i}
                    className="text-[9px] font-mono text-black/60 font-bold whitespace-nowrap absolute"
                    style={{ left }}
                  >
                    {format(day, 'MMM d')}
                  </span>
                )
              })}
            </div>
          </div>
        </div>

        {/* Legend */}
        <div className="flex items-center gap-1.5 mt-3 text-[10px] text-black/60 font-mono font-bold">
          <span>Less</span>
          {[0, 0.25, 0.5, 0.75, 1].map((opacity, i) => (
            <div
              key={i}
              style={{
                width: 11,
                height: 11,
                borderRadius: 2,
                backgroundColor: i === 0 ? 'rgba(0,0,0,0.06)' : `rgba(225,29,72,${opacity})`,
              }}
            />
          ))}
          <span>More</span>
        </div>
      </div>
    </div>
  )
}
