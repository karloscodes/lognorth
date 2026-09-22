import { useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import { Layout } from '@/components/layout'
import { Mono, Code, PageHeader, EmptyState } from '@/lib/shared'
import type { Application } from '@/lib/shared'

interface EndpointStats {
  path: string
  total: number
  errors: number
  error_rate: number
  avg_duration: number
}

interface RecentError {
  id: number
  message: string
  timestamp: string
  context?: string
}

interface Props {
  endpoints: EndpointStats[]
  appId: number
  appName: string
  applications?: Application[]
}

function EndpointRow({ endpoint, appId }: { endpoint: EndpointStats; appId: number }) {
  const [expanded, setExpanded] = useState(false)
  const [recentErrors, setRecentErrors] = useState<RecentError[]>([])
  const [loading, setLoading] = useState(false)

  const hasErrors = endpoint.errors > 0
  const errorRateNum = endpoint.total > 0 ? (endpoint.errors / endpoint.total) * 100 : 0
  const errorRate = errorRateNum.toFixed(1)
  const isHighErrorRate = errorRateNum >= 20
  const isCriticalErrorRate = errorRateNum >= 40

  const handleClick = async () => {
    if (expanded) {
      setExpanded(false)
      return
    }

    setExpanded(true)

    if (hasErrors && recentErrors.length === 0) {
      setLoading(true)
      try {
        const res = await fetch(
          `/api/v1/events?app_id=${appId}&is_error=true&search=${encodeURIComponent(endpoint.path)}&limit=5`,
          { credentials: 'same-origin' }
        )
        if (res.ok) {
          const data = await res.json()
          setRecentErrors(data.events || [])
        }
      } catch {
        // ignore
      } finally {
        setLoading(false)
      }
    }
  }

  return (
    <>
      <button
        onClick={handleClick}
        className={`w-full flex items-center gap-3 py-4 px-5 text-left transition-colors hover:bg-foreground/[0.02] ${
          isCriticalErrorRate
            ? 'bg-[#e11d48]/10 shadow-[inset_4px_0_0_0_#e11d48]'
            : isHighErrorRate
              ? 'bg-[#e11d48]/[0.06] shadow-[inset_3px_0_0_0_#e11d48]'
              : hasErrors
                ? 'shadow-[inset_3px_0_0_0_#e11d48]'
                : ''
        }`}
      >
        {/* Path */}
        <Code className={`text-sm truncate flex-1 ${isCriticalErrorRate ? 'text-foreground font-bold' : 'text-foreground font-semibold'}`}>
          {endpoint.path}
        </Code>

        {/* Stats */}
        <div className="flex items-center gap-4 shrink-0">
          <Mono className="text-sm text-foreground/50 w-16 text-right">
            {endpoint.total.toLocaleString()}
          </Mono>
          {hasErrors ? (
            <Mono className={`text-sm font-bold w-16 text-right ${isCriticalErrorRate ? 'text-[#e11d48]' : 'text-[#e11d48]/80'}`}>
              {errorRate}%
            </Mono>
          ) : (
            <Mono className="text-sm text-foreground/20 w-16 text-right">—</Mono>
          )}
          <Mono className="text-xs text-foreground/50 w-16 text-right">
            {endpoint.avg_duration}ms
          </Mono>
        </div>
      </button>

      {expanded && (
        <div className="px-5 pb-4 bg-foreground/[0.015] border-t border-foreground/5">
          <div className="py-4 space-y-4">
            {/* Stats summary */}
            <div className="flex items-center gap-4 text-xs text-foreground/50">
              <span><span className="font-mono tabular-nums">{endpoint.total.toLocaleString()}</span> requests</span>
              {hasErrors && (
                <>
                  <span>·</span>
                  <span className="text-[#e11d48] font-semibold"><span className="font-mono tabular-nums">{errorRate}</span>% error rate</span>
                </>
              )}
              <span>·</span>
              <span>avg <span className="font-mono tabular-nums">{endpoint.avg_duration}</span>ms</span>
            </div>

            {/* Recent errors */}
            {hasErrors && (
              <div>
                <p className="text-xs font-semibold text-foreground/50 uppercase tracking-wide mb-3">Recent errors</p>
                {loading ? (
                  <p className="text-sm text-foreground/40">Loading...</p>
                ) : recentErrors.length > 0 ? (
                  <div className="space-y-2">
                    {recentErrors.map((err) => (
                      <a
                        key={err.id}
                        href={`/apps/${appId}/log/${err.id}`}
                        className="block p-3 bg-card border border-foreground/10 hover:border-foreground/20 transition-colors"
                      >
                        <Code className="text-sm text-[#e11d48] truncate block">{err.message}</Code>
                        <p className="text-xs text-foreground/40 mt-1">
                          {formatDistanceToNow(new Date(err.timestamp), { addSuffix: true })}
                        </p>
                      </a>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-foreground/40">No recent errors found</p>
                )}
              </div>
            )}

            {/* View all link */}
            <a
              href={`/apps/${appId}/log?search=${encodeURIComponent(endpoint.path)}`}
              className="inline-block text-sm font-semibold text-foreground/50 hover:text-foreground"
            >
              View logs →
            </a>
          </div>
        </div>
      )}
    </>
  )
}

export default function Endpoints({ endpoints: endpointsProp, appId, appName, applications }: Props) {
  // The server sends them most broken first.
  const endpoints = endpointsProp || []
  const totalErrors = endpoints.reduce((sum, e) => sum + e.errors, 0)
  const totalRequests = endpoints.reduce((sum, e) => sum + e.total, 0)
  const overallErrorRate = totalRequests > 0 ? ((totalErrors / totalRequests) * 100).toFixed(1) : '0'

  return (
    <Layout currentPath="/endpoints" appId={appId} appName={appName} applications={applications}>
      <PageHeader
        title="Endpoints"
        subtitle={endpoints.length === 0 ? undefined : (
          totalErrors === 0 ? (
            <><Mono>{endpoints.length}</Mono> endpoints · <Mono>{totalRequests.toLocaleString()}</Mono> requests · all healthy</>
          ) : (
            <><Mono>{endpoints.length}</Mono> endpoints · <Mono>{totalRequests.toLocaleString()}</Mono> requests · <Mono className="text-[#e11d48] font-black">{overallErrorRate}%</Mono> failing</>
          )
        )}
      />

      {endpoints.length === 0 ? (
        <EmptyState title="Quiet so far" description="Requests appear once your app is connected." />
      ) : (
        <div className="bg-card border border-foreground/10 overflow-hidden">
          <div className="flex items-center gap-3 py-3 px-5 border-b border-foreground/10 text-xs font-semibold text-foreground/40 uppercase tracking-wide">
            <span className="flex-1">Path</span>
            <span className="w-16 text-right">Requests</span>
            <span className="w-16 text-right">Error %</span>
            <span className="w-16 text-right">Avg ms</span>
          </div>
          <div className="divide-y divide-foreground/10">
            {endpoints.map((endpoint) => (
              <EndpointRow key={endpoint.path} endpoint={endpoint} appId={appId} />
            ))}
          </div>
        </div>
      )}
    </Layout>
  )
}
