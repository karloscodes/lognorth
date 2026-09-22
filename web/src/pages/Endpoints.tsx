import { useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import { Layout } from '@/components/layout'
import { Mono, PageHeader, EmptyState } from '@/lib/shared'
import type { Application } from '@/lib/shared'

interface EndpointStats {
  path: string
  total: number
  errors: number
  error_rate: number
  avg_duration: number
}

// An endpoint alerting now, judged against its own history (events.ListAlerts).
interface Alert {
  path: string
  kind: 'error_rate' | 'silence'
  severity: 'warning' | 'critical'
  since: string
  summary: string
}

interface RecentError {
  id: number
  message: string
  timestamp: string
}

interface Props {
  endpoints: EndpointStats[]
  alerts?: Alert[]
  appId: number
  appName: string
  applications?: Application[]
}

const COLS = 'grid grid-cols-[1fr_9ch_150px_8ch_8ch] items-center gap-4 max-md:grid-cols-[1fr_7ch_7ch]'

function EndpointRow({ endpoint, alert, maxRate, appId }: { endpoint: EndpointStats; alert?: Alert; maxRate: number; appId: number }) {
  const [expanded, setExpanded] = useState(false)
  const [recentErrors, setRecentErrors] = useState<RecentError[]>([])
  const [loading, setLoading] = useState(false)

  const rate = endpoint.total > 0 ? (endpoint.errors / endpoint.total) * 100 : 0
  const spike = alert?.kind === 'error_rate'

  const toggle = async () => {
    if (expanded) return setExpanded(false)
    setExpanded(true)
    if (endpoint.errors > 0 && recentErrors.length === 0) {
      setLoading(true)
      try {
        const res = await fetch(`/api/v1/events?app_id=${appId}&is_error=true&search=${encodeURIComponent(endpoint.path)}&limit=5`, {
          credentials: 'same-origin',
        })
        if (res.ok) setRecentErrors((await res.json()).events || [])
      } catch {
        // the panel still links to the logs
      } finally {
        setLoading(false)
      }
    }
  }

  return (
    <div className={`border-t border-line first:border-t-0 ${alert ? 'shadow-[inset_3px_0_0_0_hsl(var(--error))]' : ''}`}>
      <button onClick={toggle} className={`${COLS} w-full px-5 py-2.5 text-left text-[13px] hover:bg-raise`}>
        <span className="flex min-w-0 items-center gap-2.5">
          <span className="truncate text-foreground">{endpoint.path}</span>
          {alert && (
            <span className="shrink-0 bg-error px-1.5 text-[11px] font-bold uppercase text-foreground">
              {spike ? 'spike' : 'silent'}
            </span>
          )}
        </span>
        <Mono className="text-right text-subtle">{endpoint.total.toLocaleString()}</Mono>
        <span className="relative h-1.5 bg-line max-md:hidden">
          <i
            className={`absolute inset-y-0 left-0 ${spike ? 'bg-error' : 'bg-subtle'}`}
            style={{ width: `${Math.min(100, (rate / maxRate) * 100)}%` }}
          />
        </span>
        <Mono className={`text-right ${spike ? 'font-bold text-error' : endpoint.errors ? 'text-text' : 'text-dim'}`}>
          {endpoint.errors ? `${rate.toFixed(1)}%` : '—'}
        </Mono>
        <Mono className="text-right text-subtle max-md:hidden">{endpoint.avg_duration}ms</Mono>
      </button>

      {expanded && (
        <div className="space-y-3 border-t border-dashed border-line px-5 pb-4 pt-3 text-[13px]">
          {alert && <p className="text-foreground">{alert.summary}</p>}
          {endpoint.errors > 0 && (
            <div>
              <p className="mb-2 text-xs text-dim">recent errors</p>
              {loading ? (
                <p className="text-dim">Loading…</p>
              ) : recentErrors.length > 0 ? (
                <div className="space-y-1">
                  {recentErrors.map((err) => (
                    <a key={err.id} href={`/apps/${appId}/log/${err.id}`} className="grid grid-cols-[1fr_auto] gap-4 hover:bg-raise">
                      <span className="truncate text-error">{err.message}</span>
                      <span className="text-dim">{formatDistanceToNow(new Date(err.timestamp), { addSuffix: true })}</span>
                    </a>
                  ))}
                </div>
              ) : (
                <p className="text-dim">Nothing here</p>
              )}
            </div>
          )}
          <a href={`/apps/${appId}/log?search=${encodeURIComponent(endpoint.path)}`} className="inline-block text-subtle hover:text-foreground">
            view logs →
          </a>
        </div>
      )}
    </div>
  )
}

export default function Endpoints({ endpoints: endpointsProp, alerts = [], appId, appName, applications }: Props) {
  // The server sends them most broken first.
  const endpoints = endpointsProp || []
  const totalErrors = endpoints.reduce((sum, e) => sum + e.errors, 0)
  const totalRequests = endpoints.reduce((sum, e) => sum + e.total, 0)
  const overallRate = totalRequests > 0 ? ((totalErrors / totalRequests) * 100).toFixed(1) : '0'
  const maxRate = Math.max(1, ...endpoints.map((e) => (e.total > 0 ? (e.errors / e.total) * 100 : 0)))
  const alertByPath = new Map(alerts.map((a) => [a.path, a]))

  return (
    <Layout currentPath="/endpoints" appId={appId} appName={appName} applications={applications}>
      <PageHeader
        title="Endpoints"
        subtitle={
          endpoints.length === 0 ? undefined : (
            <>
              <Mono>{endpoints.length}</Mono> endpoints · <Mono>{totalRequests.toLocaleString()}</Mono> requests in 4 weeks ·{' '}
              {totalErrors === 0 ? 'all healthy' : <><Mono className="text-foreground">{overallRate}%</Mono> failing</>}
              {alerts.length > 0 && (
                <>
                  {' '}· <span className="font-bold text-error">{alerts.length} alerting</span>
                </>
              )}
            </>
          )
        }
      />

      {endpoints.length === 0 ? (
        <EmptyState title="Quiet so far" description="Requests appear once your app is connected." />
      ) : (
        <>
          <div className="pane pb-1.5 pt-3.5">
            <span className="pane-title">by error rate · 4 weeks</span>
            <div className={`${COLS} px-5 pb-2 text-[11px] text-dim`}>
              <span>path</span>
              <span className="text-right">requests</span>
              <span className="max-md:hidden">errors</span>
              <span className="text-right">error %</span>
              <span className="text-right max-md:hidden">avg</span>
            </div>
            <div className="border-t border-line">
              {endpoints.map((endpoint) => (
                <EndpointRow key={endpoint.path} endpoint={endpoint} alert={alertByPath.get(endpoint.path)} maxRate={maxRate} appId={appId} />
              ))}
            </div>
          </div>
          <p className="mt-3.5 text-[13px] text-subtle">
            Red means LogNorth sees a spike against that endpoint's own history, not a fixed percentage.
          </p>
        </>
      )}
    </Layout>
  )
}
