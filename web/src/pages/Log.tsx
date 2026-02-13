import { useState, useEffect } from 'react'
import { router } from '@inertiajs/react'
import { formatDistanceToNow } from 'date-fns'
import { Search, Inbox, ChevronRight } from 'lucide-react'
import { Layout } from '@/components/layout'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { getConfig, statusColor, methodColor, parseContext } from '@/lib/shared'
import type { Application, LogEntry } from '@/lib/shared'

interface Filters {
  is_error?: boolean
  search?: string
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

function LogRow({ entry, appId, compact }: { entry: LogEntry; appId?: number; compact?: boolean }) {
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
        <span className={`font-mono text-xs font-medium ${statusColor(ctx.status)}`}>
          {ctx.status || '—'}
        </span>
        <span className="text-black/50 text-xs">{entry.duration_ms}ms</span>
        <span className="text-black/40 text-xs ml-auto">
          {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}
        </span>
      </a>
    )
  }

  // Pro two-line layout: scannable, clean, informative
  const isError = entry.is_error
  const hasPath = ctx.path || ctx.job

  return (
    <a
      href={href}
      className={`block py-4 px-5 transition-colors hover:bg-black/[0.02] ${isError ? 'border-l-2 border-l-red-500' : ''}`}
    >
      {/* Line 1: Timestamp + Method/Job + Path */}
      <div className="flex items-center gap-3">
        {/* Timestamp - always visible, muted */}
        <span className="text-xs text-black/40 tabular-nums w-20 shrink-0">
          {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}
        </span>

        {/* Method badge or Job badge */}
        {ctx.method ? (
          <span className={`font-mono text-xs font-semibold px-1.5 py-0.5 rounded ${methodColor(ctx.method)} bg-black/[0.04]`}>
            {ctx.method}
          </span>
        ) : ctx.job ? (
          <span className="font-mono text-xs font-semibold px-1.5 py-0.5 rounded text-purple-700 bg-purple-50">
            {ctx.job}
          </span>
        ) : null}

        {/* Path - the main content */}
        {ctx.path && (
          <span className="font-mono text-sm text-black font-medium truncate">
            {ctx.path}
          </span>
        )}

        {/* Right side: Status + Duration */}
        <div className="flex items-center gap-3 ml-auto shrink-0">
          {ctx.status && (
            <span className={`font-mono text-sm font-semibold ${
              ctx.status >= 500 ? 'text-red-600' :
              ctx.status >= 400 ? 'text-amber-600' :
              'text-black/40'
            }`}>
              {ctx.status}
            </span>
          )}
          <span className="font-mono text-xs text-black/40 tabular-nums">
            {entry.duration_ms}ms
          </span>
        </div>
      </div>

      {/* Line 2: Message or Error - only if meaningful */}
      {(isError && ctx.error) || (!hasPath && entry.message) ? (
        <div className="mt-2 pl-[92px]">
          {isError && ctx.error ? (
            <p className="text-sm text-red-600 truncate">
              {ctx.error.length > 120 ? ctx.error.slice(0, 120) + '…' : ctx.error}
            </p>
          ) : (
            <p className="text-sm text-black/60 truncate">
              {entry.message}
            </p>
          )}
        </div>
      ) : null}
    </a>
  )
}

