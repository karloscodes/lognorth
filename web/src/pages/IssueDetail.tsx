import { formatDistanceToNow, format } from 'date-fns'
import { Clock, AlertTriangle } from 'lucide-react'
import { Link } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { getConfig, statusColor, methodColor, parseContext } from '@/lib/shared'
import type { Application, LogEntry, Issue } from '@/lib/shared'

interface IssueDetail extends Issue {
  entries: LogEntry[]
}

interface Props {
  issue: IssueDetail
  appId?: number
  appName?: string
  applications?: Application[]
}

export default function IssueDetailPage({ issue, appId, appName, applications }: Props) {
  const issuesPath = appId ? `/apps/${appId}/issues` : '/issues'
  const logDetailPath = (entryId: number) =>
    appId ? `/apps/${appId}/log/${entryId}` : `/log/${entryId}`

  // Build app name lookup for showing which app each entry belongs to
  const appNameById = new Map<number, string>()
  applications?.forEach(app => appNameById.set(app.id, app.name))
  const showAppName = !appId // Show app name when viewing globally

  // Get unique apps affected by this issue
  const entries = issue.entries || []
  const affectedAppIds = [...new Set(entries.map(e => e.api_key_id))]
  const affectedApps = affectedAppIds.map(id => appNameById.get(id)).filter(Boolean)

  return (
    <Layout currentPath="/issues" appId={appId} appName={appName} applications={applications}>
      {/* Issue Header */}
      <div className="mb-8">
        <div className="flex items-start gap-3 mb-4">
          <div className="p-2 bg-red-100 shrink-0 mt-0.5">
            <AlertTriangle className="w-5 h-5 text-red-700" />
          </div>
          <div className="min-w-0">
            <h1 className="text-xl font-bold text-black leading-snug break-words">
              {issue.error}
            </h1>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 mt-3 text-sm text-black/60">
              {showAppName && affectedApps.length > 0 && (
                <>
                  <span>
                    <strong className="text-black">{affectedApps.join(', ')}</strong>
                  </span>
                  <span className="text-black/20">·</span>
                </>
              )}
              <span>
                <strong className="text-black">{issue.count}</strong> times
              </span>
              <span className="text-black/20">·</span>
              <span>
                First seen {formatDistanceToNow(new Date(issue.first_seen), { addSuffix: true })}
              </span>
              <span className="text-black/20">·</span>
              <span>
                Last seen {formatDistanceToNow(new Date(issue.last_seen), { addSuffix: true })}
              </span>
            </div>
          </div>
        </div>

        {/* Affected Paths */}
        {issue.paths?.length > 0 && (
          <div className="mt-4">
            <h3 className="text-xs font-semibold text-black/50 uppercase tracking-wide mb-2">
              Where it happened
            </h3>
            <div className="flex flex-wrap gap-2">
              {issue.paths?.map((path, i) => (
                <code
                  key={i}
                  className="bg-black/[0.03] text-black/70 px-2 py-1 text-sm font-mono"
                >
                  {path}
                </code>
              ))}
            </div>
          </div>
        )}

        {/* Error Timeline */}
        <ErrorTimeline entries={entries} />
      </div>

      {/* Occurrences */}
      <div>
        <h2 className="text-sm font-semibold text-black/50 uppercase tracking-wide mb-4 flex items-center gap-2">
          <Clock className="w-4 h-4" />
          History
        </h2>

        <div className="border border-black/10 rounded-sm divide-y divide-black/5">
          {entries.slice(0, 50).map((entry) => (
            <LogRow
              key={entry.id}
              entry={entry}
              detailPath={logDetailPath(entry.id)}
              appName={showAppName ? appNameById.get(entry.api_key_id) : undefined}
            />
          ))}
        </div>

        {entries.length > 50 && (
          <p className="text-sm text-black/50 mt-4 text-center">
            Showing 50 of {entries.length} occurrences
          </p>
        )}
      </div>
    </Layout>
  )
}

function LogRow({ entry, detailPath, appName }: { entry: LogEntry; detailPath: string; appName?: string }) {
  const config = getConfig(entry.is_error)
  const Icon = config.icon
  const ctx = parseContext(entry.context)

  return (
    <Link
      href={detailPath}
      className="flex items-center gap-4 py-3 px-4 hover:bg-black/[0.02] transition-colors"
    >
      <div className={`p-1.5 ${config.colors}`}>
        <Icon className="w-3.5 h-3.5" />
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 text-sm">
          {appName && (
            <span className="text-xs bg-black/5 text-black/60 px-1.5 py-0.5 font-medium">
              {appName}
            </span>
          )}
          {ctx.method && (
            <span className={`font-mono font-semibold ${methodColor(ctx.method)}`}>
              {ctx.method}
            </span>
          )}
          {ctx.job && (
            <span className="font-mono font-semibold text-purple-600">{ctx.job}</span>
          )}
          <span className="font-mono text-black/70 truncate">{entry.message}</span>
        </div>
      </div>

      <div className="flex items-center gap-4 shrink-0">
        {ctx.status && (
          <Badge variant={ctx.status >= 500 ? 'destructive' : 'secondary'} className="font-mono">
            {ctx.status}
          </Badge>
        )}
        <span className="text-xs text-black/50 w-24 text-right">
          {format(new Date(entry.timestamp), 'MMM d, HH:mm')}
        </span>
        <span className="font-mono text-xs text-black/50 w-16 text-right">
          {entry.duration_ms}ms
        </span>
      </div>
    </Link>
  )
}

function ErrorTimeline({ entries }: { entries: LogEntry[] }) {
  if (entries.length < 2) return null

  // Group by day
  const byDay = new Map<string, number>()
  entries.forEach(entry => {
    const day = format(new Date(entry.timestamp), 'MMM d')
    byDay.set(day, (byDay.get(day) || 0) + 1)
  })

  // Get last 7 days of data
  const days = Array.from(byDay.entries()).slice(0, 7).reverse()
  if (days.length < 2) return null

  const maxCount = Math.max(...days.map(([, count]) => count))
  const maxHeight = 48 // pixels

  return (
    <div className="mt-6">
      <h3 className="text-xs font-semibold text-black/50 uppercase tracking-wide mb-3">
        When it happened
      </h3>
      <div className="flex items-end gap-2">
        {days.map(([day, count]) => {
          const height = Math.max(8, Math.round((count / maxCount) * maxHeight))
          return (
            <div key={day} className="flex-1 flex flex-col items-center gap-1">
              <span className="text-xs text-black/60 font-mono">{count}</span>
              <div
                className="w-full bg-red-500 rounded-sm"
                style={{ height: `${height}px` }}
                title={`${count} errors on ${day}`}
              />
              <span className="text-[10px] text-black/40">{day}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
