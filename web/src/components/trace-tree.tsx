import { parseContext, statusColor } from '@/lib/shared'
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
  const childEntries = requestEntry ? entries.filter((e) => e.id !== requestEntry.id) : entries.filter((e) => e.id !== currentId)

  return (
    <div className="pane mb-8 pb-1.5 pt-3.5 text-[13px]">
      <span className="pane-title">{title.toLowerCase()}</span>
      {grouped && traceId && (
        <a href={`${logPath}?trace=${traceId}`} className="pane-right hover:text-foreground">
          {entries.length} {entries.length === 1 ? 'request' : 'lines'} · full trace →
        </a>
      )}
      {requestEntry && <Row entry={requestEntry} isCurrent={requestEntry.id === currentId} logPath={logPath} />}
      {childEntries.map((e) => (
        <Row key={e.id} entry={e} isCurrent={e.id === currentId} logPath={logPath} child={Boolean(requestEntry)} />
      ))}
    </div>
  )
}

function Row({ entry, isCurrent, logPath, child }: { entry: LogEntry; isCurrent: boolean; logPath: string; child?: boolean }) {
  const ctx = parseContext(entry.context)
  const label = ctx.method && ctx.path ? ctx.path : ctx.job || entry.message
  const body = (
    <>
      <span className="text-dim">{ctx.job ? 'JOB' : ctx.method}</span>
      <span className={`relative truncate ${child ? 'pl-[3ch]' : ''} ${entry.is_error ? 'font-bold text-error' : child ? 'text-subtle' : 'text-foreground'}`}>
        {child && <i className="absolute left-[1ch] top-[-0.8em] bottom-1/2 w-[1.2ch] border-b border-l border-border" />}
        {label}
        {ctx.error_file && (
          <span className="ml-2 font-normal text-dim">
            {ctx.error_file}
            {ctx.error_line ? `:${ctx.error_line}` : ''}
          </span>
        )}
      </span>
      <span className={`text-right ${ctx.status ? statusColor(ctx.status) : ''}`}>{ctx.status}</span>
      <span className="text-right text-subtle">{entry.duration_ms > 0 ? `${entry.duration_ms}ms` : ''}</span>
    </>
  )
  const cls = `grid grid-cols-[5ch_1fr_5ch_9ch] gap-3.5 whitespace-nowrap px-[18px] py-1 ${
    entry.is_error ? 'shadow-[inset_3px_0_0_0_hsl(var(--error))]' : ''
  } ${isCurrent ? 'bg-raise' : 'hover:bg-raise'}`

  return isCurrent ? (
    <div className={cls}>{body}</div>
  ) : (
    <a href={`${logPath}/${entry.id}`} className={cls}>
      {body}
    </a>
  )
}
