import { useState, useEffect } from 'react'
import { router } from '@inertiajs/react'
import { formatDistanceToNow } from 'date-fns'
import { Search, Inbox, ChevronRight, Layers } from 'lucide-react'
import { Layout } from '@/components/layout'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { getConfig, statusColor, methodColor, parseContext, PageHeader, Mono, Code } from '@/lib/shared'
import type { Application, LogEntry } from '@/lib/shared'

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
      const sorted = groupEntries.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())
      return {
        traceId,
        entries: sorted,
        mainEntry: sorted[sorted.length - 1],
        firstTimestamp: new Date(sorted[0].timestamp),
      }
    })
    .sort((a, b) => b.firstTimestamp.getTime() - a.firstTimestamp.getTime())
}

function LogRow({ entry, appId, compact, hideErrorBorder }: { entry: LogEntry; appId?: number; compact?: boolean; hideErrorBorder?: boolean }) {
  const config = getConfig(entry.is_error)
  const Icon = config.icon
  const href = appId ? `/apps/${appId}/log/${entry.id}` : `/log/${entry.id}`
  const ctx = parseContext(entry.context)

  if (compact) {
    return (
      <a
        href={href}
        className="flex items-center gap-3 py-2 px-3 transition-colors hover:bg-black/[0.03] text-sm"
      >
        <span className="font-mono text-xs text-black/60 truncate flex-1">
          {entry.message || ctx.path || ctx.job || '—'}
        </span>
        {ctx.status && (
          <span className={`font-mono text-xs font-medium ${statusColor(ctx.status)}`}>
            {ctx.status}
          </span>
        )}
        <span className="text-black/40 text-xs tabular-nums">{entry.duration_ms}ms</span>
      </a>
    )
  }

  const isError = entry.is_error
  const showErrorBorder = isError && !hideErrorBorder

  return (
    <a
      href={href}
      className={`flex items-center gap-3 py-3 px-5 transition-colors hover:bg-black/[0.02] ${showErrorBorder ? 'shadow-[inset_3px_0_0_0_#e11d48]' : ''}`}
    >
      {/* Timestamp */}
      <span className="text-xs text-black/50 tabular-nums w-16 shrink-0">
        {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true }).replace('about ', '').replace(' ago', '')}
      </span>

      {/* Type badge */}
      <span className="w-14 shrink-0">
        {ctx.method ? (
          <span className="font-mono text-xs font-semibold px-1.5 py-0.5 text-black/50 bg-black/[0.04]">
            {ctx.method}
          </span>
        ) : ctx.job ? (
          <span className="font-mono text-xs font-semibold px-1.5 py-0.5 text-black/50 bg-black/[0.04]">
            JOB
          </span>
        ) : null}
      </span>

      {/* Path + Error inline */}
      <div className="flex-1 min-w-0 flex items-center gap-3">
        <span className="font-mono text-sm text-black font-semibold truncate">
          {ctx.path || ctx.job || entry.message}
        </span>
        {isError && ctx.error && (
          <span className="font-mono text-xs text-[#e11d48] truncate">
            {ctx.error}
          </span>
        )}
      </div>

      {/* Status + Duration */}
      <div className="flex items-center gap-3 shrink-0">
        {ctx.status ? (
          <span className={`font-mono text-sm font-bold w-10 text-right ${
            ctx.status >= 500 ? 'text-[#e11d48]' :
            ctx.status >= 400 ? 'text-black' :
            'text-black/50'
          }`}>
            {ctx.status}
          </span>
        ) : (
          <span className="w-10" />
        )}
        <span className="font-mono text-xs text-black/50 tabular-nums w-14 text-right">
          {entry.duration_ms}ms
        </span>
      </div>
    </a>
  )
}

