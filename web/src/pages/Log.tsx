import { useState, useEffect, useRef, ReactNode } from 'react'
import { router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { statusColor, parseContext, PageHeader, Mono } from '@/lib/shared'
import type { Application, LogEntry } from '@/lib/shared'
import { pageKey } from '@/lib/keys'
import { ReleaseMarks, type Release } from '@/components/release-marks'

// Format as "Feb 25 20:15:07.152"
const formatTime = (date: Date) => {
  const mon = date.toLocaleDateString('en-US', { month: 'short' })
  const day = date.getDate()
  const h = String(date.getHours()).padStart(2, '0')
  const m = String(date.getMinutes()).padStart(2, '0')
  const s = String(date.getSeconds()).padStart(2, '0')
  const ms = String(date.getMilliseconds()).padStart(3, '0')
  return `${mon} ${day} ${h}:${m}:${s}.${ms}`
}

// Known LogContext keys — anything else is user context worth showing
const KNOWN_CONTEXT_KEYS = new Set([
  'method', 'path', 'status', 'job', 'error', 'user', 'ip', 'user_agent',
  'headers', 'body', 'error_type', 'error_location', 'error_class',
  'error_caller', 'error_file', 'error_line', 'stack_trace', 'extra',
])

const getExtraContext = (entry: LogEntry): [string, any][] => {
  if (!entry.context) return []
  try {
    const parsed = JSON.parse(entry.context)
    if (!parsed || typeof parsed !== 'object') return []
    // Collect from explicit extra field
    const pairs: [string, any][] = []
    if (parsed.extra && typeof parsed.extra === 'object') {
      pairs.push(...Object.entries(parsed.extra))
    }
    // Collect unknown top-level keys
    for (const [k, v] of Object.entries(parsed)) {
      if (!KNOWN_CONTEXT_KEYS.has(k) && typeof v !== 'object') {
        pairs.push([k, v])
      }
    }
    return pairs
  } catch {
    return []
  }
}

interface Filters {
  is_error?: boolean
  search?: string
  error?: string // error hash filter
  trace?: string // trace ID filter
  start_time?: string
  end_time?: string
}

interface HistogramBucket {
  timestamp: string
  total: number
  errors: number
}

interface Pagination {
  limit: number
  offset: number
  total: number
}

interface Props {
  entries?: LogEntry[]
  filters?: Filters
  pagination?: Pagination
  histogram?: HistogramBucket[]
  releases?: Release[]
  appId?: number
  appName?: string
  applications?: Application[]
}

interface LogGroup {
  traceId: string
  entries: LogEntry[]
  mainEntry: LogEntry
  firstTimestamp: Date
}

const groupByTraceId = (entries: LogEntry[]): LogGroup[] => {
  const groupMap = new Map<string, LogEntry[]>()

  for (const entry of entries) {
    const key = entry.trace_id || `single-${entry.id}`
    if (groupMap.has(key)) {
      groupMap.get(key)!.push(entry)
    } else {
      groupMap.set(key, [entry])
    }
  }

  return Array.from(groupMap.entries())
    .map(([traceId, groupEntries]) => {
      const sorted = groupEntries.sort((a, b) => a.timestamp < b.timestamp ? -1 : a.timestamp > b.timestamp ? 1 : 0)
      return {
        traceId,
        entries: sorted,
        mainEntry: sorted[sorted.length - 1],
        firstTimestamp: new Date(sorted[0].timestamp),
      }
    })
    .sort((a, b) => b.firstTimestamp.getTime() - a.firstTimestamp.getTime())
}

// Columns: time, method, path or message, status, duration.
// Widths in px, not ch: the header row is 11px and the rows 13px, and ch would
// size the same column differently in each.
const ROW = 'grid grid-cols-[148px_40px_1fr_40px_64px] items-center gap-3.5 max-md:grid-cols-[40px_1fr_32px_56px]'

function LogRow({ entry, appId, child, inGroup }: { entry: LogEntry; appId?: number; child?: boolean; inGroup?: boolean }) {
  const ctx = parseContext(entry.context)
  const isRequest = !!(ctx.method && ctx.path)
  const extraPairs = getExtraContext(entry)

  return (
    <a
      href={`/apps/${appId}/log/${entry.id}`}
      className={`${ROW} whitespace-nowrap px-[18px] py-1.5 text-[13px] hover:bg-raise ${entry.is_error && !inGroup ? 'shadow-[inset_3px_0_0_0_hsl(var(--error))]' : ''}`}
    >
      <span className="tabular-nums text-dim max-md:hidden">{formatTime(new Date(entry.timestamp)).toLowerCase()}</span>
      <span className="text-dim">
        {ctx.method || (ctx.job ? <span className="border border-border px-1 text-[11px] text-subtle">JOB</span> : '')}
      </span>
      <span className={`relative flex min-w-0 items-center gap-3 ${child ? 'pl-[3ch]' : ''}`}>
        {child && <i className="absolute left-[1ch] top-[-0.8em] bottom-1/2 w-[1.2ch] border-b border-l border-border" />}
        <span
          className={`truncate ${
            entry.is_error ? 'font-bold text-error' : isRequest ? 'text-foreground' : child ? 'text-subtle' : 'text-text'
          }`}
        >
          {isRequest ? ctx.path : entry.message || ctx.job || '—'}
        </span>
        {entry.is_error && ctx.error && isRequest && <span className="truncate text-xs text-error">{ctx.error}</span>}
        {ctx.error_file && (
          <span className="truncate text-xs text-dim">
            {ctx.error_file}
            {ctx.error_line ? `:${ctx.error_line}` : ''}
          </span>
        )}
        {extraPairs.length > 0 && (
          <span className="truncate text-xs text-dim">{extraPairs.slice(0, 3).map(([k, v]) => `${k}=${String(v)}`).join('  ')}</span>
        )}
      </span>
      <span className={`text-right ${ctx.status ? statusColor(ctx.status) : ''}`}>{ctx.status || ''}</span>
      <span className="text-right tabular-nums text-subtle">{entry.duration_ms > 0 ? `${entry.duration_ms}ms` : ''}</span>
    </a>
  )
}

function GroupedLog({ group, appId }: { group: LogGroup; appId?: number }) {
  const { entries } = group
  const hasError = entries.some((e) => e.is_error)
  // The request (method + path) leads; the rest nest under it.
  const requestIdx =
    entries.length > 1
      ? entries.findIndex((e) => {
          const c = parseContext(e.context)
          return c.method && c.path
        })
      : -1
  const ordered = requestIdx >= 0 ? [entries[requestIdx], ...entries.filter((_, i) => i !== requestIdx)] : entries

  return (
    <div className={`border-t border-line first:border-t-0 ${hasError ? 'shadow-[inset_3px_0_0_0_hsl(var(--error))]' : ''}`}>
      {ordered.map((entry, i) => (
        <LogRow key={entry.id} entry={entry} appId={appId} inGroup child={requestIdx >= 0 && i > 0} />
      ))}
    </div>
  )
}

function LogHistogram({ buckets: rawBuckets, releases }: { buckets: HistogramBucket[]; releases?: Release[] }) {
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null)

  if (!rawBuckets || rawBuckets.length === 0) return null

  // Trim trailing empty buckets (future time periods with no data)
  let lastNonEmpty = rawBuckets.length - 1
  while (lastNonEmpty >= 0 && rawBuckets[lastNonEmpty].total === 0) {
    lastNonEmpty--
  }
  const buckets = lastNonEmpty >= 0 ? rawBuckets.slice(0, lastNonEmpty + 1) : rawBuckets

  if (buckets.length === 0) return null

  // Hide chart when less than 10% of buckets have data — mostly empty charts waste space
  const nonEmptyCount = buckets.filter(b => b.total > 0).length
  if (nonEmptyCount < buckets.length * 0.1) return null

  const maxTotal = Math.max(...buckets.map(b => b.total), 1)
  const hoveredBucket = hoveredIdx !== null ? buckets[hoveredIdx] : null
  const from = new Date(buckets[0].timestamp).getTime()
  const bucketMs = rawBuckets.length > 1 ? new Date(rawBuckets[1].timestamp).getTime() - from : 3_600_000
  const to = new Date(buckets[buckets.length - 1].timestamp).getTime() + bucketMs

  return (
    <div className="pane relative mb-7 px-[18px] pb-3 pt-6">
      <span className="pane-title">requests · errors in red{releases?.length ? ' · deploys dashed' : ''}</span>
      <div className="relative flex h-16 items-end gap-0.5">
        <ReleaseMarks releases={releases} from={from} to={to} />
        {buckets.map((bucket, i) => {
          const successHeight = ((bucket.total - bucket.errors) / maxTotal) * 100
          const errorHeight = (bucket.errors / maxTotal) * 100
          const hasErrors = bucket.errors > 0
          const isHovered = hoveredIdx === i

          return (
            <div
              key={i}
              className={`flex h-full flex-1 cursor-crosshair flex-col justify-end ${
                hoveredIdx !== null && !isHovered ? 'opacity-40' : ''
              }`}
              onMouseEnter={() => setHoveredIdx(i)}
              onMouseLeave={() => setHoveredIdx(null)}
            >
              {hasErrors && (
                <div
                  className="w-full bg-error"
                  style={{ height: `${errorHeight}%`, minHeight: bucket.errors > 0 ? '2px' : 0 }}
                />
              )}
              <div
                className="w-full bg-border"
                style={{ height: `${successHeight}%`, minHeight: bucket.total - bucket.errors > 0 ? '1px' : 0 }}
              />
            </div>
          )
        })}
      </div>

      <p className="mt-2 h-4 text-center text-xs text-subtle">
      {hoveredBucket && (
          <span>
            {new Date(hoveredBucket.timestamp).toLocaleString(undefined, {
              month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
            })}
            {' '}· <span className="text-foreground">{hoveredBucket.total} requests</span>
            {hoveredBucket.errors > 0 && (
              <>
                {' '}· <span className="text-error">{hoveredBucket.errors} errors</span>
              </>
            )}
          </span>
      )}
      </p>
    </div>
  )
}

