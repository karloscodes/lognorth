import { ReactNode, useEffect } from 'react'
import { formatDistanceToNow } from 'date-fns'
import { router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { TraceTree } from '@/components/trace-tree'
import { StackTrace } from '@/components/stack-trace'
import { statusColor, parseContext } from '@/lib/shared'
import type { Application, LogEntry, LogContext } from '@/lib/shared'
import { pageKey } from '@/lib/keys'

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

// A bordered panel with its title in the top border.
function Pane({ title, children, className = '' }: { title: string; children: ReactNode; className?: string }) {
  return (
    <div className={`pane mb-7 px-4 pb-4 pt-6 text-[13px] ${className}`}>
      <span className="pane-title">{title}</span>
      {children}
    </div>
  )
}

function KeyValues({ pairs, keyWidth }: { pairs: [string, ReactNode][]; keyWidth: string }) {
  return (
    <div className="grid gap-x-5 gap-y-1.5" style={{ gridTemplateColumns: `${keyWidth} 1fr` }}>
      {pairs.map(([k, v]) => (
        <div key={k} className="contents">
          <span className="truncate text-dim">{k}</span>
          <span className="break-all text-text">{v}</span>
        </div>
      ))}
    </div>
  )
}

export default function LogDetail({ entry, entryContext, relatedEntries, appId, appName, applications, issueHash }: Props) {
  const logPath = `/apps/${appId}/log`
  const ctx = entryContext || parseContext(entry.context)
  const isHttpRequest = ctx.method && ctx.path
  const route = routeLabel(ctx)

  // esc goes back to the logs.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (pageKey(e) && e.key === 'Escape') router.visit(logPath)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [logPath])

  const isTrace = !!entry.trace_id && relatedEntries.every((e: LogEntry) => e.trace_id === entry.trace_id)

  return (
    <Layout currentPath="/log" appId={appId} appName={appName} applications={applications}>
      <p className="text-xs text-dim">
        <a href={logPath} className="hover:text-foreground">
          logs
        </a>{' '}
        / {entry.id}
        <span className="ml-3">esc back</span>
      </p>

      {issueHash && appId && (
        <a
          href={`/apps/${appId}/issues/${issueHash}`}
          className="mt-4 flex items-center justify-between border border-error px-4 py-2.5 text-[13px] hover:bg-raise"
        >
          <span className="text-error">This error is a tracked issue.</span>
          <span className="font-bold text-foreground">open issue →</span>
        </a>
      )}

      <div className="mt-5 grid gap-6 md:grid-cols-[1fr_auto] md:items-end">
        <div className="min-w-0">
          {isHttpRequest ? (
            <>
              <p className="flex items-baseline gap-3">
                <span className="text-xl font-bold text-dim">{ctx.method}</span>
                <span className={`text-3xl font-bold ${statusColor(ctx.status)}`}>{ctx.status}</span>
              </p>
              <p className="mt-1 break-all text-[21px] font-bold text-foreground">{ctx.path}</p>
              {route && <p className="mt-1 text-xs text-subtle">{route}</p>}
            </>
          ) : ctx.job ? (
            <>
              <p className="flex items-center gap-2 text-xl font-bold text-foreground">
                <span className="border border-border px-1.5 text-[11px] font-normal text-subtle">JOB</span>
                {ctx.job}
              </p>
              <p className="mt-1 text-text">{entry.message}</p>
            </>
          ) : (
            <p className={`text-[21px] font-bold ${entry.is_error ? 'text-error' : 'text-foreground'}`}>{entry.message}</p>
          )}
        </div>
        <p className="text-right">
          <span className="text-2xl tabular-nums text-foreground">{entry.duration_ms}ms</span>
          <span className="block text-xs text-dim">took</span>
        </p>
      </div>

      <div className="mb-7 mt-6 grid grid-cols-2 border border-border md:grid-cols-[auto_auto_1fr] [&>*]:border-l [&>*]:border-line [&>*:first-child]:border-l-0">
        <div className="px-[18px] py-3">
          <p className="text-xs text-dim">when</p>
          <p className="mt-0.5 text-[13px] text-foreground">{formatDistanceToNow(new Date(entry.timestamp), { addSuffix: true })}</p>
        </div>
        {ctx.environment && (
          <div className="px-[18px] py-3">
            <p className="text-xs text-dim">environment</p>
            <p className="mt-0.5 text-[13px] text-foreground">{ctx.environment}</p>
          </div>
        )}
        {entry.trace_id ? (
          <a href={`${logPath}?trace=${entry.trace_id}`} className="px-[18px] py-3 hover:bg-raise max-md:col-span-2">
            <p className="text-xs text-dim">trace</p>
            <p className="mt-0.5 flex justify-between gap-4 text-[13px]">
              <span className="truncate text-foreground">{entry.trace_id}</span>
              <span className="shrink-0 text-subtle">view trace →</span>
            </p>
          </a>
        ) : (
          <div className="px-[18px] py-3">
            <p className="text-xs text-dim">trace</p>
            <p className="mt-0.5 text-[13px] text-dim">none</p>
          </div>
        )}
      </div>

      {ctx.error && (
        <Pane title="error" className="border-error">
          <pre className="whitespace-pre-wrap break-words font-bold text-error">{ctx.error}</pre>
          {(ctx.error_class || ctx.error_file) && (
            <p className="mt-3 border-t border-dashed border-line pt-3 text-dim">
              {ctx.error_class && <span className="font-bold text-foreground">{ctx.error_class}</span>}
              {ctx.error_file && (
                <>
                  {ctx.error_class ? ' at ' : ''}
                  <span className="text-subtle">
                    {ctx.error_file}
                    {ctx.error_line ? `:${ctx.error_line}` : ''}
                  </span>
                </>
              )}
              {ctx.error_caller && (
                <>
                  {' '}in <span className="text-subtle">{ctx.error_caller}</span>
                </>
              )}
            </p>
          )}
        </Pane>
      )}

      {relatedEntries.length > 1 && (
        <TraceTree
          title={isTrace ? 'Request Context' : 'Nearby Logs'}
          entries={relatedEntries}
          currentId={entry.id}
          logPath={logPath}
          grouped={isTrace}
          traceId={entry.trace_id}
        />
      )}

      {ctx.stack_trace && (
        <div className="mb-7 [&>.pane]:mt-0">
          <StackTrace trace={ctx.stack_trace} errorFile={ctx.error_file} />
        </div>
      )}

      {ctx.user_agent && (
        <Pane title="user agent">
          <p className="break-all text-text">{ctx.user_agent}</p>
        </Pane>
      )}

      {ctx.extra && Object.keys(ctx.extra).length > 0 && (
        <Pane title="context">
          <KeyValues keyWidth="16ch" pairs={Object.entries(ctx.extra).map(([k, v]) => [k, String(v)])} />
        </Pane>
      )}

      {ctx.headers && Object.keys(ctx.headers).length > 0 && (
        <Pane title="headers">
          <KeyValues keyWidth="24ch" pairs={Object.entries(ctx.headers).map(([k, v]) => [k, v])} />
        </Pane>
      )}

      {ctx.body && (
        <Pane title="body">
          <pre className="overflow-x-auto text-text">{ctx.body}</pre>
        </Pane>
      )}
    </Layout>
  )
}
