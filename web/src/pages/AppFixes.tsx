import { useState } from 'react'
import { Link } from '@inertiajs/react'
import { Layout } from '@/components/layout'

interface FixJob {
  id: number
  issue: string
  issue_hash?: string
  status: 'queued' | 'running' | 'pr_open' | 'failed'
  pr_url?: string
  detail?: string
  updated_at?: string
}

interface Application {
  id: number
  name: string
}

interface Props {
  appId: number
  appName: string
  applications: Application[]
  tokenReady?: boolean
  connected?: boolean
  githubRepo?: string
  jobs?: FixJob[]
}

type Status = FixJob['status']

const STATUS: Record<Status, { row: string; group: string; dot: string; text: string }> = {
  running: { row: 'Running', group: 'In progress', dot: 'bg-[#00D678] animate-pulse', text: 'text-foreground' },
  queued: { row: 'Queued', group: 'Queued', dot: 'bg-foreground/30', text: 'text-foreground/60' },
  pr_open: { row: 'PR open', group: 'Done', dot: 'bg-[#00D678]', text: 'text-foreground' },
  failed: { row: 'Failed', group: 'Failed', dot: 'bg-red-600', text: 'text-red-600' },
}

const ORDER: Status[] = ['running', 'queued', 'pr_open', 'failed']

export default function AppFixes({
  appId,
  appName,
  applications,
  tokenReady = false,
  connected: initialConnected = false,
  githubRepo: initialRepo = '',
  jobs = [],
}: Props) {
  const [connected, setConnected] = useState(initialConnected)
  const [repoSaved, setRepoSaved] = useState(initialRepo)
  const [repo, setRepo] = useState(initialRepo)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const connect = async () => {
    setError(null)
    setBusy(true)
    try {
      const res = await fetch(`/apps/api-keys/${appId}/github`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ github_repo: repo }),
      })
      const data = await res.json()
      if (data.success) {
        setConnected(true)
        setRepoSaved(data.github_repo)
      } else {
        setError(data.error)
      }
    } catch {
      setError('Failed to reach GitHub. Try again.')
    } finally {
      setBusy(false)
    }
  }

  const disconnect = async () => {
    setError(null)
    setBusy(true)
    try {
      const res = await fetch(`/apps/api-keys/${appId}/github/disconnect`, { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setConnected(false)
        setRepoSaved('')
        setRepo('')
      } else {
        setError(data.error)
      }
    } catch {
      setError('Failed to disconnect. Try again.')
    } finally {
      setBusy(false)
    }
  }

  const sorted = [...jobs].sort((a, b) => ORDER.indexOf(a.status) - ORDER.indexOf(b.status))

  return (
    <Layout currentPath={`/apps/${appId}/fixes`} appId={appId} appName={appName} applications={applications}>
      <div>
        <header className="mb-8">
          <p className="text-xs font-bold uppercase tracking-wide text-foreground/40 mb-2">Fixes</p>
          <h1 className="text-2xl font-bold text-foreground">{appName}</h1>
          {!connected && (
            <p className="text-foreground/50 text-sm mt-2">
              Point this app at a repo and lognorth opens pull requests that fix its issues.
            </p>
          )}
        </header>

        {error && (
          <div className="mb-6 p-3 bg-red-600/10 text-red-600 text-sm font-medium border border-red-600">
            {error}
          </div>
        )}

        {!tokenReady ? (
          <div className="border border-foreground/10 bg-card p-10 text-center">
            <p className="text-foreground font-semibold">Connect GitHub first</p>
            <p className="text-sm text-foreground/50 mt-1 max-w-md mx-auto">
              Add your GitHub token in{' '}
              <Link href="/settings/developer" className="underline hover:text-foreground">
                Settings → Developer → On your server
              </Link>
              , then pick this app's repo here.
            </p>
          </div>
        ) : !connected ? (
          <div className="border border-foreground/10 bg-card p-6 max-w-xl">
            <label className="block text-sm font-semibold text-foreground mb-1">Repository</label>
            <input
              type="text"
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
              placeholder="owner/repo"
              className="w-full px-3 py-2 bg-background border border-foreground/10 font-mono text-sm focus:border-foreground/40 focus:outline-none"
            />
            <button
              type="button"
              onClick={connect}
              disabled={busy || !repo}
              className="mt-3 px-4 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80 transition-colors disabled:opacity-50"
            >
              {busy ? 'Checking…' : 'Connect repository'}
            </button>
            <p className="text-sm text-foreground/50 mt-3">Must be one of the repos your GitHub token can reach.</p>
          </div>
        ) : (
          <>
            <div className="flex items-center justify-between border border-foreground/10 bg-card px-4 py-3 mb-6">
              <div className="flex items-center gap-2 text-sm">
                <span className="w-2 h-2 rounded-full bg-[#00D678]" />
                <span className="text-foreground">
                  Working on <span className="font-mono font-semibold">{repoSaved}</span>
                </span>
              </div>
              <button
                type="button"
                onClick={disconnect}
                disabled={busy}
                className="text-sm font-semibold text-foreground/50 hover:text-foreground disabled:opacity-50"
              >
                {busy ? 'Working…' : 'Disconnect'}
              </button>
            </div>

            {jobs.length === 0 ? (
              <div className="border border-foreground/10 bg-card p-10 text-center">
                <p className="text-foreground font-semibold">Nothing queued</p>
                <p className="text-sm text-foreground/50 mt-1">
                  Issues the agent picks up from {repoSaved} show up here while it works them.
                </p>
              </div>
            ) : (
              <div className="border border-foreground/10 bg-card divide-y divide-foreground/10">
                {sorted.map((job) => {
                    const s = STATUS[job.status]
                    const title = (
                      <span className="text-sm text-foreground truncate">{job.issue}</span>
                    )
                    return (
                      <div key={job.id} className="flex items-center gap-4 px-4 py-3">
                        <span className="flex items-center gap-2 shrink-0 w-28">
                          <span className={`w-2 h-2 rounded-full ${s.dot}`} />
                          <span className={`text-sm ${s.text}`}>{s.row}</span>
                        </span>
                        {job.issue_hash ? (
                          <Link
                            href={`/apps/${appId}/issues/${job.issue_hash}`}
                            className="flex-1 min-w-0 hover:underline"
                          >
                            {title}
                          </Link>
                        ) : (
                          <span className="flex-1 min-w-0">{title}</span>
                        )}
                        {job.pr_url && (
                          <a
                            href={job.pr_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-sm font-semibold text-foreground/70 underline hover:text-foreground shrink-0"
                          >
                            PR →
                          </a>
                        )}
                        <span className="text-sm text-foreground/40 font-mono tabular-nums shrink-0 w-16 text-right">
                          {formatWhen(job.updated_at)}
                        </span>
                      </div>
                    )
                  })}
              </div>
            )}
          </>
        )}
      </div>
    </Layout>
  )
}

function formatWhen(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