type DatePreset = 'all' | 'today' | '7d' | '30d'

const getDatePresetFromFilters = (filters: Filters): DatePreset => {
  if (filters.start_time === 'all') return 'all'
  if (!filters.start_time) return '7d' // Server default is 7 days
  const start = new Date(filters.start_time)
  const now = new Date()
  const diffDays = Math.round((now.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))
  if (diffDays <= 1) return 'today'
  if (diffDays <= 7) return '7d'
  if (diffDays <= 30) return '30d'
  return 'all'
}

const getStartTimeForPreset = (preset: DatePreset): string | undefined => {
  if (preset === 'all') return 'all'
  const now = new Date()
  if (preset === 'today') {
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    return start.toISOString()
  }
  if (preset === '7d') {
    const start = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
    return start.toISOString()
  }
  if (preset === '30d') {
    const start = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)
    return start.toISOString()
  }
  return undefined
}

export default function Log({ entries: initialEntries = [], filters = {}, pagination = { limit: 50, offset: 0, total: 0 }, histogram = [], releases = [], appId, appName, applications }: Props) {
  const [entries, setEntries] = useState(initialEntries)
  const [search, setSearch] = useState(filters.search || '')
  const [errorFilter, setErrorFilter] = useState<string>(
    filters.is_error === true ? 'errors' : filters.is_error === false ? 'success' : 'all'
  )
  const [datePreset, setDatePreset] = useState<DatePreset>(() => getDatePresetFromFilters(filters))
  const [groupEnabled, setGroupEnabled] = useState(true)
  const [liveEnabled, setLiveEnabled] = useState(false)

  const basePath = `/apps/${appId}/log`
  const logGroups = groupEnabled && !filters.trace && !filters.search ? groupByTraceId(entries) : []

  // Live polling - fetch latest and merge with existing
  useEffect(() => {
    if (!liveEnabled || !appId) return

    const poll = async () => {
      try {
        const res = await fetch(`/api/v1/events?app_id=${appId}&limit=50`, {
          credentials: 'same-origin'
        })
        if (!res.ok) return
        const data = await res.json()
        const fetched = (data.events || []) as LogEntry[]

        if (fetched.length === 0) return

        setEntries(prev => {
          const existingIds = new Set(prev.map(e => e.id))
          const newOnes = fetched.filter(e => !existingIds.has(e.id))
          if (newOnes.length === 0) return prev
          return [...newOnes, ...prev].slice(0, 100)
        })
      } catch {
        // ignore
      }
    }

    poll()
    const interval = setInterval(poll, 2000)
    return () => clearInterval(interval)
  }, [liveEnabled, appId])

  // Reset entries when initialEntries change (navigation)
  useEffect(() => {
    setEntries(initialEntries)
  }, [initialEntries])

  const toggleGrouping = (checked: boolean) => {
    setGroupEnabled(checked)
  }

  const toggleLive = (checked: boolean) => {
    setLiveEnabled(checked)
    if (checked) {
      setGroupEnabled(false)
    }
  }

  const applyFilters = (newFilters?: { errorFilter?: string; datePreset?: DatePreset }) => {
    const params = new URLSearchParams()
    const filterValue = newFilters?.errorFilter ?? errorFilter
    const dateValue = newFilters?.datePreset ?? datePreset
    if (search) params.set('search', search)
    if (filterValue === 'errors') params.set('is_error', 'true')
    if (filterValue === 'success') params.set('is_error', 'false')
    const startTime = getStartTimeForPreset(dateValue)
    if (startTime) params.set('start_time', startTime)
    router.visit(`${basePath}?${params.toString()}`)
  }

  const handleErrorFilter = (value: string) => {
    if (!value) return
    setErrorFilter(value)
    applyFilters({ errorFilter: value })
  }

  const handleDateFilter = (preset: DatePreset) => {
    setDatePreset(preset)
    applyFilters({ datePreset: preset })
  }

  const clearFilters = () => {
    setSearch('')
    setErrorFilter('all')
    setDatePreset('all')
    router.visit(basePath)
  }

  const errorHashFilter = filters.error || ''
  const traceFilter = filters.trace || ''
  const hasFilters = search || errorFilter !== 'all' || datePreset !== 'all' || errorHashFilter || traceFilter
  const totalPages = Math.ceil(pagination.total / pagination.limit)
  const currentPage = Math.floor(pagination.offset / pagination.limit) + 1
  const grouped = groupEnabled && !filters.trace && !filters.search

  // "/" jumps to the search field.
  const searchRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (pageKey(e) && e.key === '/') {
        e.preventDefault()
        searchRef.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const goToOffset = (offset: number) => {
    const params = new URLSearchParams(window.location.search)
    params.set('offset', String(offset))
    router.visit(`${basePath}?${params.toString()}`)
  }

  return (
    <Layout currentPath="/log" appId={appId} appName={appName} applications={applications}>
      <PageHeader
        title={
          <>
            Logs
            {traceFilter && (
              <span className="ml-3 inline-flex items-center gap-2 border border-border px-2 py-0.5 align-middle text-xs font-normal text-foreground">
                trace:{traceFilter.slice(0, 8)}
                <button onClick={() => router.visit(basePath)} className="text-dim hover:text-foreground" aria-label="Clear trace">
                  ×
                </button>
              </span>
            )}
          </>
        }
        subtitle={
          liveEnabled ? (
            'Watching for requests'
          ) : (
            <>
              <Mono>{pagination.total.toLocaleString()}</Mono> request{pagination.total !== 1 ? 's' : ''}
            </>
          )
        }
      >
        {appId && (
          <button onClick={() => toggleLive(!liveEnabled)} className={`key ${liveEnabled ? 'border-foreground' : ''}`}>
            <i className={`h-[7px] w-[7px] bg-accent ${liveEnabled ? 'animate-pulse' : ''}`} />
            {liveEnabled ? 'stop' : 'live'}
          </button>
        )}
      </PageHeader>

      <div className={`mb-7 flex flex-wrap items-center gap-x-6 gap-y-3 border border-border px-3.5 py-2.5 ${liveEnabled ? 'opacity-50' : ''}`}>
        <label className="flex min-w-[220px] flex-1 items-center gap-2">
          <span className="text-dim">/</span>
          <input
            ref={searchRef}
            type="text"
            placeholder="search messages, paths, trace ids"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') applyFilters()
              if (e.key === 'Escape') (e.target as HTMLInputElement).blur()
            }}
            className="flex-1 border-0 bg-transparent text-foreground outline-none placeholder:text-dim"
            disabled={liveEnabled}
          />
        </label>
        <Segment
          options={[['all', 'All'], ['success', 'Success'], ['errors', 'Errors']]}
          value={errorFilter}
          onChange={handleErrorFilter}
          disabled={liveEnabled}
        />
        <Segment
          options={[['today', 'Today'], ['7d', '7d'], ['30d', '30d'], ['all', 'All']]}
          value={datePreset}
          onChange={(v) => handleDateFilter(v as DatePreset)}
          disabled={liveEnabled}
        />
        {!filters.trace && !filters.search && (
          <Segment options={[['on', 'Traces']]} value={groupEnabled ? 'on' : ''} onChange={() => toggleGrouping(!groupEnabled)} disabled={liveEnabled} />
        )}
        {hasFilters && (
          <button onClick={clearFilters} disabled={liveEnabled} className="text-[13px] text-subtle hover:text-foreground">
            clear
          </button>
        )}
      </div>

      {!liveEnabled && histogram.length > 0 && <LogHistogram buckets={histogram} releases={releases} />}

      {entries.length === 0 ? (
        <div className="border border-dashed border-border py-16 text-center">
          <p className="mb-1 font-bold text-foreground">{hasFilters ? 'Nothing here' : 'Quiet so far'}</p>
          <p className="text-sm text-subtle">{hasFilters ? 'Try different filters.' : 'Logs appear once your app is connected.'}</p>
          {hasFilters && (
            <button onClick={clearFilters} className="mt-4 text-sm text-foreground underline">
              clear filters
            </button>
          )}
        </div>
      ) : (
        <div className="pane pb-1.5 pt-3.5">
          <span className="pane-title">{liveEnabled ? 'tail -f' : 'tail'}</span>
          <div className="overflow-x-auto">
            <div className="min-w-[720px] max-md:min-w-0">
              <div className={`${ROW} border-b border-line px-[18px] pb-2 text-[11px] text-dim`}>
                <span className="max-md:hidden">time</span>
                <span />
                <span>path / message</span>
                <span className="text-right">code</span>
                <span className="text-right">took</span>
              </div>
              {grouped
                ? logGroups.map((group) => <GroupedLog key={group.traceId} group={group} appId={appId} />)
                : (filters.trace ? [...entries].sort((a, b) => (a.timestamp < b.timestamp ? -1 : a.timestamp > b.timestamp ? 1 : 0)) : entries)
                    .filter((e) => e && e.id)
                    .map((entry) => (
                      <div key={entry.id} className="border-t border-line first:border-t-0">
                        <LogRow entry={entry} appId={appId} />
                      </div>
                    ))}
            </div>
          </div>
        </div>
      )}

      {!liveEnabled && pagination.total > pagination.limit && (
        <div className="mt-5 flex items-center justify-between text-[13px] text-subtle">
          <p>
            <Mono>
              {pagination.offset + 1}–{Math.min(pagination.offset + pagination.limit, pagination.total)}
            </Mono>{' '}
            of <Mono>{pagination.total.toLocaleString()}</Mono>
          </p>
          <div className="flex items-center gap-4">
            <button
              onClick={() => goToOffset(Math.max(0, pagination.offset - pagination.limit))}
              disabled={pagination.offset === 0}
              className="hover:text-foreground disabled:opacity-40"
            >
              ← prev
            </button>
            <Mono>
              {currentPage} / {totalPages}
            </Mono>
            <button
              onClick={() => goToOffset(pagination.offset + pagination.limit)}
              disabled={pagination.offset + pagination.limit >= pagination.total}
              className="hover:text-foreground disabled:opacity-40"
            >
              next →
            </button>
          </div>
        </div>
      )}
    </Layout>
  )
}

// Terminal-style toggle group: the active option wears lime brackets.
function Segment({
  options,
  value,
  onChange,
  disabled,
}: {
  options: [string, ReactNode][]
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}) {
  return (
    <div className="flex gap-0.5 text-[13px]">
      {options.map(([v, label]) => (
        <button
          key={v}
          onClick={() => onChange(v)}
          disabled={disabled}
          aria-pressed={v === value}
          className={`px-1.5 py-0.5 ${v === value ? 'font-bold text-foreground' : 'text-subtle hover:text-foreground'}`}
        >
          {v === value && <span className="text-accent">[</span>}
          {label}
          {v === value && <span className="text-accent">]</span>}
        </button>
      ))}
    </div>
  )
}
