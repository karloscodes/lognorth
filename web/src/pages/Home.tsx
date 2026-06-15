import { TrendingUp } from 'lucide-react'
import { Layout } from '@/components/layout'
import { Mono } from '@/lib/shared'
import type { Application, Issue } from '@/lib/shared'

interface Props {
  applications?: Application[]
  recentIssues?: Issue[]
}

export default function Home({ applications = [], recentIssues = [] }: Props) {
  // No apps yet - onboarding
  if (applications.length === 0) {
    return (
      <Layout currentPath="/">
        <div className="py-20 max-w-md">
          <h1 className="text-3xl font-bold text-foreground mb-3">
            Know when things break.
          </h1>
          <p className="text-foreground/60 mb-6">
            Your errors. Your server. Your data.
          </p>
          <a
            href="/apps"
            className="inline-flex px-5 py-2.5 bg-foreground text-background font-semibold hover:bg-foreground/80 transition-colors"
          >
            Add your first app
          </a>
        </div>
      </Layout>
    )
  }

  // Active window for dashboard urgency.
  // The Issues page keeps its existing 24h "Active" lens; the dashboard uses
  // a wider 7-day window so apps that only fired earlier this week still
  // show as needing attention without painting the page red for stale issues.
  const ACTIVE_WINDOW_DAYS = 7
  const activeCutoff = Date.now() - ACTIVE_WINDOW_DAYS * 24 * 60 * 60 * 1000
  const isActive = (issue: Issue) =>
    !issue.muted && new Date(issue.last_seen).getTime() > activeCutoff

  const totalActive = recentIssues.filter(isActive).length
  const totalAll = recentIssues.length

  // Aggregate per-app stats. Active drives urgency; total shown only as
  // secondary context when there are stale issues.
  const appStats = new Map<number, { active: number; total: number; unseenCount: number; trending: boolean }>()
  for (const issue of recentIssues) {
    const current = appStats.get(issue.api_key_id) || { active: 0, total: 0, unseenCount: 0, trending: false }
    current.total++
    if (isActive(issue)) {
      current.active++
      if (issue.unseen) current.unseenCount++
      if (issue.trend === 'up') current.trending = true
    }
    appStats.set(issue.api_key_id, current)
  }

  // Red urgency only when an app has currently-active issues, regardless of
  // total historical count.
  const appsWithIssues = applications.filter(app => (appStats.get(app.id)?.active ?? 0) > 0)
  const healthyApps = applications.filter(app => (appStats.get(app.id)?.active ?? 0) === 0)

  // Sort: trending first, then by unseen count, then by active count
  appsWithIssues.sort((a, b) => {
    const statsA = appStats.get(a.id)!
    const statsB = appStats.get(b.id)!
    if (statsA.trending !== statsB.trending) return statsA.trending ? -1 : 1
    if (statsA.unseenCount !== statsB.unseenCount) return statsB.unseenCount - statsA.unseenCount
    return statsB.active - statsA.active
  })

  return (
    <Layout currentPath="/" applications={applications}>
      {/* Status header */}
      <div className="flex items-center justify-between mb-8">
        {totalActive > 0 ? (
          <div className="flex items-baseline gap-2">
            <Mono className="text-5xl font-black text-[#e11d48]">{totalActive}</Mono>
            <span className="text-xl font-semibold text-foreground/60">
              active issue{totalActive !== 1 ? 's' : ''}
            </span>
            {totalAll > totalActive && (
              <span className="ml-3 text-sm text-foreground/40">
                of <Mono>{totalAll}</Mono> total
              </span>
            )}
          </div>
        ) : (
          <div className="flex items-center gap-3">
            <span className="w-3 h-3 bg-accent" />
            <span className="text-xl font-semibold text-foreground/60">All clear</span>
            {totalAll > 0 && (
              <span className="text-sm text-foreground/40">
                · <Mono>{totalAll}</Mono> stale
              </span>
            )}
          </div>
        )}
        <p className="text-sm text-foreground/40">
          <Mono>{applications.length}</Mono> app{applications.length !== 1 ? 's' : ''}
        </p>
      </div>

      {/* Apps with issues */}
      {appsWithIssues.length > 0 && (
        <div className="mb-6 space-y-3">
          {appsWithIssues.map((app) => {
            const stats = appStats.get(app.id)!

            return (
              <a
                key={app.id}
                href={`/apps/${app.id}/issues`}
                className="flex items-center justify-between p-4 border-2 border-[#e11d48] bg-rose-50 hover:bg-rose-100 transition-all"
              >
                <div className="flex items-baseline gap-3">
                  <span className="font-bold text-foreground text-lg">{app.name}</span>
                  {stats.total > stats.active && (
                    <span className="text-xs text-foreground/40">
                      <Mono>{stats.total - stats.active}</Mono> stale
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-3">
                  {stats.unseenCount > 0 && (
                    <span className="text-xs font-bold text-[#e11d48] uppercase">
                      {stats.unseenCount} new
                    </span>
                  )}
                  <div className="flex items-center">
                    <span className="w-5 flex justify-center">
                      {stats.trending && <TrendingUp className="w-4 h-4 text-[#e11d48]" />}
                    </span>
                    <Mono className="text-2xl font-black text-[#e11d48] w-12 text-right">{stats.active}</Mono>
                  </div>
                </div>
              </a>
            )
          })}
        </div>
      )}

      {/* Healthy apps - compact */}
      {healthyApps.length > 0 && (
        <div className="space-y-2">
          {healthyApps.map((app) => (
            <a
              key={app.id}
              href={`/apps/${app.id}/issues`}
              className="flex items-center justify-between p-4 border border-foreground/10 hover:border-foreground/20 transition-all"
            >
              <span className="font-semibold text-foreground/70">{app.name}</span>
              <span className="w-3 h-3 bg-accent" />
            </a>
          ))}
        </div>
      )}

      <div className="mt-8">
        <a href="/apps" className="text-sm font-semibold text-foreground/50 hover:text-foreground transition-colors">
          + Add app
        </a>
      </div>
    </Layout>
  )
}
