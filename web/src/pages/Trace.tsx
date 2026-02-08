import { format } from 'date-fns'
import { Hash } from 'lucide-react'
import { Layout } from '@/components/layout'
import { getConfig, statusColor, methodColor, parseContext } from '@/lib/shared'
import type { Application, LogEntry } from '@/lib/shared'

interface Props {
  traceId: string
  entries: LogEntry[]
  appId?: number
  appName?: string
  applications?: Application[]
}

export default function Trace({ traceId, entries, appId, appName, applications }: Props) {
  const hasError = entries.some(e => e.is_error)
  const totalDuration = entries.reduce((sum, e) => sum + e.duration_ms, 0)
  const startTime = entries.length > 0 ? new Date(entries[0].timestamp) : null

  return (
    <Layout currentPath="/log" appId={appId} appName={appName} applications={applications}>
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <Hash className="w-5 h-5 text-black/40" />
          <h1 className="text-xl font-bold text-black font-mono">{traceId}</h1>
          {hasError && (
            <span className="px-2 py-0.5 bg-red-100 text-red-700 text-xs font-semibold rounded">
              Error
            </span>
          )}
        </div>
        <div className="flex items-center gap-4 text-sm text-black/60">
          {appName && <span className="font-medium">{appName}</span>}
          <span>{entries.length} events</span>
          <span>{totalDuration}ms total</span>
          {startTime && <span>{format(startTime, 'MMM d, HH:mm:ss')}</span>}
        </div>
      </div>

      {/* Timeline */}
      <div className="border-2 border-black/10 rounded-sm overflow-hidden">
        {entries.map((entry, idx) => {
          const config = getConfig(entry.is_error)
          const Icon = config.icon
          const ctx = parseContext(entry.context)
          const isFirst = idx === 0
          const isLast = idx === entries.length - 1

          return (
            <a
              key={entry.id}
              href={`/apps/${entry.api_key_id}/log/${entry.id}`}
              className="flex items-start gap-4 py-4 px-5 hover:bg-black/[0.02] transition-colors border-b border-black/5 last:border-b-0"
            >
              {/* Step indicator */}
              <div className="flex flex-col items-center">
                <div className={`w-8 h-8 rounded-full flex items-center justify-center ${config.colors}`}>
                  <Icon className="w-4 h-4" />
                </div>
                {!isLast && (
                  <div className="w-0.5 h-8 bg-black/10 mt-2" />
                )}
              </div>

              {/* Content */}
              <div className="flex-1 min-w-0 pt-1">
                <div className="flex items-center gap-2 mb-1">
                  {ctx.method && (
                    <span className={`font-mono font-semibold ${methodColor(ctx.method)}`}>
                      {ctx.method}
                    </span>
                  )}
                  {ctx.status && (
                    <span className={`font-mono font-semibold ${statusColor(ctx.status)}`}>
                      {ctx.status}
                    </span>
                  )}
                  {ctx.job && (
                    <span className="font-mono font-semibold text-purple-600">{ctx.job}</span>
                  )}
                  {isFirst && (
                    <span className="text-xs bg-black/5 text-black/60 px-1.5 py-0.5 rounded">start</span>
                  )}
                  {isLast && entries.length > 1 && (
                    <span className="text-xs bg-black/5 text-black/60 px-1.5 py-0.5 rounded">end</span>
                  )}
                </div>
                <p className="text-sm text-black/80 font-mono truncate">{entry.message}</p>
                {ctx.error && (
                  <p className="text-sm text-red-600 mt-1 truncate">{ctx.error}</p>
                )}
              </div>

              {/* Duration */}
              <div className="text-right pt-1">
                <span className="font-mono text-sm text-black/70">{entry.duration_ms}ms</span>
                <p className="text-xs text-black/40 mt-0.5">
                  {format(new Date(entry.timestamp), 'HH:mm:ss.SSS')}
                </p>
              </div>
            </a>
          )
        })}
      </div>
    </Layout>
  )
}
