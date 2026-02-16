import { formatDistanceToNow } from 'date-fns'
import { Layout } from '@/components/layout'
import type { Application, Issue } from '@/lib/shared'

interface Props {
  applications?: Application[]
  recentIssues?: Issue[]
}

export default function Home({ applications = [], recentIssues = [] }: Props) {
  const totalErrors = applications.reduce((sum, app) => sum + (app.stats?.errors || 0), 0)

  // Build lookup: app_id -> most recent issue
  const topIssueByApp = new Map<number, Issue>()
  for (const issue of recentIssues) {
    if (!topIssueByApp.has(issue.api_key_id)) {
      topIssueByApp.set(issue.api_key_id, issue)
    }
  }

  // No apps yet - onboarding
  if (applications.length === 0) {
    return (
      <Layout currentPath="/">
        <div className="py-20 max-w-md">
          <h1 className="text-3xl font-bold text-black mb-3">
            Know when things break.
          </h1>
          <p className="text-black/60 mb-6">
            Your errors. Your server. Your data.
          </p>
          <a
            href="/apps"
            className="inline-flex px-5 py-2.5 bg-black text-white font-semibold hover:bg-black/80 transition-colors"
          >
            Add your first app
          </a>
        </div>
      </Layout>
    )
  }

  const appsWithErrors = applications.filter(app => (app.stats?.errors || 0) > 0)
  const healthyApps = applications.filter(app => (app.stats?.errors || 0) === 0)

  return (
    <Layout currentPath="/" applications={applications}>
      {/* Header with status */}
      <div className="flex items-baseline justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold text-black">Overview</h1>
          <p className="text-sm text-black/50 mt-1">
            <span className="font-mono tabular-nums">{applications.length}</span> app{applications.length !== 1 ? 's' : ''}
          </p>
        </div>

        {/* Status badge */}
        {totalErrors > 0 ? (
          <div className="flex items-baseline gap-2">
            <span className="font-mono tabular-nums text-4xl font-black text-red-600">
              {totalErrors}
            </span>
            <span className="text-lg font-semibold text-black/60">
              error{totalErrors !== 1 ? 's' : ''}
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="w-3 h-3 bg-[#00D678]" />
            <span className="text-lg font-semibold text-black/60">All clear</span>
          </div>
        )}
      </div>

      {/* Apps with errors - show top error inline */}
      {appsWithErrors.length > 0 && (
        <div className="mb-6 space-y-3">
          {appsWithErrors.map((app) => {
            const errors = app.stats?.errors || 0
            const topIssue = topIssueByApp.get(app.id)

            return (
              <div
                key={app.id}
                className="border-2 border-red-500/30 bg-red-50 hover:border-red-500 transition-all"
              >
                <a
                  href={`/apps/${app.id}/issues`}
                  className="flex items-center justify-between p-4"
                >
                  <span className="font-bold text-black text-lg">{app.name}</span>
                  <span className="font-mono tabular-nums text-2xl font-black text-red-600">
                    {errors}
                  </span>
                </a>
                {topIssue && (
                  <a
                    href={`/apps/${app.id}/issues/${topIssue.hash}`}
                    className="block px-4 pb-4 -mt-1"
                  >
                    <p className="text-sm text-red-700 truncate">
                      {topIssue.error}
                    </p>
                    <p className="text-xs text-black/40 mt-1">
                      {formatDistanceToNow(new Date(topIssue.last_seen), { addSuffix: true })}
                      {topIssue.count > 1 && <> · <span className="font-mono tabular-nums">{topIssue.count}</span>×</>}
                    </p>
                  </a>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Healthy apps - compact */}
      {healthyApps.length > 0 && (
        <div>
          {appsWithErrors.length > 0 && (
            <p className="text-xs font-semibold text-black/30 uppercase tracking-wide mb-3">
              Healthy
            </p>
          )}
          <div className="space-y-2">
            {healthyApps.map((app) => (
              <a
                key={app.id}
                href={`/apps/${app.id}/issues`}
                className="flex items-center justify-between p-4 border border-black/10 hover:border-black/20 transition-all"
              >
                <span className="font-semibold text-black/70">{app.name}</span>
                <span className="w-2 h-2 bg-[#00D678]" />
              </a>
            ))}
          </div>
        </div>
      )}

      <div className="mt-8">
        <a href="/apps" className="text-sm font-semibold text-black/50 hover:text-black transition-colors">
          + Add app
        </a>
      </div>
    </Layout>
  )
}