function GroupedLog({ group, appId }: { group: LogGroup; appId?: number }) {
  const [expanded, setExpanded] = useState(true)
  const { mainEntry, entries, traceId } = group
  const totalDuration = entries.reduce((sum, e) => sum + e.duration_ms, 0)
  const hasError = entries.some(e => e.is_error)

  return (
    <div className="border-b border-black/10 last:border-b-0">
      {/* Group header */}
      <button
        onClick={() => setExpanded(!expanded)}
        className={`w-full flex items-center gap-3 py-3 px-5 text-left bg-black/[0.02] hover:bg-black/[0.04] transition-colors ${hasError ? 'shadow-[inset_3px_0_0_0_#e11d48]' : ''}`}
      >
        <ChevronRight className={`w-4 h-4 text-black/40 transition-transform shrink-0 ${expanded ? 'rotate-90' : ''}`} />
        <span className="font-mono text-xs text-black/50">{traceId.slice(0, 12)}...</span>
        <span className="text-xs text-black/40">·</span>
        <span className="text-xs text-black/50">
          {formatDistanceToNow(new Date(mainEntry.timestamp), { addSuffix: true }).replace('about ', '').replace(' ago', '')}
        </span>
        <span className="text-xs text-black/40">·</span>
        <span className="text-xs text-black/50">{entries.length} event{entries.length !== 1 ? 's' : ''}</span>
        <span className="flex-1" />
        <span className="font-mono text-xs text-black/50 tabular-nums">{totalDuration}ms</span>
      </button>

      {/* Expanded children */}
      {expanded && (
        <div className="border-t border-black/5">
          {entries.map((entry) => (
            <div key={entry.id} className="pl-8">
              <LogRow entry={entry} appId={appId} hideErrorBorder />
            </div>
          ))}
        </div>
      )}
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

  const maxTotal = Math.max(...buckets.map(b => b.total), 1)
  const hoveredBucket = hoveredIdx !== null ? buckets[hoveredIdx] : null

  return (
    <div className="mb-4 bg-white border border-black/10 p-3 relative">
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
          <span className="bg-black text-white px-2 py-1">
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
  if (!filters.start_time) return '7d' // Default is 7 days
  const start = new Date(filters.start_time)
  const now = new Date()
  const diffDays = Math.round((now.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))
  if (diffDays <= 1) return 'today'
  if (diffDays <= 7) return '7d'
  if (diffDays <= 30) return '30d'
  return 'all'
}

const getStartTimeForPreset = (preset: DatePreset): string | undefined => {
  if (preset === 'all') return undefined
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
  const [groupEnabled, setGroupEnabled] = useState(() => {
    try {
      return localStorage.getItem('log_group_by_trace') === 'true'
    } catch {
      return false
    }
  })
  const [liveEnabled, setLiveEnabled] = useState(false)

  const basePath = appId ? `/apps/${appId}/log` : '/log'
  const logGroups = groupEnabled ? groupByTraceId(entries) : []

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
    try {
      localStorage.setItem('log_group_by_trace', String(checked))
    } catch {
      // ignore
    }
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
                <span className="w-1.5 h-1.5 bg-white animate-pulse" />
                Live
              </span>
            )}
            {traceFilter && (
              <span className="ml-3 inline-flex items-center gap-1.5 px-2 py-1 bg-black text-white text-xs font-semibold font-mono align-middle">
                Trace: {traceFilter.slice(0, 8)}...
              </span>
            )}
          </>
        }
        subtitle={liveEnabled ? 'Watching...' : (
          <><Mono>{pagination.total.toLocaleString()}</Mono> log{pagination.total !== 1 ? 's' : ''}{hasFilters && ' (filtered)'}</>
        )}
      >
        {appId && (
          <button
            onClick={() => toggleLive(!liveEnabled)}
            className={`flex items-center gap-2 px-4 py-2 text-sm font-semibold transition-all ${
              liveEnabled
                ? 'bg-[#e11d48] text-white'
                : 'border border-black/20 text-black/70 hover:border-black/40 hover:text-black'
            }`}
          >
            <span className={`w-2 h-2 ${liveEnabled ? 'bg-white animate-pulse' : 'bg-accent'}`} />
            {liveEnabled ? 'Stop' : 'Live'}
          </button>
        )}
      </PageHeader>

      {/* Filters Bar */}
      <div className="flex items-center gap-4 mb-6 p-4 bg-white border border-black/10">
        {/* Error Filter - ToggleGroup */}
        <ToggleGroup type="single" value={errorFilter} onValueChange={handleErrorFilter} disabled={liveEnabled}>
          <ToggleGroupItem value="all" aria-label="All">All</ToggleGroupItem>
          <ToggleGroupItem value="success" aria-label="Success">Success</ToggleGroupItem>
          <ToggleGroupItem value="errors" aria-label="Errors">Errors</ToggleGroupItem>
        </ToggleGroup>

        <div className="h-5 w-px bg-black/10" />

        {/* Date Filter */}
        <ToggleGroup type="single" value={datePreset} onValueChange={(v) => v && handleDateFilter(v as DatePreset)} disabled={liveEnabled}>
          <ToggleGroupItem value="today" aria-label="Today">Today</ToggleGroupItem>
          <ToggleGroupItem value="7d" aria-label="7 days">7d</ToggleGroupItem>
          <ToggleGroupItem value="30d" aria-label="30 days">30d</ToggleGroupItem>
          <ToggleGroupItem value="all" aria-label="All time">All</ToggleGroupItem>
        </ToggleGroup>

        <div className="h-5 w-px bg-black/10" />

        {/* Search */}
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-black/40" />
          <Input
            type="text"
            placeholder="Search..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
            className="pl-9 bg-black/[0.02] border-black/10"
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
        <button
          onClick={() => !liveEnabled && toggleGrouping(!groupEnabled)}
          disabled={liveEnabled}
          className={`flex items-center gap-2 px-3 py-2 text-sm font-semibold transition-all ${
            groupEnabled
              ? 'bg-black text-white'
              : 'text-black/50 hover:text-black hover:bg-black/[0.02]'
          } ${liveEnabled ? 'opacity-50 cursor-not-allowed' : ''}`}
        >
          <Layers className="w-4 h-4" />
          Traces
        </button>
      </div>

      {/* Volume Histogram */}
      {!liveEnabled && histogram.length > 0 && (
        <LogHistogram buckets={histogram} />
      )}

      {/* Log List */}
      {entries.length === 0 ? (
        <div className="bg-white border border-black/10 py-16 text-center">
          <Inbox className="w-10 h-10 text-black/20 mx-auto mb-4" />
          <p className="font-semibold text-black mb-1">
            {hasFilters ? 'Nothing here' : 'Quiet so far'}
          </p>
          <p className="text-sm text-black/50">
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
      ) : groupEnabled ? (
        <div className="bg-white border border-black/10 overflow-hidden">
          <div className="flex items-center gap-3 py-3 px-5 border-b border-black/10 text-xs font-semibold text-black/40 uppercase tracking-wide">
            <span className="w-16">Time</span>
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
        <div className="bg-white border border-black/10 overflow-hidden">
          <div className="flex items-center gap-3 py-3 px-5 border-b border-black/10 text-xs font-semibold text-black/40 uppercase tracking-wide">
            <span className="w-16">Time</span>
            <span className="w-14"></span>
            <span className="flex-1">Path</span>
            <span className="w-10 text-right">Code</span>
            <span className="w-14 text-right">Duration</span>
          </div>
          <div className="divide-y divide-black/10">
            {entries.filter(e => e && e.id).map((entry) => (
              <LogRow key={entry.id} entry={entry} appId={appId} />
            ))}
          </div>
        </div>
      )}

      {/* Pagination */}
      {!liveEnabled && pagination.total > pagination.limit && (
        <div className="flex justify-between items-center mt-6 py-4">
          <p className="text-sm text-black/50">
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
            <span className="text-sm text-black/50 px-2">
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
