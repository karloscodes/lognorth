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
        className="flex items-center gap-2 py-2 px-3 transition-colors hover:bg-black/[0.03] text-sm"
      >
        <div className={`p-1 rounded ${config.colors}`}>
          <Icon className="w-3 h-3" />
        </div>
        {ctx.status && (
          <span className={`font-mono font-medium ${statusColor(ctx.status)}`}>{ctx.status}</span>
        )}
        <span className="text-black/70">{entry.duration_ms}ms</span>
        <span className="text-black/70 text-xs ml-auto">
          {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}
        </span>
      </a>
    )
  }

  return (
    <a
      href={href}
      className="flex items-center gap-3 py-3 px-4 transition-colors hover:bg-black/[0.02]"
    >
      <div className={`p-2 rounded flex items-center justify-center ${config.colors}`}>
        <Icon className="w-4 h-4" />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          {ctx.method && (
            <span className={`font-mono text-sm font-semibold ${methodColor(ctx.method)}`}>
              {ctx.method}
            </span>
          )}
          {ctx.status && (
            <span className={`font-mono text-sm ${statusColor(ctx.status)}`}>{ctx.status}</span>
          )}
          {ctx.job && (
            <span className="font-mono text-sm font-semibold text-purple-600">{ctx.job}</span>
          )}
          <span className="text-xs text-black/70">
            {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}
          </span>
        </div>
        <p className="font-mono text-sm text-black truncate mt-0.5">{entry.message || '(no message)'}</p>
      </div>
      <span className="font-mono text-xs text-black/70 shrink-0">{entry.duration_ms}ms</span>
    </a>
  )
}

