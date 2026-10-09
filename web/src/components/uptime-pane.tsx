import { useState } from 'react'
import { ReleaseMarks, type Release } from '@/components/release-marks'
import { format } from 'date-fns'
import { uptimeLabel, percent, shortDuration, failureText, type UptimeReport, type UptimeCheck, type UptimeBar } from '@/lib/uptime'

// "503 · 1,204ms" or "no answer within 5s".
function checkSummary(c: UptimeCheck): string {
  if (c.status > 0) return `${c.status} · ${c.latency_ms.toLocaleString()}ms`
  return c.error || 'no answer'
}

// The app's URL ping over the last 24 hours: one bar per 15 minutes, red where
// a ping failed, plus the latest result and the latest failure. A red bar
// opens the app's errors in its 15 minutes.
export function UptimePane({ report, logPath, releases }: { report: UptimeReport; logPath: string; releases?: Release[] }) {
  const label = uptimeLabel(report)
  const [hovered, setHovered] = useState<number | null>(null)

  if (report.status === 'none' || report.status === 'off') {
    return (
      <div className="pane mb-7 px-4 pb-4 pt-6 text-[13px]">
        <span className="pane-title">uptime</span>
        <p className="text-subtle">
          {report.status === 'none' ? 'No URL yet. Add one and LogNorth pings it every minute.' : 'Uptime checks are off for this app.'}{' '}
          <a href="/apps" className="text-foreground hover:underline">
            {report.status === 'none' ? 'add a URL →' : 'turn them on →'}
          </a>
        </p>
      </div>
    )
  }

  const hasChecks = report.bars.some((b) => b.checks > 0)
  const barMs = report.bars.length > 1 ? new Date(report.bars[1].start).getTime() - new Date(report.bars[0].start).getTime() : 0

  return (
    <div className="pane mb-7 px-4 pb-4 pt-6 text-[13px]">
      <span className="pane-title max-w-[60%] truncate">uptime · {report.url}</span>
      {report.uptime_24h !== undefined && (
        <span className="pane-right">
          <span className="text-foreground">{percent(report.uptime_24h)}</span> · 24h
        </span>
      )}

      <div className="relative" onMouseLeave={() => setHovered(null)}>
        {hovered !== null && <BarTooltip bar={report.bars[hovered]} index={hovered} total={report.bars.length} barMs={barMs} />}
        <div className="relative flex h-7 items-stretch gap-px">
          {report.bars.length > 0 && (
            <ReleaseMarks
              releases={releases}
              from={new Date(report.bars[0].start).getTime()}
              to={new Date(report.bars[0].start).getTime() + report.bars.length * barMs}
            />
          )}
          {report.bars.map((bar, i) => {
            const label = `${format(new Date(bar.start), 'HH:mm')}: ${bar.checks} pings, ${bar.fails} failed`
            if (bar.fails > 0) {
              return (
                <a
                  key={i}
                  href={errorsIn(logPath, bar, barMs)}
                  onMouseEnter={() => setHovered(i)}
                  className={`flex-1 ${hovered === i ? 'bg-error' : 'bg-error/80'}`}
                  aria-label={`${label}. Open the errors of these minutes`}
                />
              )
            }
            return (
              <i
                key={i}
                onMouseEnter={() => setHovered(i)}
                className={`flex-1 cursor-crosshair ${bar.checks === 0 ? 'bg-line' : hovered === i ? 'bg-foreground' : 'bg-dim'}`}
                aria-label={label}
              />
            )
          })}
        </div>
      </div>
      <div className="mt-1.5 flex justify-between text-[10px] text-dim">
        <span>24h ago</span>
        <span>12h</span>
        <span>now</span>
      </div>

      <p className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1">
        <i className={`h-[7px] w-[7px] ${label.dot}`} />
        <span className={label.bad ? 'font-bold text-error' : 'text-foreground'}>{label.text}</span>
        {report.status === 'down' && report.down_since && (
          <span className="text-dim">· since {format(new Date(report.down_since), 'MMM d HH:mm').toLowerCase()}</span>
        )}
        {report.checked_at && (
          <span className="text-dim">
            · last ping {shortDuration(report.checked_at)} ago
          </span>
        )}
        {report.status === 'pending' && <span className="text-dim">· the first ping runs within a minute</span>}
        {!hasChecks && report.status !== 'pending' && <span className="text-dim">· no pings in the last 24h</span>}
      </p>

      {report.last_failure && (
        <p className="mt-1 text-dim">
          last failure <span className="text-subtle">{format(new Date(report.last_failure.checked_at), 'MMM d HH:mm').toLowerCase()}</span> ·{' '}
          <span className="text-error">{checkSummary(report.last_failure)}</span>
        </p>
      )}
    </div>
  )
}

// The app's errors logged during one bar.
function errorsIn(logPath: string, bar: UptimeBar, barMs: number): string {
  const start = new Date(bar.start)
  const end = new Date(start.getTime() + barMs)
  const params = new URLSearchParams({ is_error: 'true', start_time: start.toISOString(), end_time: end.toISOString() })
  return `${logPath}?${params}`
}

// What happened in one bar: shown above the strip while the mouse is on it.
function BarTooltip({ bar, index, total, barMs }: { bar: UptimeBar; index: number; total: number; barMs: number }) {
  const start = new Date(bar.start)
  const end = new Date(start.getTime() + barMs)
  // Near the edges the box grows inward instead of centering on the bar.
  const pos = index < 8 ? { left: 0 } : index > total - 9 ? { right: 0 } : { left: `${((index + 0.5) / total) * 100}%`, transform: 'translateX(-50%)' }

  return (
    <div className="pane absolute bottom-[calc(100%+10px)] z-10 w-max max-w-[340px] bg-background px-3 pb-2 pt-3.5 text-xs" style={pos}>
      <span className="pane-title text-foreground">
        {format(start, 'HH:mm')}–{format(end, 'HH:mm')}
      </span>
      {bar.checks === 0 ? (
        <p className="text-dim">no pings</p>
      ) : (
        <>
          <p className="text-subtle">
            {bar.checks} ping{bar.checks !== 1 ? 's' : ''}
            {bar.fails > 0 ? <span className="font-bold text-error"> · {bar.fails} failed</span> : ' · all up'}
            {bar.latency_ms ? ` · avg ${bar.latency_ms}ms` : ''}
          </p>
          {bar.failures?.map((f) => (
            <p key={f.label} className="text-error">
              {failureText(f.label)} <span className="text-dim">×{f.count}</span>
            </p>
          ))}
          {bar.fails > 0 && <p className="mt-1 text-dim">click for the errors of these minutes</p>}
        </>
      )}
    </div>
  )
}
