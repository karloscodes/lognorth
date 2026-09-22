import { formatDistanceToNow } from 'date-fns'
import { Clock, Hash, Briefcase, ArrowRight, Layers } from 'lucide-react'
import { Layout } from '@/components/layout'
import { getConfig, statusColor, parseContext } from '@/lib/shared'
import type { Application, LogEntry, LogContext } from '@/lib/shared'

interface Props {
  entry: LogEntry
  entryContext: LogContext
  relatedEntries: LogEntry[]
  appId?: number
  appName?: string
  applications?: Application[]
  issueHash?: string
}

function routeLabel(ctx: LogContext): string | null {
  // Rails: controller + action render as "ConversationsController#index".
  // Go/Node frameworks: usually route pattern + handler function name.
  if (ctx.controller && ctx.action) return `${ctx.controller}#${ctx.action}`
  if (ctx.route && ctx.handler) return `${ctx.route} · ${ctx.handler}`
  return ctx.route || ctx.handler || null
}

export default function LogDetail({ entry, entryContext, relatedEntries, appId, appName, applications, issueHash }: Props) {
  const config = getConfig(entry.is_error)
  const Icon = config.icon
  const logPath = `/apps/${appId}/log`
  const ctx = entryContext || parseContext(entry.context)
  const isHttpRequest = ctx.method && ctx.path
  const isJob = ctx.job

  return (
    <Layout currentPath="/log" appId={appId} appName={appName} applications={applications}>
      {/* Issue Link */}
      {issueHash && appId && (
        <a
          href={`/apps/${appId}/issues/${issueHash}`}
          className="flex items-center justify-between px-4 py-3 mb-4 border-2 border-[#e11d48]/20 bg-[#e11d48]/[0.06] hover:bg-[#e11d48]/10 transition-colors"
        >
          <span className="text-sm text-[#e11d48] font-semibold">This error is a tracked issue</span>
          <span className="text-xs bg-[#e11d48] text-white px-2 py-1 font-bold">View Issue →</span>
        </a>
      )}

      {/* Header Card */}
      <div className="border-2 border-foreground/10 overflow-hidden mb-8">
        <div className="flex items-center gap-4 p-6">
          <div className={`p-3 ${config.colors}`}>
            <Icon className="w-6 h-6" />
          </div>
          <div className="flex-1 min-w-0">
            {isHttpRequest ? (
              <>
                <div className="flex items-center gap-3">
                  <span className={`font-mono text-xl font-bold text-foreground/50`}>
                    {ctx.method}
                  </span>
                  <span className={`text-3xl font-bold ${statusColor(ctx.status)}`}>
                    {ctx.status}
                  </span>
                </div>
                <p className="font-mono text-foreground mt-1 break-all">{ctx.path}</p>
                {routeLabel(ctx) && (
                  <p className="font-mono text-xs text-foreground/60 mt-1">{routeLabel(ctx)}</p>
                )}
              </>
            ) : isJob ? (
              <>
                <div className="flex items-center gap-3">
                  <Briefcase className="w-5 h-5 text-accent" />
                  <span className="font-mono text-xl font-bold text-accent">{ctx.job}</span>
                </div>
                <p className="text-foreground mt-1">{entry.message}</p>
              </>
            ) : (
              <p className="text-xl font-bold text-foreground">{entry.message}</p>
            )}
          </div>
          <div className="text-right">
            <span className="font-mono text-2xl text-foreground/70">{entry.duration_ms}ms</span>
            <p className="text-xs text-foreground/70 mt-1">duration</p>
          </div>
        </div>

        {/* Quick Stats */}
        <div className="border-t border-foreground/10 bg-foreground/[0.01]">
          <div className="flex items-center divide-x divide-foreground/10">
            <div className="px-4 py-3">
              <div className="flex items-center gap-2 text-foreground/70 text-xs mb-1">
                <Clock className="w-3 h-3" />
                When
              </div>
              <p className="text-sm text-foreground/80">
                {formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}
              </p>
            </div>
            {ctx.environment && (
              <div className="px-4 py-3">
                <div className="flex items-center gap-2 text-foreground/70 text-xs mb-1">
                  <Layers className="w-3 h-3" />
                  Environment
                </div>
                <p className="text-sm text-foreground font-mono font-semibold">{ctx.environment}</p>
              </div>
            )}
            {entry.trace_id ? (
              <a
                href={`${logPath}?trace=${entry.trace_id}`}
                className="flex-1 px-4 py-3 hover:bg-foreground/[0.02] transition-colors group"
              >
                <div className="flex items-center gap-2 text-foreground/70 text-xs mb-1">
                  <Hash className="w-3 h-3" />
                  Trace ID
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm font-mono text-foreground">{entry.trace_id}</span>
                  <span className="flex items-center gap-1 text-xs font-semibold text-foreground/50 group-hover:text-foreground transition-colors">
                    View trace <ArrowRight className="w-3 h-3" />
                  </span>
                </div>
              </a>
            ) : (
              <div className="flex-1 px-4 py-3">
                <div className="flex items-center gap-2 text-foreground/70 text-xs mb-1">
                  <Hash className="w-3 h-3" />
                  Trace ID
                </div>
                <p className="text-sm text-foreground/40 font-mono">No trace</p>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Error Block - enhanced with structured info */}
      {ctx.error && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-[#e11d48] uppercase tracking-wider mb-3">Error</h3>
          <div className="bg-[#e11d48]/10 border-2 border-[#e11d48] p-4">
            <pre className="text-sm font-mono whitespace-pre-wrap text-[#e11d48] font-bold break-words">
              {ctx.error}
            </pre>
            {/* Show error class and location if available */}
            {(ctx.error_class || ctx.error_file) && (
              <div className="mt-3 pt-3 border-t border-[#e11d48]/20 text-sm font-mono">
                {ctx.error_class && (
                  <span className="text-[#e11d48] font-semibold">{ctx.error_class}</span>
                )}
                {ctx.error_file && (
                  <span className="text-[#e11d48]/70">
                    {ctx.error_class ? ' at ' : ''}{ctx.error_file}{ctx.error_line ? `:${ctx.error_line}` : ''}
                  </span>
                )}
                {ctx.error_caller && (
                  <span className="text-[#e11d48]/50"> in {ctx.error_caller}</span>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Trace Log - what happened before/after in this request */}
      {relatedEntries.length > 1 && (() => {
        const isTrace = entry.trace_id && relatedEntries.every((e: LogEntry) => e.trace_id === entry.trace_id)
        const isRequest = (e: LogEntry) => {
          const c = parseContext(e.context)
          return c.method && c.status && c.path
        }

        // Separate the request wrapper from child events
        const requestEntry = isTrace ? relatedEntries.find(e => isRequest(e)) : null
        const childEntries = requestEntry
          ? relatedEntries.filter(e => e.id !== requestEntry.id)
          : relatedEntries.filter(e => e.id !== entry.id)

        const requestCtx = requestEntry ? parseContext(requestEntry.context) : null
        const requestConfig = requestEntry ? getConfig(requestEntry.is_error) : null

        return (
          <div className="mb-8">
            <h3 className="text-sm font-black text-foreground uppercase tracking-wider mb-3">
              {isTrace ? 'Request Context' : 'Nearby Logs'}
            </h3>
            <div className="border-2 border-foreground/10 overflow-hidden font-mono text-xs">
              {/* Request wrapper header */}
              {requestEntry && requestCtx && requestConfig && (() => {
                const ReqIcon = requestConfig.icon
                const isCurrent = requestEntry.id === entry.id
                return (
                  <a
                    href={isCurrent ? undefined : `${logPath}/${requestEntry.id}`}
                    className={`flex items-center gap-2 px-4 py-2.5 bg-foreground/[0.02] border-b border-foreground/10 ${isCurrent ? 'font-bold' : 'hover:bg-foreground/[0.04] transition-colors'}`}
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
                const isCurrent = e.id === entry.id
                const eConfig = getConfig(e.is_error)
                const EIcon = eConfig.icon
                const eCtx = isCurrent ? ctx : parseContext(e.context)
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

              {isTrace && entry.trace_id && (
                <a
                  href={`${logPath}?trace=${entry.trace_id}`}
                  className="flex items-center justify-between px-4 py-2.5 bg-foreground/[0.02] border-t border-foreground/10 font-mono text-xs font-bold text-foreground/60 hover:text-foreground transition-colors"
                >
                  <span>{relatedEntries.length} entries in this trace</span>
                  <span>View full trace →</span>
                </a>
              )}
            </div>
          </div>
        )
      })()}

      {/* Stack Trace - always visible for errors, no collapse */}
      {ctx.stack_trace && (
        <div className="mb-8">
          <h3 className="text-sm font-black text-[#e11d48] uppercase tracking-wider mb-3">Stack Trace</h3>
          <pre className="p-4 text-xs font-mono text-foreground/80 overflow-x-auto whitespace-pre bg-foreground/[0.03] border border-foreground/10">
            {ctx.stack_trace}
          </pre>
        </div>
      )}

      {/* User Agent */}
      {ctx.user_agent && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-foreground/70 uppercase tracking-wider mb-3">User Agent</h3>
          <p className="text-sm text-foreground/70 font-mono break-all bg-foreground/[0.02] p-3">
            {ctx.user_agent}
          </p>
        </div>
      )}

      {/* Extra Context */}
      {ctx.extra && Object.keys(ctx.extra).length > 0 && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-foreground/70 uppercase tracking-wider mb-3">Context</h3>
          <div className="border-2 border-foreground/10 divide-y divide-foreground/5">
            {Object.entries(ctx.extra).map(([key, value]) => (
              <div key={key} className="flex gap-4 px-4 py-2.5 text-sm">
                <span className="text-foreground/70 w-32 shrink-0">{key}</span>
                <span className="font-mono text-foreground/80">{String(value)}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Headers */}
      {ctx.headers && Object.keys(ctx.headers).length > 0 && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-foreground/70 uppercase tracking-wider mb-3">Headers</h3>
          <div className="border-2 border-foreground/10 divide-y divide-foreground/5">
            {Object.entries(ctx.headers).map(([key, value]) => (
              <div key={key} className="flex gap-4 px-4 py-2.5 text-sm">
                <span className="text-foreground/70 font-mono w-48 shrink-0">{key}</span>
                <span className="font-mono text-foreground/80 break-all">{value}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Body */}
      {ctx.body && (
        <div className="mb-8">
          <h3 className="text-sm font-semibold text-foreground/70 uppercase tracking-wider mb-3">Body</h3>
          <pre className="font-mono text-sm bg-foreground text-background p-4 overflow-x-auto">
            {ctx.body}
          </pre>
        </div>
      )}

    </Layout>
  )
}
