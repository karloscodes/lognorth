import { useEffect, useRef, useState } from 'react'
import { formatDistanceToNowStrict } from 'date-fns'
import { router } from '@inertiajs/react'
import { Layout } from '@/components/layout'
import { Mono, PageHeader, EmptyState } from '@/lib/shared'
import type { Application, Issue } from '@/lib/shared'
import { pageKey } from '@/lib/keys'
import { usePageCommand } from '@/lib/commands'

// The issues that failed first in an issue's requests, by issue hash.
type Causes = Record<string, { hash: string; error: string; error_class?: string; traces: number; of: number }[]>

interface Props {
  issues?: Issue[]
  causes?: Causes
  appId?: number
  appName?: string
  applications?: Application[]
}

// "15h ago", "2mo ago": short enough for a meta line.
const ago = (iso: string) =>
  formatDistanceToNowStrict(new Date(iso))
    .replace(/ seconds?/, 's')
    .replace(/ minutes?/, 'm')
    .replace(/ hours?/, 'h')
    .replace(/ days?/, 'd')
    .replace(/ months?/, 'mo')
    .replace(/ years?/, 'y') + ' ago'

export default function Issues({ issues = [], causes = {}, appId, appName, applications }: Props) {
  const issuePath = (issue: Issue) => `/apps/${appId || issue.api_key_id}/issues/${issue.hash}`
  const activeIssues = issues.filter((i) => i.active)
  const staleIssues = issues.filter((i) => !i.active)
  const ordered = [...activeIssues, ...staleIssues]
  const unseenCount = issues.filter((i) => i.unseen).length

  const appNameById = new Map<number, string>()
  applications?.forEach((app) => appNameById.set(app.id, app.name))
  const showAppName = !appId

  const markAllSeen = () => router.post(appId ? `/issues/seen?app_id=${appId}` : '/issues/seen', {}, { preserveScroll: true })
  const markSeen = (issue: Issue) => router.post(`/issues/${issue.hash}/seen`, {}, { preserveScroll: true })
  const toggleMute = (issue: Issue) =>
    router.post(`/issues/${issue.hash}/${issue.muted ? 'unmute' : 'mute'}`, {}, { preserveScroll: true })

  // Browsers may restore /issues from bfcache on "back" from an issue page. That
  // hides the seen state the server already applied, so fetch fresh.
  useEffect(() => {
    const onPageShow = (e: PageTransitionEvent) => {
      if (e.persisted) router.reload()
    }
    window.addEventListener('pageshow', onPageShow)
    return () => window.removeEventListener('pageshow', onPageShow)
  }, [])

  // Keys on the list: j/k move, enter opens, m mutes, e marks seen, E marks all seen.
  const [sel, setSel] = useState(0)
  usePageCommand('mute', () => {
    const current = ordered[Math.min(sel, ordered.length - 1)]
    if (!current) return 'E: no issue to mute'
    toggleMute(current)
  })
  const rowRefs = useRef<(HTMLDivElement | null)[]>([])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!pageKey(e) || !ordered.length) return
      const current = ordered[Math.min(sel, ordered.length - 1)]
      const move = (to: number) => {
        setSel(to)
        rowRefs.current[to]?.scrollIntoView({ block: 'nearest' })
      }
      if (e.key === 'j') move(Math.min(ordered.length - 1, sel + 1))
      if (e.key === 'k') move(Math.max(0, sel - 1))
      if (e.key === 'Enter') router.visit(issuePath(current))
      if (e.key === 'm') toggleMute(current)
      if (e.key === 'e') markSeen(current)
      if (e.key === 'E') markAllSeen()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const row = (issue: Issue, stale = false) => {
    const i = ordered.indexOf(issue)
    return (
      <IssueRow
        key={issue.hash}
        ref={(el) => (rowRefs.current[i] = el)}
        issue={issue}
        after={causes[issue.hash]}
        href={issuePath(issue)}
        appName={showAppName ? appNameById.get(issue.api_key_id) : undefined}
        stale={stale}
        selected={i === sel}
        onSelect={() => setSel(i)}
        onMute={() => toggleMute(issue)}
      />
    )
  }

  return (
    <Layout currentPath="/issues" appId={appId} appName={appName} applications={applications}>
      <PageHeader
        title="Issues"
        subtitle={
          issues.length === 0 ? undefined : (
            <>
              <Mono>{issues.length}</Mono> issue{issues.length !== 1 ? 's' : ''}
              {unseenCount > 0 && (
                <>
                  {' '}· <Mono>{unseenCount}</Mono> new
                </>
              )}
            </>
          )
        }
      >
        {unseenCount > 0 && (
          <button onClick={markAllSeen} className="key">
            <kbd>E</kbd> mark all seen
          </button>
        )}
      </PageHeader>

      {issues.length === 0 ? (
        <EmptyState title="No issues" description="Enjoy your day." />
      ) : (
        <div className="space-y-9">
          {activeIssues.length > 0 && (
            <section>
              <p className="rule-label mb-3">
                <b>active</b> last 7 days
              </p>
              <div className="border border-border">{activeIssues.map((issue) => row(issue))}</div>
            </section>
          )}
          {staleIssues.length > 0 && (
            <section>
              <p className="rule-label mb-3">
                <b>stale</b> nothing new in 7 days
              </p>
              <div className="border border-border">{staleIssues.map((issue) => row(issue, true))}</div>
            </section>
          )}
        </div>
      )}
    </Layout>
  )
}

