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
  const unseenCount = issues.filter((i) => i.unseen).length

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
      {/* Header */}
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold text-black">Issues</h1>
          <p className="text-sm text-black/50 mt-1">
            {issues.length === 0 ? (
              'Errors grouped by message'
            ) : (
              <>
                <span className="font-mono tabular-nums">{issues.length}</span> unique error{issues.length !== 1 ? 's' : ''}
                {unseenCount > 0 && <> · <span className="font-mono tabular-nums">{unseenCount}</span> new</>}
              </>
            )}
          </p>
        </div>
        {hasUnseen && (
          <button
            onClick={markAllSeen}
            className="flex items-center gap-2 px-4 py-2 text-sm font-semibold text-black/70 hover:text-black bg-white border border-black/10 hover:border-black/20 transition-colors"
          >
            <Check className="w-4 h-4" />
            Mark all seen
          </button>
        )}
      </div>

      {issues.length === 0 ? (
        <div className="border border-black/10 py-16 text-center">
          <p className="font-semibold text-black mb-1">No issues</p>
          <p className="text-sm text-black/50">Enjoy your day.</p>
        </div>
      ) : (
        <div className="space-y-8">
          {/* Active Issues */}
          {activeIssues.length > 0 && (
            <section>
              <div className="flex items-center gap-2 mb-4">
                <span className="w-2 h-2 rounded-full bg-red-500" />
                <h2 className="text-sm font-bold text-black/50 uppercase tracking-wide">
                  Active
                </h2>
                <span className="text-xs text-black/30">Last 24 hours</span>
              </div>
              <div className="bg-white border border-black/10 divide-y divide-black/5 overflow-hidden">
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
              <div className="flex items-center gap-2 mb-4">
                <Clock className="w-3.5 h-3.5 text-black/30" />
                <h2 className="text-sm font-bold text-black/30 uppercase tracking-wide">
                  Stale
                </h2>
                <span className="text-xs text-black/20">No recent occurrences</span>
              </div>
              <div className="bg-white border border-black/5 divide-y divide-black/5 overflow-hidden opacity-60 hover:opacity-100 transition-opacity">
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
      className={`flex items-start gap-4 p-5 transition-colors hover:bg-black/[0.02] ${
        isUnseen ? 'border-l-4 border-l-red-600 bg-red-50/50 pl-[16px]' : ''
      }`}
    >
      {/* Main content */}
      <div className="flex-1 min-w-0">
        <p className={`leading-snug ${
          isUnseen
            ? 'font-semibold text-black'
            : stale
              ? 'text-black/50'
              : 'text-black/80'
        }`}>
          {issue.error}
        </p>
        <div className="flex items-center gap-2 mt-2 text-xs text-black/40">
          {appName && (
            <>
              <span className="bg-black/5 text-black/60 px-1.5 py-0.5 font-medium">
                {appName}
              </span>
              <span>·</span>
            </>
          )}
          <span>{formatDistanceToNow(new Date(issue.last_seen), { addSuffix: true })}</span>
          {issue.paths?.length > 0 && (
            <>
              <span>·</span>
              <code className="font-mono">{issue.paths[0]}</code>
            </>
          )}
          {issue.paths?.length > 1 && (
            <span className="text-black/30">+{issue.paths.length - 1}</span>
          )}
        </div>
      </div>

      {/* Count badge - BIG and BOLD */}
      <span className={`font-mono text-lg font-bold tabular-nums ${
        isUnseen
          ? 'text-red-600'
          : stale
            ? 'text-black/20'
            : 'text-black/40'
      }`}>
        {issue.count}×
      </span>
    </a>
  )
}
