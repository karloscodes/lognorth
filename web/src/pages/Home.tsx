import { Layout } from '@/components/layout'
import { formatDistanceToNow } from 'date-fns'
import { AlertTriangle } from 'lucide-react'
import type { Application, Issue } from '@/lib/shared'

interface Props {
  applications?: Application[]
  recentIssues?: Issue[]
}

export default function Home({ applications = [], recentIssues = [] }: Props) {
  const totalErrors = applications.reduce((sum, app) => sum + (app.stats?.errors || 0), 0)
  const activeIssues = recentIssues.filter((i) => i.active)

  // No apps yet - onboarding
  if (applications.length === 0) {
    return (
      <Layout currentPath="/">
        <div className="py-12">
          <h1 className="text-3xl font-bold text-black mb-6">Know when things break.</h1>
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

  return (
    <Layout currentPath="/" applications={applications}>
      {/* Status */}
      <div className="mb-10">
        {totalErrors > 0 ? (
          <p className="text-xl text-red-600 font-bold">
            {totalErrors} error{totalErrors !== 1 ? 's' : ''} need attention
          </p>
        ) : (
          <p className="flex items-center gap-2 text-xl font-semibold">
            <span className="w-2.5 h-2.5 rounded-full bg-[#00D678]" />
            <span className="text-black">No errors</span>
          </p>
        )}
      </div>

      {/* Active Issues - the main focus */}
      <section className="mb-12">
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-xl font-bold text-black">Active Issues</h2>
          <a href="/issues" className="text-sm font-medium text-black/70 hover:text-black transition-colors">
            View all →
          </a>
        </div>

        {activeIssues.length === 0 ? (
          <div className="py-12 text-center border-2 border-dashed border-black/10">
            <p className="text-black/60 font-medium">Nothing broke today</p>
          </div>
        ) : (
          <div className="border border-black/10 divide-y divide-black/5">
            {activeIssues.slice(0, 5).map((issue) => (
              <a
                key={issue.hash}
                href={`/issues/${issue.hash}`}
                className={`flex items-center gap-4 p-4 transition-colors ${
                  issue.unseen
                    ? 'bg-red-500/[0.04] hover:bg-red-500/[0.08] border-l-2 border-red-500'
                    : 'hover:bg-black/[0.02]'
                }`}
              >
                {/* Error message */}
                <div className="flex-1 min-w-0">
                  <p className={`truncate ${issue.unseen ? 'font-semibold text-black' : 'font-medium text-black/80'}`}>
                    {issue.error}
                  </p>
                  <p className="text-sm text-black/60 mt-0.5">
                    {formatDistanceToNow(new Date(issue.last_seen), { addSuffix: true })}
                    {issue.paths?.length > 0 && (
                      <span className="ml-2 font-mono">{issue.paths[0]}</span>
                    )}
                  </p>
                </div>

                {/* Count */}
                <span className={`font-mono text-sm font-bold tabular-nums ${
                  issue.unseen ? 'text-red-600' : 'text-black/60'
                }`}>
                  {issue.count}×
                </span>
              </a>
            ))}
          </div>
        )}
      </section>

      {/* Apps overview - secondary */}
      <section>
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-xl font-bold text-black">Apps</h2>
          <a href="/apps" className="text-sm font-medium text-black/70 hover:text-black transition-colors">
            + Add app
          </a>
        </div>

        <div className="grid gap-3">
          {applications.map((app) => (
            <a
              key={app.id}
              href={`/apps/${app.id}/issues`}
              className="group flex items-center justify-between p-4 border-2 border-black/10 hover:border-black/30 transition-colors"
            >
              <div>
                <span className="font-semibold text-black">{app.name}</span>
                <span className="text-sm text-black/60 ml-3">
                  {(app.stats?.total || 0).toLocaleString()} requests
                </span>
              </div>

              {(app.stats?.errors || 0) > 0 ? (
                <div className="flex items-center gap-1.5 text-red-600 text-sm">
                  <AlertTriangle className="w-3.5 h-3.5" />
                  <span className="font-semibold">{app.stats?.errors}</span>
                </div>
              ) : (
                <span className="w-2 h-2 rounded-full bg-[#00D678]" />
              )}
            </a>
          ))}
        </div>
      </section>
    </Layout>
  )
}