// 14 bars, one per day. The last two days turn red when they have errors.
function Sparkline({ daily = [], hot }: { daily?: number[]; hot: boolean }) {
  const max = Math.max(1, ...daily)
  return (
    <span className="hidden h-[18px] items-end gap-[2px] md:inline-flex" aria-hidden="true">
      {daily.map((n, i) => (
        <i
          key={i}
          className={`w-1 ${hot && n > 0 && i >= daily.length - 2 ? 'bg-error' : 'bg-border'}`}
          style={{ height: n > 0 ? Math.max(3, Math.round((n / max) * 18)) : 1 }}
        />
      ))}
    </span>
  )
}

interface RowProps {
  issue: Issue
  after?: Causes[string]
  href: string
  appName?: string
  stale: boolean
  selected: boolean
  onSelect: () => void
  onMute: () => void
  ref: (el: HTMLDivElement | null) => void
}

function IssueRow({ issue, after, href, appName, stale, selected, onSelect, onMute, ref }: RowProps) {
  const unseen = issue.unseen && !stale && !issue.muted
  const extraPaths = (issue.paths?.length ?? 0) - 1
  const where = issue.error_file ? `${issue.error_file}${issue.error_line ? `:${issue.error_line}` : ''}` : ''

  return (
    <div
      ref={ref}
      onMouseEnter={onSelect}
      className={`group grid scroll-my-24 grid-cols-[1fr_auto] gap-5 border-l-[3px] border-t border-t-line py-3.5 pl-4 pr-5 first:border-t-0 ${
        unseen ? 'border-l-error' : 'border-l-transparent'
      } ${selected ? 'bg-raise' : ''} ${issue.muted ? 'opacity-40' : stale ? 'opacity-60' : ''}`}
    >
      <a href={href} className="min-w-0">
        <p className="text-xs text-error">
          {appName && <span className="mr-2 text-subtle">[{appName}]</span>}
          {issue.error_class || 'Error'}
          {issue.is_new && !stale && <span className="ml-2 font-bold uppercase text-error">new</span>}
        </p>
        <p className={`mt-0.5 truncate text-[15px] ${unseen ? 'font-medium text-foreground' : 'text-text'}`}>{issue.error}</p>
        <p className="mt-1 truncate text-xs text-dim">
          last <span className="text-subtle">{ago(issue.last_seen)}</span> · first {ago(issue.first_seen)}
          {issue.paths?.length > 0 && (
            <>
              {' '}· <span className="text-subtle">{issue.paths[0]}</span>
              {extraPaths > 0 && ` +${extraPaths}`}
            </>
          )}
          {where && ` · ${where}`}
        </p>
        {after && after.length > 0 && (
          <p className="mt-1 truncate text-xs text-subtle">
            ↳ after <span className="text-error">{after[0].error_class || after[0].error}</span> in {after[0].traces} of {after[0].of} requests
          </p>
        )}
      </a>

      <div className="flex items-center gap-5">
        <Sparkline daily={issue.daily} hot={!stale && !issue.muted} />
        <button
          type="button"
          onClick={onMute}
          className={`w-[7ch] text-left text-xs text-dim hover:text-foreground ${selected || issue.muted ? 'visible' : 'invisible group-hover:visible'}`}
        >
          <kbd className="font-[inherit]">m</kbd> {issue.muted ? 'unmute' : 'mute'}
        </button>
        <Mono
          className={`min-w-[6ch] text-right text-xl font-bold ${unseen ? 'text-error' : stale || issue.muted ? 'text-subtle' : 'text-foreground'}`}
          title={`${issue.count} times`}
        >
          {issue.count}×
        </Mono>
      </div>
    </div>
  )
}
