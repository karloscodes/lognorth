import { formatDistanceToNow } from 'date-fns'
import { Clock, Check, TrendingUp, TrendingDown } from 'lucide-react'
import { router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { Mono, Code, PageHeader, EmptyState } from '@/lib/shared'
import type { Application, Issue } from '@/lib/shared'

interface Props {
  issues?: Issue[]
  appId?: number
  appName?: string
  applications?: Application[]
}

export default function Issues({ issues = [], appId, appName, applications }: Props) {
  // Click issue → go to issue detail page
  const issuePath = (issue: Issue) => {
    const id = appId || issue.api_key_id
    return `/apps/${id}/issues/${issue.hash}`
  }
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
      <PageHeader
        title="Issues"
        subtitle={issues.length === 0 ? undefined : (
          <>
            <Mono>{issues.length}</Mono> issue{issues.length !== 1 ? 's' : ''}
            {unseenCount > 0 && <> · <Mono>{unseenCount}</Mono> new</>}
          </>
        )}
      >
        {hasUnseen && (
          <button
            onClick={markAllSeen}
            className="flex items-center gap-2 px-4 py-2 text-sm font-semibold text-black/70 hover:text-black bg-white border border-black/10 hover:border-black/20 transition-colors"
          >
            <Check className="w-4 h-4" />
            Mark all seen
          </button>
        )}
      </PageHeader>

      {issues.length === 0 ? (
        <EmptyState title="No issues" description="Enjoy your day." />
      ) : (
        <div className="space-y-8">
          {/* Active Issues */}
          {activeIssues.length > 0 && (
            <section>
              <div className="flex items-center gap-2 mb-4">
                <span className="w-2 h-2 rounded-full bg-[#e11d48]" />
                <h2 className="text-sm font-bold text-black/50 uppercase tracking-wide">
                  Active
                </h2>
                <span className="text-xs text-black/40">Last 24 hours</span>
              </div>
              <div className="bg-white border border-black/10 divide-y divide-black/10 overflow-hidden">
                {activeIssues.map((issue) => (
                  <IssueRow
                    key={issue.hash}
                    issue={issue}
                    issuePath={issuePath(issue)}
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
                <Clock className="w-3.5 h-3.5 text-black/40" />
                <h2 className="text-sm font-bold text-black/50 uppercase tracking-wide">
                  Stale
                </h2>
                <span className="text-xs text-black/40">No recent occurrences</span>
              </div>
              <div className="bg-white border border-black/10 divide-y divide-black/10 overflow-hidden">
                {staleIssues.map((issue) => (
                  <IssueRow
                    key={issue.hash}
                    issue={issue}
                    issuePath={issuePath(issue)}
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

  const toggleMute = () => {
    const url = issue.muted ? `/issues/${issue.hash}/unmute` : `/issues/${issue.hash}/mute`
    router.post(url, {}, { preserveScroll: true })
  }

  return (
    <div
      className={`flex items-center gap-4 p-5 transition-colors hover:bg-black/[0.02] ${
        isUnseen && !issue.muted ? 'shadow-[inset_3px_0_0_0_#e11d48] bg-rose-50/50' : ''
      } ${issue.muted ? 'opacity-50' : ''}`}
    >
      {/* Main content - clickable link */}
      <a href={issuePath} className="flex-1 min-w-0 py-1">
        <div className="flex items-center gap-2">
          <Code className={`leading-snug ${
            isUnseen && !issue.muted
              ? 'font-semibold text-black'
              : stale || issue.muted
                ? 'text-black/50'
                : 'text-black/80'
          }`}>
            {issue.error}
          </Code>
          {issue.is_new && !stale && (
            <span className="text-[10px] font-bold text-[#e11d48] uppercase tracking-wide">
              New
            </span>
          )}
        </div>
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
              <Code>{issue.paths[0]}</Code>
            </>
          )}
          {issue.paths?.length > 1 && (
            <span className="text-black/30">+{issue.paths.length - 1}</span>
          )}
        </div>
      </a>

      {/* Right side: mute + count aligned */}
      <div className="flex items-center gap-4 shrink-0">
        <button
          type="button"
          onClick={toggleMute}
          className="text-xs font-semibold text-black/50 hover:text-black transition-colors w-12 text-right"
        >
          {issue.muted ? 'Unmute' : 'Mute'}
        </button>

        <div className="flex items-center">
          <span className="w-5 flex justify-center">
            {issue.trend === 'up' && !stale && (
              <TrendingUp className="w-4 h-4 text-[#e11d48]" />
            )}
            {issue.trend === 'down' && !stale && (
              <TrendingDown className="w-4 h-4 text-black/30" />
            )}
          </span>
          <Mono
            className={`text-xl font-black w-16 text-right ${
              isUnseen && !issue.muted
                ? 'text-[#e11d48]'
                : stale || issue.muted
                  ? 'text-black/30'
                  : 'text-black'
            }`}
            title={`${issue.count} occurrences`}
          >
            {issue.count}×
          </Mono>
        </div>
      </div>
    </div>
  )
}