function GroupedLog({ group, appId }: { group: LogGroup; appId?: number }) {
  const [expanded, setExpanded] = useState(false)
  const { mainEntry, entries } = group
  const href = appId ? `/apps/${appId}/log/${mainEntry.id}` : `/log/${mainEntry.id}`
  const hasMultiple = entries.length > 1
  const totalDuration = entries.reduce((sum, e) => sum + e.duration_ms, 0)
  const ctx = parseContext(mainEntry.context)
  const isError = mainEntry.is_error
  const hasPath = ctx.path || ctx.job

  return (
    <div className="border-b border-black/10 last:border-b-0">
      <div className={`flex items-stretch ${isError ? 'border-l-2 border-l-red-500' : ''}`}>
        {hasMultiple && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="px-3 hover:bg-black/[0.02] transition-colors flex items-center"
          >
            <ChevronRight className={`w-4 h-4 text-black/40 transition-transform ${expanded ? 'rotate-90' : ''}`} />
          </button>
        )}
        <a
          href={href}
          className={`flex-1 block py-4 pr-5 transition-colors hover:bg-black/[0.02] ${hasMultiple ? '' : 'pl-5'}`}
        >
          {/* Line 1: Timestamp + Method/Job + Path */}
          <div className="flex items-center gap-3">
            <span className="text-xs text-black/40 tabular-nums w-20 shrink-0">
              {formatDistanceToNow(new Date(mainEntry.timestamp), { addSuffix: true })}
            </span>

            {ctx.method ? (
              <span className={`font-mono text-xs font-semibold px-1.5 py-0.5 rounded ${methodColor(ctx.method)} bg-black/[0.04]`}>
                {ctx.method}
              </span>
            ) : ctx.job ? (
              <span className="font-mono text-xs font-semibold px-1.5 py-0.5 rounded text-purple-700 bg-purple-50">
                {ctx.job}
              </span>
            ) : null}

            {ctx.path && (
              <span className="font-mono text-sm text-black font-medium truncate">
                {ctx.path}
              </span>
            )}

            {hasMultiple && (
              <span className="text-xs text-black/50 font-medium">
                ×{entries.length}
              </span>
            )}

            <div className="flex items-center gap-3 ml-auto shrink-0">
              {ctx.status && (
                <span className={`font-mono text-sm font-semibold ${
                  ctx.status >= 500 ? 'text-red-600' :
                  ctx.status >= 400 ? 'text-amber-600' :
                  'text-black/40'
                }`}>
                  {ctx.status}
                </span>
              )}
              <span className="font-mono text-xs text-black/40 tabular-nums">
                {totalDuration}ms
              </span>
            </div>
          </div>

          {/* Line 2: Error message if applicable */}
          {(isError && ctx.error) || (!hasPath && mainEntry.message) ? (
            <div className="mt-2 pl-[92px]">
              {isError && ctx.error ? (
                <p className="text-sm text-red-600 truncate">
                  {ctx.error.length > 120 ? ctx.error.slice(0, 120) + '…' : ctx.error}
                </p>
              ) : (
                <p className="text-sm text-black/60 truncate">
                  {mainEntry.message}
                </p>
              )}
            </div>
          ) : null}
        </a>
      </div>

      {expanded && hasMultiple && (
        <div className="bg-black/[0.015] border-t border-black/5">
          {entries.map((entry, idx) => (
            <div key={entry.id} className="flex items-stretch">
              <div className="w-8 flex flex-col items-center py-2">
                <div className={`w-1.5 h-1.5 rounded-full ${idx === entries.length - 1 ? 'bg-black' : 'bg-black/20'}`} />
                {idx < entries.length - 1 && <div className="w-px flex-1 bg-black/10 mt-1" />}
              </div>
              <div className="flex-1">
                <LogRow entry={entry} appId={appId} compact />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default function Log({ entries: initialEntries = [], filters = {}, pagination = { limit: 50, offset: 0, total: 0 }, appId, appName, applications }: Props) {
  const [entries, setEntries] = useState(initialEntries)
  const [search, setSearch] = useState(filters.search || '')
  const [errorFilter, setErrorFilter] = useState<string>(
    filters.is_error === true ? 'errors' : filters.is_error === false ? 'success' : 'all'
  )
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

  const applyFilters = (newFilters?: { errorFilter?: string }) => {
    const params = new URLSearchParams()
    const filterValue = newFilters?.errorFilter ?? errorFilter
    if (search) params.set('search', search)
    if (filterValue === 'errors') params.set('is_error', 'true')
    if (filterValue === 'success') params.set('is_error', 'false')
    router.visit(`${basePath}?${params.toString()}`)
  }

  const handleErrorFilter = (value: string) => {
    if (!value) return
    setErrorFilter(value)
    applyFilters({ errorFilter: value })
  }

  const clearFilters = () => {
    setSearch('')
    setErrorFilter('all')
    router.visit(basePath)
  }

  const hasFilters = search || errorFilter !== 'all'
  const totalPages = Math.ceil(pagination.total / pagination.limit)
  const currentPage = Math.floor(pagination.offset / pagination.limit) + 1

  return (
    <Layout currentPath="/log" appId={appId} appName={appName} applications={applications}>
      {/* Header */}
      <div className="flex items-center justify-between mb-8">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-black">Logs</h1>
            {liveEnabled && (
              <span className="flex items-center gap-1.5 px-2 py-1 bg-red-100 text-red-700 text-xs font-bold rounded">
                <span className="w-1.5 h-1.5 rounded-full bg-red-500 animate-pulse" />
                Live
              </span>
            )}
          </div>
          <p className="text-sm text-black/50 mt-1">
            {liveEnabled ? 'Streaming new entries' : `${pagination.total.toLocaleString()} entries`}
            {hasFilters && !liveEnabled && ' (filtered)'}
          </p>
        </div>

        {/* Live toggle */}
        {appId && (
          <button
            onClick={() => toggleLive(!liveEnabled)}
            className={`flex items-center gap-2 px-4 py-2 text-sm font-semibold rounded-lg transition-all ${
              liveEnabled
                ? 'bg-red-100 text-red-700 border border-red-200'
                : 'bg-white text-black/60 border border-black/10 hover:border-black/20'
            }`}
          >
            <span className={`w-2 h-2 rounded-full ${liveEnabled ? 'bg-red-500 animate-pulse' : 'bg-black/20'}`} />
            {liveEnabled ? 'Stop' : 'Live'}
          </button>
        )}
      </div>

      {/* Filters Bar */}
      <div className="flex items-center gap-4 mb-6 p-4 bg-white rounded-xl border border-black/10">
        {/* Error Filter - ToggleGroup */}
        <ToggleGroup type="single" value={errorFilter} onValueChange={handleErrorFilter} disabled={liveEnabled}>
          <ToggleGroupItem value="all" aria-label="All">All</ToggleGroupItem>
          <ToggleGroupItem value="success" aria-label="Success">Success</ToggleGroupItem>
          <ToggleGroupItem value="errors" aria-label="Errors">Errors</ToggleGroupItem>
        </ToggleGroup>

        <div className="h-5 w-px bg-black/10" />

        {/* Search */}
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-black/30" />
          <Input
            type="text"
            placeholder="Search logs..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
            className="pl-9 bg-black/[0.02] border-0"
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
          className={`flex items-center gap-2 px-3 py-2 text-sm font-medium rounded-lg transition-all ${
            groupEnabled
              ? 'bg-black text-white'
              : 'text-black/50 hover:text-black hover:bg-black/[0.03]'
          } ${liveEnabled ? 'opacity-50 cursor-not-allowed' : ''}`}
        >
          Group by trace
        </button>
      </div>

      {/* Log List */}
      {entries.length === 0 ? (
        <div className="bg-white rounded-xl border border-black/10 py-16 text-center">
          <Inbox className="w-10 h-10 text-black/20 mx-auto mb-4" />
          <p className="font-semibold text-black mb-1">
            {hasFilters ? 'Nothing matches' : 'No log entries yet'}
          </p>
          <p className="text-sm text-black/50">
            {hasFilters
              ? 'Try adjusting your filters or search terms.'
              : 'Log entries from your app will show up here.'}
          </p>
          {hasFilters && (
            <Button variant="link" onClick={clearFilters} className="mt-4">
              Clear filters
            </Button>
          )}
        </div>
      ) : groupEnabled ? (
        <div className="bg-white rounded-xl border border-black/10 overflow-hidden">
          {logGroups.map((group) => (
            <GroupedLog key={group.traceId} group={group} appId={appId} />
          ))}
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-black/10 divide-y divide-black/5 overflow-hidden">
          {entries.filter(e => e && e.id).map((entry) => (
            <LogRow key={entry.id} entry={entry} appId={appId} />
          ))}
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
              className="rounded-lg"
            >
              Previous
            </Button>
            <span className="text-sm text-black/40 px-2">
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
              className="rounded-lg"
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </Layout>
  )
}
