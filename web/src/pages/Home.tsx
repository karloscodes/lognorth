import { Layout } from '@/components/layout'
import { Mono } from '@/lib/shared'
import type { Application, Issue } from '@/lib/shared'

interface Props {
  applications?: Application[]
  recentIssues?: Issue[]
}

export default function Home({ applications = [], recentIssues = [] }: Props) {
  const totalIssues = recentIssues.length

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

  // Count issues per app
  const issueCountByApp = new Map<number, number>()
  for (const issue of recentIssues) {
    issueCountByApp.set(issue.api_key_id, (issueCountByApp.get(issue.api_key_id) || 0) + 1)
  }

  const appsWithIssues = applications.filter(app => issueCountByApp.has(app.id))
  const healthyApps = applications.filter(app => !issueCountByApp.has(app.id))

  return (
    <Layout currentPath="/" applications={applications}>
      {/* Status header */}
      <div className="flex items-center justify-between mb-8">
        {totalIssues > 0 ? (
          <div className="flex items-baseline gap-2">
            <Mono className="text-5xl font-black text-[#e11d48]">{totalIssues}</Mono>
            <span className="text-xl font-semibold text-black/60">
              issue{totalIssues !== 1 ? 's' : ''}
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-3">
            <span className="w-3 h-3 bg-accent" />
            <span className="text-xl font-semibold text-black/60">All clear</span>
          </div>
        )}
        <p className="text-sm text-black/40">
          <Mono>{applications.length}</Mono> app{applications.length !== 1 ? 's' : ''}
        </p>
      </div>

      {/* Apps with issues */}
      {appsWithIssues.length > 0 && (
        <div className="mb-6 space-y-3">
          {appsWithIssues.map((app) => {
            const issueCount = issueCountByApp.get(app.id) || 0

            return (
              <a
                key={app.id}
                href={`/apps/${app.id}/issues`}
                className="flex items-center justify-between p-4 border-2 border-[#e11d48] bg-rose-50 hover:bg-rose-100 transition-all"
              >
                <span className="font-bold text-black text-lg">{app.name}</span>
                <Mono className="text-2xl font-black text-[#e11d48]">{issueCount}</Mono>
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
              className="flex items-center justify-between p-4 border border-black/10 hover:border-black/20 transition-all"
            >
              <span className="font-semibold text-black/70">{app.name}</span>
              <span className="w-3 h-3 bg-accent" />
            </a>
          ))}
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
