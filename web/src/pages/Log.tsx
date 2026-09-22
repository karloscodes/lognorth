import { useState, useEffect } from 'react'
import { router } from '@inertiajs/react'
import { Search, Inbox, Layers } from 'lucide-react'
import { Layout } from '@/components/layout'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { getConfig, statusColor, methodColor, parseContext, PageHeader, Mono } from '@/lib/shared'
import type { Application, LogEntry } from '@/lib/shared'

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

function LogRow({ entry, appId, hideErrorBorder, treeLine }: { entry: LogEntry; appId?: number; hideErrorBorder?: boolean; treeLine?: 'parent' | 'first-child' | 'child' | 'last-child' | 'only-child' }) {
  const href = `/apps/${appId}/log/${entry.id}`
  const ctx = parseContext(entry.context)
  const isRequest = !!(ctx.method && ctx.path)
  const isError = entry.is_error
  const showErrorBorder = isError && !hideErrorBorder
  const isChild = treeLine === 'first-child' || treeLine === 'child' || treeLine === 'last-child' || treeLine === 'only-child'
  const extraPairs = getExtraContext(entry)
  // GET badge center x ≈ 28px, dot x = 50px (right of badge, shows hierarchy)
  const badgeCenterX = 28
  const dotX = 50

  return (
    <a
      href={href}
      className={`relative flex items-center gap-3 py-3 px-5 overflow-hidden transition-colors hover:bg-foreground/[0.02] ${showErrorBorder ? 'shadow-[inset_3px_0_0_0_#e11d48]' : ''}`}
    >
      {/* Timestamp */}
      <span className="text-xs text-foreground/40 tabular-nums shrink-0 font-mono" style={{ width: '145px' }}>
        {formatTime(new Date(entry.timestamp))}
      </span>

      {/* Type badge + Tree connector */}
      <div className="w-14 shrink-0 relative flex items-center justify-center" style={{ height: '20px' }}>
        {treeLine === 'parent' && (
          /* Vertical line down from GET badge center */
          <span className="absolute w-px bg-foreground/15" style={{ left: badgeCenterX, top: '50%', bottom: '-16px' }} />
        )}
        {treeLine === 'first-child' && (
          <>
            {/* L-shape: vertical from parent down, then horizontal to dot */}
            <span className="absolute w-px bg-foreground/15" style={{ left: badgeCenterX, top: '-16px', bottom: '50%' }} />
            <span className="absolute h-px bg-foreground/15" style={{ left: badgeCenterX, top: '50%', width: dotX - badgeCenterX }} />
            {/* Vertical line continues down to next children */}
            <span className="absolute w-px bg-foreground/15" style={{ left: dotX, top: '50%', bottom: '-16px' }} />
            {/* Dot */}
            <span className="absolute w-[5px] h-[5px] rounded-full bg-foreground/25" style={{ left: dotX - 2, top: 'calc(50% - 2.5px)' }} />
          </>
        )}
        {treeLine === 'only-child' && (
          <>
            {/* L-shape only, no line continuing down */}
            <span className="absolute w-px bg-foreground/15" style={{ left: badgeCenterX, top: '-16px', bottom: '50%' }} />
            <span className="absolute h-px bg-foreground/15" style={{ left: badgeCenterX, top: '50%', width: dotX - badgeCenterX }} />
            {/* Dot */}
            <span className="absolute w-[5px] h-[5px] rounded-full bg-foreground/25" style={{ left: dotX - 2, top: 'calc(50% - 2.5px)' }} />
          </>
        )}
        {treeLine === 'child' && (
          <>
            {/* Vertical line through dot position */}
            <span className="absolute w-px bg-foreground/15" style={{ left: dotX, top: '-16px', bottom: '-16px' }} />
            {/* Dot */}
            <span className="absolute w-[5px] h-[5px] rounded-full bg-foreground/25" style={{ left: dotX - 2, top: 'calc(50% - 2.5px)' }} />
          </>
        )}
        {treeLine === 'last-child' && (
          <>
            {/* Vertical line from top to dot center */}
            <span className="absolute w-px bg-foreground/15" style={{ left: dotX, top: '-16px', bottom: '50%' }} />
            {/* Dot */}
            <span className="absolute w-[5px] h-[5px] rounded-full bg-foreground/25" style={{ left: dotX - 2, top: 'calc(50% - 2.5px)' }} />
          </>
        )}
        {ctx.method && (
          <span className="relative font-mono text-xs font-semibold px-1.5 py-0.5 text-foreground/50 bg-foreground/[0.04]">
            {ctx.method}
          </span>
        )}
        {ctx.job && !ctx.method && (
          <span className="relative font-mono text-xs font-semibold px-1.5 py-0.5 text-foreground/50 bg-foreground/[0.04]">
            JOB
          </span>
        )}
      </div>

      {/* Path / Message + inline metadata — entire section truncates */}
      <div className="flex-1 min-w-0 flex items-center gap-3 overflow-hidden">
        {isRequest ? (
          <span className="font-mono text-sm text-foreground font-semibold truncate shrink-0" style={{ maxWidth: '40%' }}>
            {ctx.path}
          </span>
        ) : (
          <span className={`font-mono text-sm truncate ${isError ? 'text-[#e11d48] font-semibold' : 'text-foreground/60'}`} style={{ maxWidth: '50%' }}>
            {entry.message || ctx.job || '—'}
          </span>
        )}
        {isError && ctx.error && isRequest && (
          <span className="font-mono text-xs text-[#e11d48] truncate min-w-0">
            {ctx.error}
          </span>
        )}
        {ctx.error_file && (
          <span className="font-mono text-xs text-foreground/30 truncate min-w-0">
            {ctx.error_file}{ctx.error_line ? `:${ctx.error_line}` : ''}
          </span>
        )}
        {extraPairs.length > 0 && (
          <span className="font-mono text-xs text-foreground/25 truncate min-w-0">
            {extraPairs.slice(0, 3).map(([k, v]) => `${k}=${String(v)}`).join('  ')}
          </span>
        )}
      </div>

      {/* Status + Duration */}
      <div className="flex items-center gap-3 shrink-0">
        {ctx.status ? (
          <span className={`font-mono text-sm font-bold w-10 text-right ${
            ctx.status >= 500 ? 'text-[#e11d48]' :
            ctx.status >= 400 ? 'text-foreground' :
            'text-foreground/50'
          }`}>
            {ctx.status}
          </span>
        ) : (
          <span className="w-10" />
        )}
        {entry.duration_ms > 0 ? (
          <span className="font-mono text-xs text-foreground/50 tabular-nums w-14 text-right">
            {entry.duration_ms}ms
          </span>
        ) : (
          <span className="w-14" />
        )}
      </div>
    </a>
  )
}

