import { Link } from '@inertiajs/react'
import { Layout } from '@/components/layout'

interface FixJob {
  id: number
  issue: string
  status: 'queued' | 'running' | 'pr_open' | 'failed'
  pr_url?: string
  updated_at?: string
}

interface Props {
  jobs?: FixJob[]
  connected?: boolean
}

const GROUPS: { key: FixJob['status']; label: string }[] = [
  { key: 'running', label: 'In progress' },
  { key: 'queued', label: 'Queued' },
  { key: 'pr_open', label: 'Done' },
  { key: 'failed', label: 'Failed' },
]

export default function Fixes({ jobs = [], connected = false }: Props) {
  return (
    <Layout currentPath="/fixes">
      <div className="max-w-4xl mx-auto">
        <header className="mb-8">
          <p className="text-xs font-bold uppercase tracking-wide text-foreground/40 mb-2">Agent</p>
          <h1 className="text-2xl font-bold text-foreground">
            Fixes<span className="text-accent">.</span>
          </h1>
          <p className="text-foreground/50 text-sm mt-2">
            Every issue the agent picked up, and the pull request it opened.
          </p>
        </header>

        {!connected ? (
          <EmptyCard
            title="No repository connected"
            body={
              <>
                Open an app and connect its repo on the{' '}
                <Link href="/apps" className="underline hover:text-foreground">
                  Fixes
                </Link>{' '}
                tab. The agent then opens PRs for that app's issues.
              </>
            }
          />
        ) : jobs.length === 0 ? (
          <EmptyCard title="Nothing queued" body="Issues the agent picks up show up here while it works them." />
        ) : (
          <div className="space-y-8">
            {GROUPS.map(({ key, label }) => {
              const group = jobs.filter((j) => j.status === key)
              if (group.length === 0) return null
              return (
                <section key={key}>
                  <div className="flex items-baseline justify-between mb-3">
                    <p className="text-xs font-bold uppercase tracking-wide text-foreground/40">{label}</p>
                    <span className="text-sm text-foreground/40 font-mono tabular-nums">{group.length}</span>
                  </div>
                  <div className="border border-foreground/10 bg-card divide-y divide-foreground/10">
                    {group.map((job) => (
                      <div key={job.id} className="flex items-center gap-3 px-4 py-3">
                        <JobStatus status={job.status} />
                        <span className="flex-1 text-sm text-foreground truncate">{job.issue}</span>
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
                        <span className="text-sm text-foreground/40 font-mono tabular-nums shrink-0">
                          {formatWhen(job.updated_at)}
                        </span>
                      </div>
                    ))}
                  </div>
                </section>
              )
            })}
          </div>
        )}
      </div>
    </Layout>
  )
}

function EmptyCard({ title, body }: { title: string; body: React.ReactNode }) {
  return (
    <div className="border border-foreground/10 bg-card p-10 text-center">
      <p className="text-foreground font-semibold">{title}</p>
      <p className="text-sm text-foreground/50 mt-1 max-w-md mx-auto">{body}</p>
    </div>
  )
}

export function JobStatus({ status }: { status: FixJob['status'] }) {
  const map: Record<FixJob['status'], { dot: string; label: string }> = {
    queued: { dot: 'bg-foreground/30', label: 'Queued' },
    running: { dot: 'bg-[#00D678] animate-pulse', label: 'Running' },
    pr_open: { dot: 'bg-[#00D678]', label: 'PR open' },
    failed: { dot: 'bg-red-600', label: 'Failed' },
  }
  const { dot, label } = map[status]
  return (
    <span className="flex items-center gap-2 shrink-0 w-24">
      <span className={`w-2 h-2 rounded-full ${dot}`} />
      <span className="text-sm text-foreground/60">{label}</span>
    </span>
  )
}

function formatWhen(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
