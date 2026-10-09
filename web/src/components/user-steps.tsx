import { Mono, parseContext, statusColor, type LogEntry } from '@/lib/shared'

// "−2m 10s" before the error.
function before(stepISO: string, untilISO: string): string {
  const s = Math.max(0, Math.round((new Date(untilISO).getTime() - new Date(stepISO).getTime()) / 1000))
  if (s === 0) return '0s'
  if (s < 60) return `−${s}s`
  return `−${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`
}

// What one user did before an error: their requests in the 30 minutes up to
// it, oldest first. The same user ID ties them, so they read as the steps to
// reproduce it.
export function UserSteps({ steps, user, until, logPath }: { steps?: LogEntry[] | null; user: string; until: string; logPath: string }) {
  if (!steps?.length) return null
  return (
    <div className="pane mb-7 mt-7 pb-1.5 pt-3.5 text-[13px]">
      <span className="pane-title">what user {user} did before</span>
      {steps.map((step) => {
        const ctx = parseContext(step.context)
        return (
          <a
            key={step.id}
            href={`${logPath}/${step.id}`}
            className="grid grid-cols-[9ch_6ch_1fr_5ch] gap-3.5 whitespace-nowrap border-t border-line px-[18px] py-1.5 first-of-type:border-t-0 hover:bg-raise"
          >
            <Mono className="text-dim">{before(step.timestamp, until)}</Mono>
            <span className="text-dim">{ctx.method}</span>
            <span className="truncate text-foreground">{ctx.path}</span>
            <span className={`text-right ${ctx.status ? statusColor(ctx.status) : 'text-dim'}`}>{ctx.status}</span>
          </a>
        )
      })}
    </div>
  )
}