function GroupedLog({ group, appId }: { group: LogGroup; appId?: number }) {
  const { entries } = group
  const hasError = entries.some(e => e.is_error)
  const isGroup = entries.length > 1

  // Find the request entry (has method+path) — it's the parent
  const requestIdx = isGroup
    ? entries.findIndex(e => {
        const c = parseContext(e.context)
        return c.method && c.path
      })
    : -1

  // Find last child index
  const childIndices = entries.map((_, i) => i).filter(i => i !== requestIdx)
  const lastChildIdx = childIndices.length > 0 ? childIndices[childIndices.length - 1] : -1

  const firstChildIdx = childIndices.length > 0 ? childIndices[0] : -1

  const getTreeLine = (i: number): 'parent' | 'first-child' | 'child' | 'last-child' | 'only-child' | undefined => {
    if (!isGroup) return undefined
    if (i === requestIdx) return childIndices.length > 0 ? 'parent' : undefined
    if (childIndices.length === 1) return 'only-child'
    if (i === firstChildIdx) return 'first-child'
    if (i === lastChildIdx) return 'last-child'
    return 'child'
  }

  return (
    <div className={`border-b border-foreground/10 last:border-b-0 ${hasError ? 'shadow-[inset_3px_0_0_0_#e11d48]' : ''}`}>
      {entries.map((entry, i) => (
        <LogRow
          key={entry.id}
          entry={entry}
          appId={appId}
          hideErrorBorder
          treeLine={getTreeLine(i)}
        />
      ))}
    </div>
  )
}

