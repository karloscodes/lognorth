import { formatDistanceToNowStrict } from 'date-fns'
import { Link } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { Mono } from '@/lib/shared'
import type { Application, Issue } from '@/lib/shared'
import { uptimeLabel, percent } from '@/lib/uptime'

interface Props {
  applications?: Application[]
  recentIssues?: Issue[]
}

const APP_COLS = 'grid grid-cols-[1fr_7ch_5ch_11ch_9ch_22ch] gap-4 max-md:grid-cols-[1fr_7ch]'

// Uptime at a glance: "up 84ms · 99.98%" or "down 12m".
function PingCell({ app }: { app: Application }) {
  const u = app.uptime
  if (!u || u.status === 'none') return <span className="text-dim max-md:hidden">—</span>
  const label = uptimeLabel(u)
  return (
    <span className={`flex items-center gap-1.5 truncate max-md:hidden ${label.bad ? 'font-bold text-error' : 'text-subtle'}`} title={u.url}>
      <i className={`h-[7px] w-[7px] shrink-0 ${label.dot}`} />
      {label.text}
      {u.uptime_24h !== undefined && <span className="text-dim"> · {percent(u.uptime_24h)}</span>}
    </span>
  )
}

export default function Home({ applications = [], recentIssues = [] }: Props) {
  // No apps yet: onboarding.
  if (applications.length === 0) {
    return (
      <Layout currentPath="/">
        <div className="max-w-md py-20">
          <p className="text-xs text-dim">~ $ lognorth</p>
          <h1 className="mt-3 text-3xl font-bold tracking-tight text-foreground">Know when things break.</h1>
          <p className="mb-7 mt-3 text-text">Your errors. Your server. Your data.</p>
          <a href="/apps" className="inline-flex bg-accent px-5 py-2.5 text-sm font-bold text-accent-foreground hover:bg-foreground">
            Add your first app
          </a>
        </div>
      </Layout>
    )
  }

  // The dashboard's active window is 7 days, so apps that fired earlier this
  // week still ask for attention without painting stale issues red.
  const activeCutoff = Date.now() - 7 * 24 * 60 * 60 * 1000
  const isActive = (issue: Issue) => !issue.muted && new Date(issue.last_seen).getTime() > activeCutoff

  const active = recentIssues.filter(isActive)
  const totalAll = recentIssues.length

  const appStats = new Map<number, { active: number; total: number; unseen: number }>()
  for (const issue of recentIssues) {
    const s = appStats.get(issue.api_key_id) || { active: 0, total: 0, unseen: 0 }
    s.total++
    if (isActive(issue)) {
      s.active++
      if (issue.unseen) s.unseen++
    }
    appStats.set(issue.api_key_id, s)
  }
  const statsOf = (id: number) => appStats.get(id) || { active: 0, total: 0, unseen: 0 }
  const appName = new Map(applications.map((a) => [a.id, a.name]))

  // Apps with active issues first, most unseen first.
  const apps = [...applications].sort((a, b) => {
    const sa = statsOf(a.id)
    const sb = statsOf(b.id)
    return sb.active - sa.active || sb.unseen - sa.unseen
  })

  return (
    <Layout currentPath="/" applications={applications}>
      <div className="flex flex-wrap items-baseline gap-x-5 gap-y-2">
        {active.length > 0 ? (
          <>
            <Mono className="text-[clamp(72px,12vw,120px)] font-bold leading-[0.9] tracking-[-0.05em] text-error">{active.length}</Mono>
            <span className="text-[22px] font-bold text-foreground">
              active issue{active.length !== 1 ? 's' : ''}
              <span className="block text-[13px] font-normal text-subtle">
                of <Mono>{totalAll}</Mono> total · <Mono>{applications.length}</Mono> app{applications.length !== 1 ? 's' : ''}
              </span>
            </span>
          </>
        ) : (
          <span className="flex items-center gap-3 text-[22px] font-bold text-foreground">
            <i className="h-3 w-3 bg-accent" />
            All clear
            {totalAll > 0 && (
              <span className="text-[13px] font-normal text-subtle">
                · <Mono>{totalAll}</Mono> stale
              </span>
            )}
          </span>
        )}
      </div>

      <div className="pane mt-9 pb-1.5 pt-3.5">
        <span className="pane-title">apps</span>
        <div className={`${APP_COLS} border-b border-line px-[18px] pb-2 text-[11px] text-dim`}>
          <span>name</span>
          <span className="text-right">active</span>
          <span className="text-right max-md:hidden">new</span>
          <span className="text-right max-md:hidden">requests</span>
          <span className="text-right max-md:hidden">errors</span>
          <span className="max-md:hidden">ping</span>
        </div>
        {apps.map((app) => {
          const s = statsOf(app.id)
          return (
            <Link
              key={app.id}
              href={`/apps/${app.id}/issues`}
              className={`${APP_COLS} items-center border-t border-line px-[18px] py-2.5 text-[13px] first-of-type:border-t-0 hover:bg-raise ${
                s.active > 0 ? 'shadow-[inset_3px_0_0_0_hsl(var(--error))]' : ''
              }`}
            >
              <span className="flex items-center gap-2.5 font-bold text-foreground">
                {s.active === 0 && <i className="h-[7px] w-[7px] bg-accent" />}
                {app.name}
              </span>
              <Mono className={`text-right ${s.active > 0 ? 'font-bold text-error' : 'text-dim'}`}>{s.active || '—'}</Mono>
              <Mono className="text-right text-foreground max-md:hidden">{s.unseen || ''}</Mono>
              <Mono className="text-right text-subtle max-md:hidden">{app.stats ? app.stats.total.toLocaleString() : ''}</Mono>
              <Mono className="text-right text-subtle max-md:hidden">{app.stats ? app.stats.errors.toLocaleString() : ''}</Mono>
              <PingCell app={app} />
            </Link>
          )
        })}
        <a href="/apps" className="block border-t border-dashed border-border px-[18px] py-2.5 text-[13px] text-subtle hover:text-foreground">
          + add app
        </a>
      </div>

      {active.length > 0 && (
        <>
          <p className="rule-label mb-3 mt-9">
            <b>latest</b> across all apps
          </p>
          <div className="border border-border">
            {active.slice(0, 5).map((issue) => (
              <a
                key={issue.hash}
                href={`/apps/${issue.api_key_id}/issues/${issue.hash}`}
                className={`grid grid-cols-[1fr_auto] gap-5 border-l-[3px] border-t border-t-line py-3 pl-4 pr-5 first:border-t-0 hover:bg-raise ${
                  issue.unseen ? 'border-l-error' : 'border-l-transparent'
                }`}
              >
                <span className="min-w-0">
                  <span className="block text-xs text-error">{issue.error_class || 'Error'}</span>
                  <span className={`mt-0.5 block truncate text-[15px] ${issue.unseen ? 'font-medium text-foreground' : 'text-text'}`}>{issue.error}</span>
                  <span className="mt-1 block truncate text-xs text-dim">
                    {appName.get(issue.api_key_id)} · last {formatDistanceToNowStrict(new Date(issue.last_seen))} ago
                    {issue.paths?.[0] && <> · {issue.paths[0]}</>}
                  </span>
                </span>
                <Mono className={`self-center text-xl font-bold ${issue.unseen ? 'text-error' : 'text-foreground'}`}>{issue.count}×</Mono>
              </a>
            ))}
          </div>
        </>
      )}
    </Layout>
  )
}