function GroupedLog({ group, appId }: { group: LogGroup; appId?: number }) {
  const [expanded, setExpanded] = useState(false)
  const { mainEntry, entries } = group
  const config = getConfig(mainEntry.is_error)
  const Icon = config.icon
  const href = appId ? `/apps/${appId}/log/${mainEntry.id}` : `/log/${mainEntry.id}`
  const hasMultiple = entries.length > 1
  const totalDuration = entries.reduce((sum, e) => sum + e.duration_ms, 0)
  const ctx = parseContext(mainEntry.context)

  return (
    <div className="border-b border-black/10 last:border-b-0">
      <div className="flex items-center">
        {hasMultiple && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="p-3 hover:bg-black/[0.02] transition-colors"
          >
            <ChevronRight className={`w-4 h-4 text-black/70 transition-transform ${expanded ? 'rotate-90' : ''}`} />
          </button>
        )}
        <a
          href={href}
          className={`flex-1 flex items-center gap-3 py-3 pr-4 transition-colors hover:bg-black/[0.02] ${hasMultiple ? '' : 'pl-4'}`}
        >
          <div className={`p-2 rounded flex items-center justify-center ${config.colors}`}>
            <Icon className="w-4 h-4" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              {ctx.method && (
                <span className={`font-mono text-sm font-semibold ${methodColor(ctx.method)}`}>
                  {ctx.method}
                </span>
              )}
              {ctx.status && (
                <span className={`font-mono text-sm ${statusColor(ctx.status)}`}>{ctx.status}</span>
              )}
              {ctx.job && (
                <span className="font-mono text-sm font-semibold text-purple-600">{ctx.job}</span>
              )}
              {hasMultiple && (
                <span className="text-xs bg-black/10 text-black/70 px-1.5 py-0.5 rounded font-medium">
                  {entries.length} events
                </span>
              )}
              <span className="text-xs text-black/70">
                {formatDistanceToNow(new Date(mainEntry.timestamp), { addSuffix: true })}
              </span>
            </div>
            <p className="font-mono text-sm text-black truncate mt-0.5">{mainEntry.message}</p>
          </div>
          <span className="font-mono text-xs text-black/70 shrink-0">{totalDuration}ms</span>
        </a>
      </div>

      {expanded && hasMultiple && (
        <div className="bg-black/[0.02] border-t border-black/5">
          {entries.map((entry, idx) => (
            <div key={entry.id} className="flex items-stretch">
              <div className="w-10 flex flex-col items-center py-2">
                <div className={`w-2 h-2 rounded-full ${idx === entries.length - 1 ? 'bg-black' : 'bg-black/30'}`} />
                {idx < entries.length - 1 && <div className="w-px flex-1 bg-black/20 mt-1" />}
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
      {/* Page Title */}
      <div className="mb-6">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-bold text-black">Log</h2>
          {liveEnabled && (
            <span className="flex items-center gap-1.5 text-xs font-semibold text-red-600">
              <span className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />
              Live
            </span>
          )}
        </div>
        <p className="text-sm text-black/70 mt-1">
          {liveEnabled ? 'Streaming new entries' : `${pagination.total.toLocaleString()} entries`}
          {hasFilters && !liveEnabled && ' (filtered)'}
        </p>
      </div>

      {/* Filters Bar */}
      <div className="flex items-center justify-between gap-6 mb-6 pb-6 border-b border-black/10">
        <div className="flex items-center gap-4">
          {/* Error Filter - ToggleGroup */}
          <ToggleGroup type="single" value={errorFilter} onValueChange={handleErrorFilter} disabled={liveEnabled}>
            <ToggleGroupItem value="all" aria-label="All">All</ToggleGroupItem>
            <ToggleGroupItem value="success" aria-label="Success">Success</ToggleGroupItem>
            <ToggleGroupItem value="errors" aria-label="Errors">Errors</ToggleGroupItem>
          </ToggleGroup>

          <div className="h-5 w-px bg-black/10" />

          {/* Search */}
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-black/40" />
            <Input
              type="text"
              placeholder="Search logs..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
              className="w-56 pl-8"
              disabled={liveEnabled}
            />
          </div>
          {hasFilters && (
            <Button variant="ghost" size="sm" onClick={clearFilters} disabled={liveEnabled}>
              Clear
            </Button>
          )}
        </div>

        <div className="flex items-center gap-4">
          {/* Group by trace */}
          <div className={`flex items-center gap-2 px-3 py-1.5 rounded transition-colors ${groupEnabled ? 'bg-black/5' : ''}`}>
            <span className={`text-sm ${groupEnabled ? 'font-medium text-black' : 'text-black/60'}`}>
              Group
            </span>
            <Switch
              checked={groupEnabled}
              onCheckedChange={toggleGrouping}
              disabled={liveEnabled}
            />
          </div>

          {/* Live */}
          {appId && (
            <div className={`flex items-center gap-2 px-3 py-1.5 rounded transition-colors ${liveEnabled ? 'bg-red-50 border border-red-200' : ''}`}>
              {liveEnabled && <span className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />}
              <span className={`text-sm ${liveEnabled ? 'font-medium text-red-600' : 'text-black/60'}`}>
                Live
              </span>
              <Switch
                checked={liveEnabled}
                onCheckedChange={toggleLive}
              />
            </div>
          )}
        </div>
      </div>

      {/* Log List */}
      {entries.length === 0 ? (
        <div className="py-20 text-center">
          <Inbox className="w-12 h-12 text-black/30 mx-auto mb-4" />
          <p className="text-lg text-black/70 mb-2">
            {hasFilters ? 'Nothing matches' : 'No log entries yet'}
          </p>
          <p className="text-sm text-black/70 max-w-sm mx-auto">
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
        <div className="border border-black/10 rounded-sm">
          {logGroups.map((group) => (
            <GroupedLog key={group.traceId} group={group} appId={appId} />
          ))}
        </div>
      ) : (
        <div className="border border-black/10 rounded-sm divide-y divide-black/5">
          {entries.filter(e => e && e.id).map((entry) => (
            <LogRow key={entry.id} entry={entry} appId={appId} />
          ))}
        </div>
      )}

      {/* Pagination - hidden in live mode */}
      {!liveEnabled && pagination.total > pagination.limit && (
        <div className="flex justify-between items-center mt-8 pt-6 border-t border-black/10">
          <p className="text-sm text-black/70">
            {pagination.offset + 1}–{Math.min(pagination.offset + pagination.limit, pagination.total)} of {pagination.total.toLocaleString()}
          </p>
          <div className="flex items-center gap-4">
            <span className="text-sm text-black/70">
              Page {currentPage} of {totalPages}
            </span>
            <div className="flex gap-2">
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
                Prev
              </Button>
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
        </div>
      )}
    </Layout>
  )
}
