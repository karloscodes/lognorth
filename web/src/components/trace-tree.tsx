import { getConfig, parseContext, statusColor } from '@/lib/shared'
import type { LogEntry } from '@/lib/shared'

interface TraceTreeProps {
  title: string
  entries: LogEntry[]
  currentId: number
  logPath: string
  // grouped: the entries share one trace, so the HTTP request becomes the
  // header and the rest nest under it, with a link to the full trace.
  grouped: boolean
  traceId?: string
}

const isRequest = (e: LogEntry) => {
  const c = parseContext(e.context)
  return !!(c.method && c.status && c.path)
}

// TraceTree shows the entries around one log entry: the request it belongs
// to, and what else happened in that request.
export function TraceTree({ title, entries, currentId, logPath, grouped, traceId }: TraceTreeProps) {
  const requestEntry = grouped ? entries.find(isRequest) : undefined
  const childEntries = requestEntry
    ? entries.filter(e => e.id !== requestEntry.id)
    : entries.filter(e => e.id !== currentId)

  return (
    <div className="mb-8">
      <h3 className="text-sm font-black text-foreground uppercase tracking-wider mb-4">{title}</h3>
      <div className="border-2 border-foreground/10 overflow-hidden font-mono text-xs">
        {requestEntry && <RequestHeader entry={requestEntry} isCurrent={requestEntry.id === currentId} logPath={logPath} />}

        {childEntries.map((e, i) => {
          const isCurrent = e.id === currentId
          const borderClass = i < childEntries.length - 1 ? 'border-b border-foreground/5' : ''
          const indent = requestEntry ? 'pl-8' : 'pl-4'

          if (isCurrent) {
            return (
              <div key={e.id} className={`relative flex items-center gap-2 ${indent} pr-4 py-2.5 bg-foreground/[0.04] ${borderClass}`}>
                <div className="absolute left-0 top-0 bottom-0 w-1 bg-foreground" />
                <ChildRow entry={e} isCurrent />
              </div>
            )
          }
          return (
            <a
              key={e.id}
              href={`${logPath}/${e.id}`}
              className={`flex items-center gap-2 ${indent} pr-4 py-2.5 hover:bg-foreground/[0.02] transition-colors ${borderClass}`}
            >
              <ChildRow entry={e} isCurrent={false} />
            </a>
          )
        })}

        {grouped && traceId && (
          <a
            href={`${logPath}?trace=${traceId}`}
            className="flex items-center justify-between px-4 py-2.5 border-t border-foreground/10 font-mono text-xs font-bold text-foreground/60 hover:text-foreground transition-colors"
          >
            <span>{entries.length} {entries.length === 1 ? 'entry' : 'entries'} in this trace</span>
            <span>View full trace →</span>
          </a>
        )}
      </div>
    </div>
  )
}

function RequestHeader({ entry, isCurrent, logPath }: { entry: LogEntry; isCurrent: boolean; logPath: string }) {
  const ctx = parseContext(entry.context)
  const config = getConfig(entry.is_error)
  const Icon = config.icon

  return (
    <a
      href={isCurrent ? undefined : `${logPath}/${entry.id}`}
      className={`flex items-center gap-2 px-4 py-2.5 bg-foreground/[0.02] border-b border-foreground/10 ${isCurrent ? 'font-bold' : 'hover:bg-foreground/[0.04] transition-colors'}`}
    >
      <div className={`p-1 ${config.colors}`}>
        <Icon className="w-3 h-3" />
      </div>
      {ctx.method && <span className="font-bold text-foreground/50">{ctx.method}</span>}
      <span className="text-foreground/70">{ctx.path}</span>
      {ctx.status && <span className={`font-bold ${statusColor(ctx.status)}`}>{ctx.status}</span>}
      <span className="text-foreground/40 ml-auto shrink-0">{entry.duration_ms}ms</span>
    </a>
  )
}

function ChildRow({ entry, isCurrent }: { entry: LogEntry; isCurrent: boolean }) {
  const ctx = parseContext(entry.context)
  const config = getConfig(entry.is_error)
  const Icon = config.icon

  return (
    <>
      <div className={`p-1 ${config.colors}`}>
        <Icon className="w-3 h-3" />
      </div>
      <span className={`truncate ${isCurrent ? 'text-foreground font-bold' : 'text-foreground/70'}`}>{ctx.path || entry.message}</span>
      {ctx.error_file && <span className="text-foreground/40">{ctx.error_file}{ctx.error_line ? `:${ctx.error_line}` : ''}</span>}
      {ctx.method && <span className="font-bold text-foreground/50">{ctx.method}</span>}
      {ctx.status && <span className={`font-bold ${statusColor(ctx.status)}`}>{ctx.status}</span>}
      {entry.duration_ms > 0 && <span className="text-foreground/40 ml-auto shrink-0">{entry.duration_ms}ms</span>}
    </>
  )
}