function LogHistogram({ buckets: rawBuckets }: { buckets: HistogramBucket[] }) {
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

  return (
    <div className="mb-4 bg-card border border-foreground/10 p-3 relative">
      {/* Bars with gaps */}
      <div className="h-16 flex items-end gap-0.5">
        {buckets.map((bucket, i) => {
          const successHeight = ((bucket.total - bucket.errors) / maxTotal) * 100
          const errorHeight = (bucket.errors / maxTotal) * 100
          const hasErrors = bucket.errors > 0
          const isHovered = hoveredIdx === i

          return (
            <div
              key={i}
              className={`flex-1 flex flex-col justify-end h-full cursor-crosshair transition-opacity ${
                hoveredIdx !== null && !isHovered ? 'opacity-40' : ''
              }`}
              onMouseEnter={() => setHoveredIdx(i)}
              onMouseLeave={() => setHoveredIdx(null)}
            >
              {hasErrors && (
                <div
                  className="w-full bg-[#e11d48]"
                  style={{ height: `${errorHeight}%`, minHeight: bucket.errors > 0 ? '2px' : 0 }}
                />
              )}
              <div
                className="w-full bg-accent"
                style={{ height: `${successHeight}%`, minHeight: bucket.total - bucket.errors > 0 ? '1px' : 0 }}
              />
            </div>
          )
        })}
      </div>

      {/* Tooltip overlay - black background */}
      {hoveredBucket && (
        <div className="absolute bottom-1 left-0 right-0 text-xs text-center pointer-events-none">
          <span className="bg-foreground text-background px-2 py-1">
            {new Date(hoveredBucket.timestamp).toLocaleString(undefined, {
              month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
            })}
            <span className="mx-2">·</span>
            <span className="font-mono font-semibold">{hoveredBucket.total} requests</span>
            {hoveredBucket.errors > 0 && (
              <>
                <span className="mx-2">·</span>
                <span className="font-mono font-semibold text-[#f87171]">{hoveredBucket.errors} errors</span>
              </>
            )}
          </span>
        </div>
      )}
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

export default function Log({ entries: initialEntries = [], filters = {}, pagination = { limit: 50, offset: 0, total: 0 }, histogram = [], appId, appName, applications }: Props) {
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

  return (
    <Layout currentPath="/log" appId={appId} appName={appName} applications={applications}>
      <PageHeader
        title={
          <>
            Logs
            {liveEnabled && (
              <span className="ml-3 inline-flex items-center gap-1.5 px-2 py-1 bg-[#e11d48] text-white text-xs font-semibold align-middle">
                <span className="w-1.5 h-1.5 bg-card animate-pulse" />
                Live
              </span>
            )}
            {traceFilter && (
              <span className="ml-3 inline-flex items-center gap-1.5 px-2 py-1 bg-foreground text-background text-xs font-semibold font-mono align-middle">
                Trace: {traceFilter.slice(0, 8)}...
                <button onClick={() => router.visit(basePath)} className="ml-1 hover:text-background/60 transition-colors">&times;</button>
              </span>
            )}
          </>
        }
        subtitle={liveEnabled ? 'Watching...' : (
          <><Mono>{pagination.total.toLocaleString()}</Mono> request{pagination.total !== 1 ? 's' : ''}</>
        )}
      >
        {appId && (
          <button
            onClick={() => toggleLive(!liveEnabled)}
            className={`flex items-center gap-2 px-4 py-2 text-sm font-semibold transition-all ${
              liveEnabled
                ? 'bg-[#e11d48] text-white'
                : 'border border-foreground/20 text-foreground/70 hover:border-foreground/40 hover:text-foreground'
            }`}
          >
            <span className={`w-2 h-2 ${liveEnabled ? 'bg-card animate-pulse' : 'bg-accent'}`} />
            {liveEnabled ? 'Stop' : 'Live'}
          </button>
        )}
      </PageHeader>

      {/* Filters Bar */}
      <div className="flex items-center gap-4 mb-6 p-4 bg-card border border-foreground/10">
        {/* Error Filter - ToggleGroup */}
        <ToggleGroup type="single" value={errorFilter} onValueChange={handleErrorFilter} disabled={liveEnabled}>
          <ToggleGroupItem value="all" aria-label="All">All</ToggleGroupItem>
          <ToggleGroupItem value="success" aria-label="Success">Success</ToggleGroupItem>
          <ToggleGroupItem value="errors" aria-label="Errors">Errors</ToggleGroupItem>
        </ToggleGroup>

        <div className="h-5 w-px bg-foreground/10" />

        {/* Date Filter */}
        <ToggleGroup type="single" value={datePreset} onValueChange={(v) => v && handleDateFilter(v as DatePreset)} disabled={liveEnabled}>
          <ToggleGroupItem value="today" aria-label="Today">Today</ToggleGroupItem>
          <ToggleGroupItem value="7d" aria-label="7 days">7d</ToggleGroupItem>
          <ToggleGroupItem value="30d" aria-label="30 days">30d</ToggleGroupItem>
          <ToggleGroupItem value="all" aria-label="All time">All</ToggleGroupItem>
        </ToggleGroup>

        <div className="h-5 w-px bg-foreground/10" />

        {/* Search */}
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-foreground/40" />
          <Input
            type="text"
            placeholder="Search..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
            className="pl-9 bg-foreground/[0.02] border-foreground/10"
            disabled={liveEnabled}
          />
        </div>

        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={clearFilters} disabled={liveEnabled}>
            Clear
          </Button>
        )}

        <div className="flex-1" />

        {/* Group by trace */}
        {!filters.trace && !filters.search && (
          <button
            onClick={() => !liveEnabled && toggleGrouping(!groupEnabled)}
            disabled={liveEnabled}
            className={`flex items-center gap-2 px-3 py-2 text-sm font-semibold transition-all ${
              groupEnabled
                ? 'bg-foreground text-background'
                : 'text-foreground/50 hover:text-foreground hover:bg-foreground/[0.02]'
            } ${liveEnabled ? 'opacity-50 cursor-not-allowed' : ''}`}
          >
            <Layers className="w-4 h-4" />
            Traces
          </button>
        )}
      </div>

      {/* Volume Histogram */}
      {!liveEnabled && histogram.length > 0 && (
        <LogHistogram buckets={histogram} />
      )}

      {/* Log List */}
      {entries.length === 0 ? (
        <div className="bg-card border border-foreground/10 py-16 text-center">
          <Inbox className="w-10 h-10 text-foreground/20 mx-auto mb-4" />
          <p className="font-semibold text-foreground mb-1">
            {hasFilters ? 'Nothing here' : 'Quiet so far'}
          </p>
          <p className="text-sm text-foreground/50">
            {hasFilters
              ? 'Try different filters.'
              : 'Logs appear once your app is connected.'}
          </p>
          {hasFilters && (
            <Button variant="link" onClick={clearFilters} className="mt-4">
              Clear filters
            </Button>
          )}
        </div>
      ) : groupEnabled && !filters.trace && !filters.search ? (
        <div className="bg-card border border-foreground/10 overflow-hidden">
          <div className="flex items-center gap-3 py-3 px-5 border-b border-foreground/10 text-xs font-semibold text-foreground/40 uppercase tracking-wide">
            <span style={{ width: '145px' }}>Time</span>
            <span className="w-14"></span>
            <span className="flex-1">Path</span>
            <span className="w-10 text-right">Code</span>
            <span className="w-14 text-right">Duration</span>
          </div>
          {logGroups.map((group) => (
            <GroupedLog key={group.traceId} group={group} appId={appId} />
          ))}
        </div>
      ) : (
        <div className="bg-card border border-foreground/10 overflow-hidden">
          <div className="flex items-center gap-3 py-3 px-5 border-b border-foreground/10 text-xs font-semibold text-foreground/40 uppercase tracking-wide">
            <span style={{ width: '145px' }}>Time</span>
            <span className="w-14"></span>
            <span className="flex-1">Path</span>
            <span className="w-10 text-right">Code</span>
            <span className="w-14 text-right">Duration</span>
          </div>
          <div className="divide-y divide-foreground/10">
            {(filters.trace ? [...entries].sort((a, b) => a.timestamp < b.timestamp ? -1 : a.timestamp > b.timestamp ? 1 : 0) : entries).filter(e => e && e.id).map((entry) => (
              <LogRow key={entry.id} entry={entry} appId={appId} />
            ))}
          </div>
        </div>
      )}

      {/* Pagination */}
      {!liveEnabled && pagination.total > pagination.limit && (
        <div className="flex justify-between items-center mt-6 py-4">
          <p className="text-sm text-foreground/50">
            {pagination.offset + 1}–{Math.min(pagination.offset + pagination.limit, pagination.total)} of {pagination.total.toLocaleString()}
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                const params = new URLSearchParams(window.location.search)
                params.set('offset', Math.max(0, pagination.offset - pagination.limit).toString())
                router.visit(`${basePath}?${params.toString()}`)
              }}
              disabled={pagination.offset === 0}
            >
              Previous
            </Button>
            <span className="text-sm text-foreground/50 px-2">
              {currentPage} / {totalPages}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                const params = new URLSearchParams(window.location.search)
                params.set('offset', (pagination.offset + pagination.limit).toString())
                router.visit(`${basePath}?${params.toString()}`)
              }}
              disabled={pagination.offset + pagination.limit >= pagination.total}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </Layout>
  )
}
