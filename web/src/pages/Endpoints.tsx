import { useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import { Inbox } from 'lucide-react'
import { Layout } from '@/components/layout'
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
  const errorRate = endpoint.total > 0 ? ((endpoint.errors / endpoint.total) * 100).toFixed(1) : '0'

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
        className={`w-full flex items-center gap-3 py-4 px-5 text-left transition-colors hover:bg-black/[0.02] border-l-2 ${hasErrors ? 'border-l-red-500' : 'border-l-transparent'}`}
      >
        {/* Path */}
        <span className="font-mono text-sm text-black font-medium truncate flex-1">
          {endpoint.path}
        </span>

        {/* Stats */}
        <div className="flex items-center gap-4 shrink-0">
          <span className="font-mono text-sm text-black/50 tabular-nums w-16 text-right">
            {endpoint.total.toLocaleString()}
          </span>
          <span className={`font-mono text-sm font-semibold tabular-nums w-10 text-right ${
            hasErrors ? 'text-red-600' : 'text-black/30'
          }`}>
            {endpoint.errors}
          </span>
          <span className="font-mono text-xs text-black/50 tabular-nums w-16 text-right">
            {endpoint.avg_duration}ms
          </span>
        </div>
      </button>

      {expanded && (
        <div className="px-5 pb-4 bg-black/[0.015] border-t border-black/5">
          <div className="py-4 space-y-4">
            {/* Stats summary */}
            <div className="flex items-center gap-4 text-xs text-black/50">
              <span><span className="font-mono tabular-nums">{endpoint.total.toLocaleString()}</span> requests</span>
              {hasErrors && (
                <>
                  <span>·</span>
                  <span className="text-red-600 font-semibold"><span className="font-mono tabular-nums">{errorRate}</span>% error rate</span>
                </>
              )}
              <span>·</span>
              <span>avg <span className="font-mono tabular-nums">{endpoint.avg_duration}</span>ms</span>
            </div>

            {/* Recent errors */}
            {hasErrors && (
              <div>
                <p className="text-xs font-semibold text-black/50 uppercase tracking-wide mb-3">Recent errors</p>
                {loading ? (
                  <p className="text-sm text-black/40">Loading...</p>
                ) : recentErrors.length > 0 ? (
                  <div className="space-y-2">
                    {recentErrors.map((err) => (
                      <a
                        key={err.id}
                        href={`/apps/${appId}/log/${err.id}`}
                        className="block p-3 bg-white border border-black/10 hover:border-black/20 transition-colors"
                      >
                        <p className="text-sm text-red-600 truncate">{err.message}</p>
                        <p className="text-xs text-black/40 mt-1">
                          {formatDistanceToNow(new Date(err.timestamp), { addSuffix: true })}
                        </p>
                      </a>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-black/40">No recent errors found</p>
                )}
              </div>
            )}

            {/* View all link */}
            <a
              href={`/apps/${appId}/log?search=${encodeURIComponent(endpoint.path)}`}
              className="inline-block text-sm font-semibold text-black/50 hover:text-black"
            >
              View all requests →
            </a>
          </div>
        </div>
      )}
    </>
  )
}

export default function Endpoints({ endpoints: endpointsProp, appId, appName, applications }: Props) {
  const endpoints = (endpointsProp || []).sort((a, b) => b.errors - a.errors)
  const totalErrors = endpoints.reduce((sum, e) => sum + e.errors, 0)
  const totalRequests = endpoints.reduce((sum, e) => sum + e.total, 0)

  return (
    <Layout currentPath="/endpoints" appId={appId} appName={appName} applications={applications}>
      {/* Header */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-black">Endpoints</h1>
        <p className="text-sm text-black/50 mt-1">
          {endpoints.length === 0 ? (
            'HTTP requests grouped by path'
          ) : totalErrors === 0 ? (
            <><span className="font-mono tabular-nums">{endpoints.length}</span> endpoints · <span className="font-mono tabular-nums">{totalRequests.toLocaleString()}</span> requests · all healthy</>
          ) : (
            <>
              <span className="font-mono tabular-nums">{endpoints.length}</span> endpoints · <span className="font-mono tabular-nums">{totalRequests.toLocaleString()}</span> requests · <span className="text-red-600 font-semibold font-mono tabular-nums">{totalErrors}</span> errors
            </>
          )}
        </p>
      </div>

      {endpoints.length === 0 ? (
        <div className="bg-white border border-black/10 py-16 text-center">
          <Inbox className="w-10 h-10 text-black/20 mx-auto mb-4" />
          <p className="font-semibold text-black mb-1">Quiet so far</p>
          <p className="text-sm text-black/50">HTTP requests will show up once your app sends them.</p>
        </div>
      ) : (
        <div className="bg-white border border-black/10 divide-y divide-black/10 overflow-hidden">
          {endpoints.map((endpoint) => (
            <EndpointRow key={endpoint.path} endpoint={endpoint} appId={appId} />
          ))}
        </div>
      )}
    </Layout>
  )
}
