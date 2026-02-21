import { formatDistanceToNow } from 'date-fns'
import { Clock, Globe, User, Hash, Briefcase } from 'lucide-react'
import { Layout } from '@/components/layout'
import { getConfig, statusColor, methodColor, parseContext } from '@/lib/shared'
import type { Application, LogEntry, LogContext } from '@/lib/shared'

interface Props {
  entry: LogEntry
  entryContext: LogContext
  relatedEntries: LogEntry[]
  appId?: number
  appName?: string
  applications?: Application[]
}

export default function LogDetail({ entry, entryContext, relatedEntries, appId, appName, applications }: Props) {
  const config = getConfig(entry.is_error)
  const Icon = config.icon
  const logPath = appId ? `/apps/${appId}/log` : '/log'
  const ctx = entryContext || parseContext(entry.context)
  const isHttpRequest = ctx.method && ctx.path
  const isJob = ctx.job

  return (
    <Layout currentPath="/log" appId={appId} appName={appName} applications={applications}>
      {/* Header Card */}
      <div className="border-2 border-black/10 overflow-hidden mb-8">
        <div className="flex items-center gap-4 p-6">
          <div className={`p-3 ${config.colors}`}>
            <Icon className="w-6 h-6" />
          </div>
          <div className="flex-1 min-w-0">
            {isHttpRequest ? (
              <>
                <div className="flex items-center gap-3">
                  <span className={`font-mono text-xl font-bold ${methodColor(ctx.method)}`}>
                    {ctx.method}
                  </span>
                  <span className={`text-3xl font-bold ${statusColor(ctx.status)}`}>
                    {ctx.status}
                  </span>
                </div>
                <p className="font-mono text-black mt-1 break-all">{ctx.path}</p>
              </>
            ) : isJob ? (
              <>
                <div className="flex items-center gap-3">
                  <Briefcase className="w-5 h-5 text-purple-600" />
                  <span className="font-mono text-xl font-bold text-purple-600">{ctx.job}</span>
                </div>
                <p className="text-black mt-1">{entry.message}</p>
              </>
            ) : (
              <p className="text-xl font-bold text-black">{entry.message}</p>
            )}
          </div>
          <div className="text-right">
            <span className="font-mono text-2xl text-black/70">{entry.duration_ms}ms</span>
            <p className="text-xs text-black/70 mt-1">duration</p>
          </div>
        </div>

        {/* Quick Stats */}
        <div className="grid grid-cols-4 border-t border-black/10 divide-x divide-black/10 bg-black/[0.01]">
          <div className="px-4 py-3">
            <div className="flex items-center gap-2 text-black/70 text-xs mb-1">
              <Clock className="w-3 h-3" />
              When
            </div>
            <p className="text-sm text-black/80">
              {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}
            </p>
          </div>
          <div className="px-4 py-3">
            <div className="flex items-center gap-2 text-black/70 text-xs mb-1">
              <Hash className="w-3 h-3" />
              Trace ID
            </div>
            {entry.trace_id ? (
              <a
                href={`${logPath}?trace=${entry.trace_id}`}
                className="inline-flex items-center gap-1.5 text-sm font-mono text-black hover:text-black/70"
              >
                {entry.trace_id.slice(0, 12)}...
                <span className="text-[10px] bg-black text-white px-1.5 py-0.5 font-semibold">→</span>
              </a>
            ) : (
              <p className="text-sm text-black/80 font-mono truncate">—</p>
            )}
          </div>
          <div className="px-4 py-3">
            <div className="flex items-center gap-2 text-black/70 text-xs mb-1">
              <Globe className="w-3 h-3" />
              IP Address
            </div>
            <p className="text-sm text-black/80 font-mono">{ctx.ip || '—'}</p>
          </div>
          <div className="px-4 py-3">
            <div className="flex items-center gap-2 text-black/70 text-xs mb-1">
              <User className="w-3 h-3" />
              User
            </div>
            <p className="text-sm text-black/80">{ctx.user || '—'}</p>
          </div>
        </div>
      </div>

      {/* Error Block */}
      {ctx.error && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-[#e11d48] uppercase tracking-wider mb-3">Error</h3>
          <div className="bg-rose-50 border-2 border-[#e11d48] p-4">
            <pre className="text-sm font-mono whitespace-pre-wrap text-[#e11d48] break-words">
              {ctx.error}
            </pre>
          </div>
        </div>
      )}

      {/* User Agent */}
      {ctx.user_agent && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-black/70 uppercase tracking-wider mb-3">User Agent</h3>
          <p className="text-sm text-black/70 font-mono break-all bg-black/[0.02] p-3">
            {ctx.user_agent}
          </p>
        </div>
      )}

      {/* Extra Context */}
      {ctx.extra && Object.keys(ctx.extra).length > 0 && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-black/70 uppercase tracking-wider mb-3">Context</h3>
          <div className="border-2 border-black/10 divide-y divide-black/5">
            {Object.entries(ctx.extra).map(([key, value]) => (
              <div key={key} className="flex gap-4 px-4 py-2.5 text-sm">
                <span className="text-black/70 w-32 shrink-0">{key}</span>
                <span className="font-mono text-black/80">{String(value)}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Headers */}
      {ctx.headers && Object.keys(ctx.headers).length > 0 && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-black/70 uppercase tracking-wider mb-3">Headers</h3>
          <div className="border-2 border-black/10 divide-y divide-black/5">
            {Object.entries(ctx.headers).map(([key, value]) => (
              <div key={key} className="flex gap-4 px-4 py-2.5 text-sm">
                <span className="text-black/70 font-mono w-48 shrink-0">{key}</span>
                <span className="font-mono text-black/80 break-all">{value}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Body */}
      {ctx.body && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-black/70 uppercase tracking-wider mb-3">Body</h3>
          <pre className="font-mono text-sm bg-black text-white p-4 overflow-x-auto">
            {ctx.body}
          </pre>
        </div>
      )}

      {/* Related Entries */}
      {relatedEntries.length > 1 && (
        <div id="trace-timeline" className="border-t border-black/10 pt-8">
          <h3 className="text-sm font-semibold text-black/70 uppercase tracking-wider mb-4">
            Trace ({relatedEntries.length} requests)
          </h3>
          <div className="border-2 border-black/10 overflow-hidden">
            {relatedEntries.map((e, idx) => {
              const eConfig = getConfig(e.is_error)
              const EIcon = eConfig.icon
              const isCurrent = e.id === entry.id
              const eCtx = parseContext(e.context)

              return (
                <a
                  key={e.id}
                  href={isCurrent ? undefined : `${logPath}/${e.id}`}
                  className={`flex items-center gap-3 py-3 px-4 transition-colors border-b border-black/5 last:border-b-0 ${
                    isCurrent ? 'bg-black/[0.03]' : 'hover:bg-black/[0.02] cursor-pointer'
                  }`}
                >
                  <div className="w-6 h-6 flex items-center justify-center">
                    <span className="text-xs text-black/70 font-mono">{idx + 1}</span>
                  </div>
                  <div className={`p-1.5 flex items-center justify-center ${eConfig.colors}`}>
                    <EIcon className="w-3 h-3" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      {eCtx.method && (
                        <span className={`font-mono text-sm font-semibold ${methodColor(eCtx.method)}`}>
                          {eCtx.method}
                        </span>
                      )}
                      {eCtx.status && (
                        <span className={`font-mono text-sm ${statusColor(eCtx.status)}`}>{eCtx.status}</span>
                      )}
                      {eCtx.job && (
                        <span className="font-mono text-sm font-semibold text-purple-600">{eCtx.job}</span>
                      )}
                      {isCurrent && (
                        <span className="text-xs bg-black text-white px-1.5 py-0.5 font-semibold">
                          CURRENT
                        </span>
                      )}
                    </div>
                    <p className="font-mono text-xs text-black/70 truncate mt-0.5">{e.message}</p>
                  </div>
                  <span className="font-mono text-xs text-black/70 shrink-0">{e.duration_ms}ms</span>
                </a>
              )
            })}
          </div>
        </div>
      )}
    </Layout>
  )
}
