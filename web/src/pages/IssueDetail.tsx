import React from 'react'
import { formatDistanceToNow, format, eachDayOfInterval, subDays, differenceInDays } from 'date-fns'
import { router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { Mono, Code, parseContext, getConfig, statusColor } from '@/lib/shared'
import type { Application, IssueDetail as IssueDetailType, OccurrencePoint, LogEntry, RelatedLocation } from '@/lib/shared'

interface Props {
  issue: IssueDetailType
  occurrences: OccurrencePoint[] | null
  traces: Record<string, LogEntry[]> | null
  alsoHappensAt?: RelatedLocation[] | null
  appId: number
  appName: string
  applications?: Application[]
}

export default function IssueDetail({ issue, occurrences, traces, alsoHappensAt, appId, appName, applications }: Props) {
  const toggleMute = () => {
    const url = issue.muted ? `/issues/${issue.hash}/unmute` : `/issues/${issue.hash}/mute`
    router.post(url, {}, { preserveScroll: true })
  }

  const entries = issue.entries || []
  const recentEntries = entries.slice(0, 20)
  const logPath = `/apps/${appId}/log`

  // Extract structured error identity from issue or first entry context
  const firstCtx = recentEntries.length > 0 ? parseContext(recentEntries[0].context) : null
  const errorClass = issue.error_class || firstCtx?.error_class || firstCtx?.error_type
  const errorFile = issue.error_file || firstCtx?.error_file
  const errorLine = issue.error_line || firstCtx?.error_line
  const errorCaller = firstCtx?.error_caller
  const stackTrace = firstCtx?.stack_trace

  // Latest trace context: find the most recent entry with a trace
  const latestTraced = recentEntries.find((e) => e.trace_id && traces?.[e.trace_id])
  const latestTraceEntries = latestTraced && traces ? traces[latestTraced.trace_id!] : null

  return (
    <Layout currentPath="/issues" appId={appId} appName={appName} applications={applications}>
      {/* Header Card */}
      <div className="border border-foreground/10 overflow-hidden mb-8">
        <div className="p-6">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-3 mb-3">
                <span className="bg-[#e11d48] text-white px-2 py-0.5 text-xs font-black uppercase">Error</span>
                {issue.active && (
                  <span className="bg-[#e11d48]/10 text-[#e11d48] px-2 py-0.5 text-xs font-black">Active</span>
                )}
                {issue.muted && (
                  <span className="bg-foreground/10 text-foreground px-2 py-0.5 text-xs font-black">Muted</span>
                )}
              </div>
              <Code className="text-lg leading-snug block text-foreground font-bold break-all">{issue.error}</Code>
              {(errorClass || errorFile) && (
                <div className="mt-2 text-sm font-mono">
                  {errorClass && <span className="text-foreground font-semibold">{errorClass}</span>}
                  {errorFile && (
                    <span className="text-foreground/60">
                      {errorClass ? ' at ' : ''}{errorFile}{errorLine ? `:${errorLine}` : ''}
                    </span>
                  )}
                  {errorCaller && <span className="text-foreground/40"> in {errorCaller}</span>}
                </div>
              )}
              {alsoHappensAt && alsoHappensAt.length > 0 && (() => {
                const sameFile = alsoHappensAt.filter((l) => l.same_file)
                const diffFile = alsoHappensAt.filter((l) => !l.same_file)
                const parts: React.ReactNode[] = []

                if (sameFile.length > 0) {
                  parts.push(
                    <span key="sf">
                      also at line{sameFile.length > 1 ? 's' : ''}{' '}
                      {sameFile.slice(0, 2).map((loc, i) => (
                        <React.Fragment key={loc.hash}>
                          {i > 0 && ', '}
                          <a href={`/apps/${appId}/issues/${loc.hash}`} className="underline hover:text-foreground/60">
                            {loc.line}
                          </a>
                          {' '}({loc.count}x)
                        </React.Fragment>
                      ))}
                      {sameFile.length > 2 && <span> (+{sameFile.length - 2} more)</span>}
                    </span>
                  )
                }
                if (diffFile.length > 0) {
                  parts.push(
                    <span key="df">
                      {sameFile.length > 0 ? 'and in ' : 'same error in '}
                      {diffFile.slice(0, 2).map((loc, i) => (
                        <React.Fragment key={loc.hash}>
                          {i > 0 && ', '}
                          <a href={`/apps/${appId}/issues/${loc.hash}`} className="underline hover:text-foreground/60">
                            {loc.file}{loc.line > 0 ? `:${loc.line}` : ''}
                          </a>
                          {' '}({loc.count}x)
                        </React.Fragment>
                      ))}
                      {diffFile.length > 2 && <span> (+{diffFile.length - 2} more)</span>}
                    </span>
                  )
                }

                return (
                  <div className="mt-1 text-sm font-mono text-foreground/40">
                    {parts.map((p, i) => <React.Fragment key={i}>{i > 0 ? ', ' : ''}{p}</React.Fragment>)}
                  </div>
                )
              })()}
            </div>
            <button
              onClick={toggleMute}
              className="shrink-0 px-4 py-2 text-sm font-bold border border-foreground/10 hover:border-foreground/20 text-foreground/70 hover:text-foreground transition-colors"
            >
              {issue.muted ? 'Unmute' : 'Mute'}
            </button>
          </div>
        </div>

        <div className="grid grid-cols-3 border-t border-foreground/10 divide-x divide-foreground/10 bg-foreground/[0.02]">
          <div className="px-5 py-3">
            <div className="text-xs text-foreground/70 font-bold mb-1">Times</div>
            <Mono className="text-2xl font-black text-[#e11d48]">{issue.count}</Mono>
          </div>
          <div className="px-5 py-3">
            <div className="text-xs text-foreground/70 font-bold mb-1">First seen</div>
            <Mono className="text-sm text-foreground font-semibold">{formatDistanceToNow(new Date(issue.first_seen), { addSuffix: true })}</Mono>
          </div>
          <div className="px-5 py-3">
            <div className="text-xs text-foreground/70 font-bold mb-1">Last seen</div>
            <Mono className="text-sm text-foreground font-semibold">{formatDistanceToNow(new Date(issue.last_seen), { addSuffix: true })}</Mono>
          </div>
        </div>
      </div>

      {/* Stack Trace */}
      {stackTrace && (
        <details className="mb-8 border border-foreground/10">
          <summary className="px-4 py-3 text-sm font-black text-foreground uppercase tracking-wider cursor-pointer hover:bg-foreground/[0.02]">
            Stack Trace
          </summary>
          <pre className="px-4 py-3 text-xs font-mono text-foreground/80 overflow-x-auto whitespace-pre border-t border-foreground/10 bg-foreground/[0.02]">
            {stackTrace}
          </pre>
        </details>
      )}

      {/* Frequency Heatmap */}
      <FrequencyHeatmap occurrences={occurrences} firstSeen={issue.first_seen} />

      {/* In Context — surrounding log lines from the same trace (hidden when solo entry) */}
      {latestTraced && latestTraceEntries && latestTraceEntries.length > 1 && (
        <InContextSection
          errorEntry={latestTraced}
          traceEntries={latestTraceEntries}
          logPath={logPath}
        />
      )}

      {/* Recent Occurrences Timeline */}
      {recentEntries.length > 0 && (
        <div className="mb-8">
          <h3 className="text-sm font-black text-foreground uppercase tracking-wider mb-4">
            Last {recentEntries.length} times
          </h3>
          <div className="relative pl-6">
            <div className="absolute left-[7px] top-2 bottom-2 w-px bg-foreground/20" />
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
// Shows the request wrapper and child events from the same trace
function InContextSection({ errorEntry, traceEntries, logPath }: { errorEntry: LogEntry; traceEntries: LogEntry[]; logPath: string }) {
  const errorIdx = traceEntries.findIndex((e) => e.id === errorEntry.id)
  if (errorIdx < 0) return null

  const isRequest = (e: LogEntry) => {
    const c = parseContext(e.context)
    return c.method && c.status && c.path
  }

  const requestEntry = traceEntries.find(e => isRequest(e))
  const childEntries = requestEntry
    ? traceEntries.filter(e => e.id !== requestEntry.id)
    : traceEntries.filter(e => e.id !== errorEntry.id)

  const requestCtx = requestEntry ? parseContext(requestEntry.context) : null
  const requestConfig = requestEntry ? getConfig(requestEntry.is_error) : null

  return (
    <div className="mb-8">
      <h3 className="text-sm font-black text-foreground uppercase tracking-wider mb-4">
        Request Context
      </h3>
      <div className="border-2 border-foreground/10 overflow-hidden font-mono text-xs">
        {/* Request wrapper header */}
        {requestEntry && requestCtx && requestConfig && (() => {
          const ReqIcon = requestConfig.icon
          return (
            <a
              href={`${logPath}/${requestEntry.id}`}
              className="flex items-center gap-2 px-4 py-2.5 bg-foreground/[0.02] border-b border-foreground/10 hover:bg-foreground/[0.04] transition-colors"
            >
              <div className={`p-1 ${requestConfig.colors}`}>
                <ReqIcon className="w-3 h-3" />
              </div>
              {requestCtx.method && <span className={`font-bold text-foreground/50`}>{requestCtx.method}</span>}
              <span className="text-foreground/70">{requestCtx.path}</span>
              {requestCtx.status && <span className={`font-bold ${statusColor(requestCtx.status)}`}>{requestCtx.status}</span>}
              <span className="text-foreground/40 ml-auto shrink-0">{requestEntry.duration_ms}ms</span>
            </a>
          )
        })()}

        {/* Child events */}
        {childEntries.map((e, i) => {
          const isCurrent = e.id === errorEntry.id
          const eConfig = getConfig(e.is_error)
          const EIcon = eConfig.icon
          const eCtx = parseContext(e.context)
          const isLast = i === childEntries.length - 1
          const borderClass = !isLast ? 'border-b border-foreground/5' : ''
          const indent = requestEntry ? 'pl-8' : 'pl-4'

          const content = (
            <>
              <div className={`p-1 ${eConfig.colors}`}>
                <EIcon className="w-3 h-3" />
              </div>
              <span className={`truncate ${isCurrent ? 'text-foreground font-bold' : 'text-foreground/70'}`}>{eCtx.path || e.message}</span>
              {eCtx.error_file && <span className="text-foreground/40">{eCtx.error_file}{eCtx.error_line ? `:${eCtx.error_line}` : ''}</span>}
              {eCtx.method && <span className={`font-bold text-foreground/50`}>{eCtx.method}</span>}
              {eCtx.status && <span className={`font-bold ${statusColor(eCtx.status)}`}>{eCtx.status}</span>}
              {e.duration_ms > 0 && <span className="text-foreground/40 ml-auto shrink-0">{e.duration_ms}ms</span>}
            </>
          )

          if (isCurrent) {
            return (
              <div key={e.id} className={`relative flex items-center gap-2 ${indent} pr-4 py-2.5 bg-foreground/[0.04] ${borderClass}`}>
                <div className="absolute left-0 top-0 bottom-0 w-1 bg-foreground" />
                {content}
              </div>
            )
          }

          return (
            <a
              key={e.id}
              href={`${logPath}/${e.id}`}
              className={`flex items-center gap-2 ${indent} pr-4 py-2.5 hover:bg-foreground/[0.02] transition-colors ${borderClass}`}
            >
              {content}
            </a>
          )
        })}

        {/* View full trace */}
        {errorEntry.trace_id && (
          <a
            href={`${logPath}?trace=${errorEntry.trace_id}`}
            className="flex items-center justify-between px-4 py-2.5 border-t border-foreground/10 font-mono text-xs font-bold text-foreground/60 hover:text-foreground transition-colors"
          >
            <span>{traceEntries.length} {traceEntries.length === 1 ? 'entry' : 'entries'} in this trace</span>
            <span>View full trace →</span>
          </a>
        )}
      </div>
    </div>
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
        isFirst ? 'bg-[#e11d48] border-[#e11d48]' : 'bg-card border-foreground/40 group-hover:border-foreground/60'
      }`} />
      <div className="flex-1 min-w-0 py-0.5">
        <div className="flex items-center gap-2">
          {isJob ? (
            <span className="font-mono text-sm font-black text-foreground">{ctx.job}</span>
          ) : (
            <>
              {ctx.method && (
                <span className={`font-mono text-sm font-black text-foreground/50`}>{ctx.method}</span>
              )}
              {ctx.status && (
                <span className={`font-mono text-sm font-black ${statusColor(ctx.status)}`}>{ctx.status}</span>
              )}
              <Code className="text-sm text-foreground/80 font-semibold truncate group-hover:text-foreground">{ctx.path || entry.message}</Code>
            </>
          )}
        </div>
        <div className="flex items-center gap-2 mt-0.5 text-xs text-foreground/60 font-mono">
          <span className="font-semibold">{format(new Date(entry.timestamp), 'MMM d, HH:mm:ss')}</span>
          <span>·</span>
          <span>{entry.duration_ms}ms</span>
          {entry.trace_id && (
            <>
              <span>·</span>
              <span className="text-foreground/40">trace:{entry.trace_id.slice(0, 8)}</span>
            </>
          )}
        </div>
      </div>
    </a>
  )
}

// --- Frequency Heatmap ---
// Full width, right edge = today, fills backwards. Data placed at correct position relative to now.

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
  const [numCols, setNumCols] = React.useState(0) // Start at 0, measure before render
  const [hoveredCell, setHoveredCell] = React.useState<{ day: Date; bucket: typeof TIME_BUCKETS[number]; count: number } | null>(null)

  // Measure container to fill full width - useLayoutEffect to measure before paint
  React.useLayoutEffect(() => {
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

  // Server returns each bucket as RFC3339 UTC ("2026-04-14T11:00:00Z").
  // Convert to a local Date and key by local "YYYY-MM-DD HH" so the heatmap
  // axes (day columns, hour rows) reflect the viewer's timezone.
  const countMap = new Map<string, number>()
  if (occurrences) {
    for (const p of occurrences) {
      const d = new Date(p.hour)
      if (isNaN(d.getTime())) continue
      const key = `${format(d, 'yyyy-MM-dd')} ${String(d.getHours()).padStart(2, '0')}`
      countMap.set(key, (countMap.get(key) || 0) + p.count)
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
    const d = new Date(firstSeen)
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

  // Don't render grid until container is measured
  if (numCols === 0) {
    return (
      <div className="mb-8" ref={containerRef}>
        <h3 className="text-sm font-black text-foreground uppercase tracking-wider mb-4">
          Frequency
        </h3>
        <div className="border border-foreground/10 p-4 h-[200px]" />
      </div>
    )
  }

  return (
    <div className="mb-8" ref={containerRef}>
      <h3 className="text-sm font-black text-foreground uppercase tracking-wider mb-4">
        Frequency
      </h3>
      <div className="border border-foreground/10 p-4 relative">
        <div className="flex">
          {/* Row labels */}
          <div className="shrink-0 pr-2" style={{ width: LABEL_WIDTH }}>
            {TIME_BUCKETS.map((bucket, i) => (
              <div
                key={bucket.label}
                className="text-[9px] font-mono text-foreground/70 font-bold text-right flex items-center justify-end"
                style={{ height: 13 }}
              >
                {i % 2 === 0 ? bucket.label : ''}
              </div>
            ))}
          </div>

          {/* Grid — uses CSS grid to fill available width */}
          <div
            className="overflow-x-auto flex-1 scrollbar-hide"
            style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
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
                  const intensity = count > 0 ? Math.max(0.4, count / maxBucket) : 0
                  const isHovered = hoveredCell?.day === day && hoveredCell?.bucket === bucket
                  return (
                    <div
                      key={`${bucket.label}-${i}`}
                      className={`cursor-crosshair transition-opacity ${hoveredCell && !isHovered ? 'opacity-40' : ''}`}
                      style={{
                        aspectRatio: '1',
                        borderRadius: 2,
                        backgroundColor: count > 0 ? `rgba(225, 29, 72, ${intensity})` : 'rgba(255,255,255,0.06)',
                      }}
                      onMouseEnter={() => setHoveredCell({ day, bucket, count })}
                      onMouseLeave={() => setHoveredCell(null)}
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
                    className="text-[9px] font-mono text-foreground/60 font-bold whitespace-nowrap absolute"
                    style={{ left }}
                  >
                    {format(day, 'MMM d')}
                  </span>
                )
              })}
            </div>
          </div>
        </div>

        {/* Hover tooltip - floating */}
        {hoveredCell && (
          <div className="absolute bottom-2 left-0 right-0 text-xs text-center pointer-events-none">
            <span className="bg-foreground text-background px-2 py-1">
              {format(hoveredCell.day, 'EEE, MMM d')} {hoveredCell.bucket.label}
              <span className="mx-2">·</span>
              <span className="font-mono font-semibold">{hoveredCell.count} error{hoveredCell.count !== 1 ? 's' : ''}</span>
            </span>
          </div>
        )}

        {/* Legend */}
        <div className="flex items-center gap-1.5 mt-3 text-[10px] text-foreground/60 font-mono font-bold">
          <span>Less</span>
          {[0, 0.25, 0.5, 0.75, 1].map((opacity, i) => (
            <div
              key={i}
              style={{
                width: 11,
                height: 11,
                borderRadius: 2,
                backgroundColor: i === 0 ? 'rgba(255,255,255,0.06)' : `rgba(225,29,72,${opacity})`,
              }}
            />
          ))}
          <span>More</span>
        </div>
      </div>
    </div>
  )
}
