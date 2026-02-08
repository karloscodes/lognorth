import { formatDistanceToNow } from 'date-fns'
import { Clock, Check } from 'lucide-react'
import { router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import type { Application, Issue } from '@/lib/shared'

interface Props {
  issues?: Issue[]
  appId?: number
  appName?: string
  applications?: Application[]
}

export default function Issues({ issues = [], appId, appName, applications }: Props) {
  const issuePath = (hash: string) => appId ? `/apps/${appId}/issues/${hash}` : `/issues/${hash}`
  const activeIssues = issues.filter((i) => i.active)
  const staleIssues = issues.filter((i) => !i.active)
  const hasUnseen = issues.some((i) => i.unseen)

  // Build app name lookup for global view
  const appNameById = new Map<number, string>()
  applications?.forEach(app => appNameById.set(app.id, app.name))
  const showAppName = !appId

  const markAllSeen = () => {
    const url = appId ? `/issues/seen?app_id=${appId}` : '/issues/seen'
    router.post(url)
  }

  return (
    <Layout currentPath="/issues" appId={appId} appName={appName} applications={applications}>
        <div className="flex items-start justify-between mb-8">
          <div>
            <h2 className="text-2xl font-bold text-black">Issues</h2>
            <p className="text-sm text-black/60 mt-1">
              {issues.length === 0
                ? 'Errors grouped by message'
                : `${issues.length} unique error${issues.length !== 1 ? 's' : ''}`}
            </p>
          </div>
          {hasUnseen && (
            <button
              onClick={markAllSeen}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-black/70 hover:text-black border border-black/10 hover:border-black/20 transition-colors"
            >
              <Check className="w-4 h-4" />
              Mark all seen
            </button>
          )}
        </div>

        {issues.length === 0 ? (
          <div className="py-16 text-center">
            <p className="text-black/40">No errors yet. That's good.</p>
          </div>
        ) : (
          <div className="space-y-10">
            {/* Active Issues */}
            {activeIssues.length > 0 && (
              <section>
                <div className="flex items-center gap-2 mb-3 pb-3 border-b border-black/10">
                  <div className="w-2 h-2 rounded-full bg-red-500" />
                  <h3 className="font-semibold text-black text-sm uppercase tracking-wide">Active</h3>
                  <span className="text-xs text-black/40">Last 24 hours</span>
                </div>
                <div className="divide-y divide-black/5">
                  {activeIssues.map((issue) => (
                    <IssueRow
                      key={issue.hash}
                      issue={issue}
                      issuePath={issuePath(issue.hash)}
                      appName={showAppName ? appNameById.get(issue.api_key_id) : undefined}
                    />
                  ))}
                </div>
              </section>
            )}

            {/* Stale Issues */}
            {staleIssues.length > 0 && (
              <section>
                <div className="flex items-center gap-2 mb-3 pb-3 border-b border-black/5">
                  <Clock className="w-3.5 h-3.5 text-black/30" />
                  <h3 className="font-semibold text-black/50 text-sm uppercase tracking-wide">Stale</h3>
                  <span className="text-xs text-black/30">No recent occurrences</span>
                </div>
                <div className="divide-y divide-black/5 opacity-50 hover:opacity-100 transition-opacity">
                  {staleIssues.map((issue) => (
                    <IssueRow
                      key={issue.hash}
                      issue={issue}
                      issuePath={issuePath(issue.hash)}
                      appName={showAppName ? appNameById.get(issue.api_key_id) : undefined}
                      stale
                    />
                  ))}
                </div>
              </section>
            )}
          </div>
        )}
    </Layout>
  )
}

function IssueRow({ issue, issuePath, appName, stale }: { issue: Issue; issuePath: string; appName?: string; stale?: boolean }) {
  const isUnseen = issue.unseen && !stale

  return (
    <a
      href={issuePath}
      className={`group flex items-start gap-4 py-4 -mx-2 px-2 rounded transition-colors ${
        isUnseen
          ? 'bg-red-500/[0.04] hover:bg-red-500/[0.08] border-l-2 border-red-500 pl-4'
          : 'hover:bg-black/[0.02]'
      }`}
    >
      {/* Main content */}
      <div className="flex-1 min-w-0">
        <p className={`leading-snug ${
          isUnseen
            ? 'font-semibold text-black'
            : stale
              ? 'font-medium text-black/60'
              : 'font-medium text-black/80'
        }`}>
          {issue.error}
        </p>
        <div className="flex items-center gap-3 mt-1.5 text-xs text-black/50">
          {appName && (
            <>
              <span className="bg-black/5 text-black/60 px-1.5 py-0.5 rounded font-medium">
                {appName}
              </span>
              <span className="text-black/20">·</span>
            </>
          )}
          <span>{formatDistanceToNow(new Date(issue.last_seen), { addSuffix: true })}</span>
          {issue.paths?.length > 0 && (
            <>
              <span className="text-black/20">·</span>
              <code className="font-mono text-black/50">{issue.paths[0]}</code>
            </>
          )}
          {issue.paths?.length > 1 && (
            <span className="text-black/40">+{issue.paths.length - 1} more</span>
          )}
        </div>
      </div>

      {/* Count */}
      <span className={`font-mono text-sm font-bold tabular-nums ${
        isUnseen ? 'text-red-600' : stale ? 'text-black/30' : 'text-black/50'
      }`}>
        {issue.count}×
      </span>
    </a>
  )
}
