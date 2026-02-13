import { useState } from 'react'
import { AlertTriangle, CheckCircle, Activity, TrendingUp, ChevronDown } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
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

interface AlertStatus {
  path: string
  status: 'learning' | 'healthy' | 'warning' | 'critical'
  sample_count: number
  samples_needed: number
  baseline_mean?: number
  baseline_stddev?: number
  current_rate?: number
  since_spiking?: string
}

interface Props {
  endpoints: EndpointStats[]
  alertStatuses?: AlertStatus[]
  appId: number
  appName: string
  applications?: Application[]
}

function EndpointRow({ endpoint, appId }: { endpoint: EndpointStats; appId: number }) {
  const [expanded, setExpanded] = useState(false)
  const [recentErrors, setRecentErrors] = useState<RecentError[]>([])
  const [loading, setLoading] = useState(false)

  const isHealthy = endpoint.error_rate === 0
  const isCritical = endpoint.error_rate >= 10

  const handleClick = async () => {
    if (expanded) {
      setExpanded(false)
      return
    }

    setExpanded(true)

    // Only fetch if we have errors and haven't loaded yet
    if (endpoint.errors > 0 && recentErrors.length === 0) {
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
    <div className={`border-b border-black/5 last:border-b-0 ${isCritical ? 'bg-red-500/[0.03]' : ''}`}>
      <button
        onClick={handleClick}
        className="w-full grid grid-cols-12 gap-4 px-5 py-4 text-left hover:bg-black/[0.02] transition-colors"
      >
        <div className="col-span-6 flex items-center gap-3">
          <ChevronDown className={`w-4 h-4 text-black/30 transition-transform ${expanded ? 'rotate-180' : ''}`} />
          {isHealthy ? (
            <CheckCircle className="w-4 h-4 text-[#00D678] shrink-0" />
          ) : (
            <AlertTriangle className={`w-4 h-4 shrink-0 ${isCritical ? 'text-red-500' : 'text-amber-500'}`} />
          )}
          <code className="font-mono text-sm text-black truncate">{endpoint.path}</code>
        </div>
        <div className="col-span-2 text-right font-mono text-sm text-black/70">
          {endpoint.total.toLocaleString()}
        </div>
        <div className={`col-span-2 text-right font-mono text-sm font-semibold ${
          isHealthy ? 'text-black/40' : isCritical ? 'text-red-600' : 'text-amber-600'
        }`}>
          {endpoint.error_rate}%
        </div>
        <div className="col-span-2 text-right font-mono text-sm text-black/70">
          {endpoint.avg_duration}ms
        </div>
      </button>

      {expanded && (
        <div className="px-5 pb-4 pt-1 border-t border-black/5 bg-black/[0.01]">
          <div className="pl-11 space-y-3">
            {/* Stats summary */}
            <div className="flex gap-6 text-sm">
              <div>
                <span className="text-black/50">Total requests</span>
                <span className="ml-2 font-semibold text-black">{endpoint.total.toLocaleString()}</span>
              </div>
              <div>
                <span className="text-black/50">Errors</span>
                <span className="ml-2 font-semibold text-red-600">{endpoint.errors.toLocaleString()}</span>
              </div>
              <div>
                <span className="text-black/50">Avg response</span>
                <span className="ml-2 font-semibold text-black">{endpoint.avg_duration}ms</span>
              </div>
            </div>

            {/* Recent errors */}
            {endpoint.errors > 0 && (
              <div>
                <p className="text-xs font-semibold text-black/50 uppercase tracking-wide mb-2">Recent errors</p>
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
    </div>
  )
}

export default function Endpoints({ endpoints: endpointsProp, alertStatuses, appId, appName, applications }: Props) {
  // Normalize null to empty array (Go sends null for empty slices)
  const endpoints = endpointsProp || []
  const problemCount = endpoints.filter(e => e.error_rate > 0).length
  const totalRequests = endpoints.reduce((sum, e) => sum + e.total, 0)
  const totalErrors = endpoints.reduce((sum, e) => sum + e.errors, 0)
  const healthPercent = totalRequests > 0 ? ((1 - totalErrors / totalRequests) * 100).toFixed(1) : 100

  // Build a map for quick lookup
  const alertByPath = new Map(alertStatuses?.map(s => [s.path, s]) ?? [])

  const getStatusBadge = (status: AlertStatus['status']) => {
    switch (status) {
      case 'learning':
        return <span className="px-2 py-0.5 text-xs font-medium bg-black/5 text-black/60 rounded">Learning</span>
      case 'healthy':
        return <span className="px-2 py-0.5 text-xs font-medium bg-[#00D678]/10 text-[#00D678] rounded">Healthy</span>
      case 'warning':
        return <span className="px-2 py-0.5 text-xs font-medium bg-amber-500/10 text-amber-600 rounded">Warning</span>
      case 'critical':
        return <span className="px-2 py-0.5 text-xs font-medium bg-red-500/10 text-red-600 rounded">Critical</span>
    }
  }

  return (
    <Layout currentPath="/endpoints" appId={appId} appName={appName} applications={applications}>
      {/* Header with health score */}
      <div className="mb-8">
        <div className="flex items-baseline gap-3">
          <span className={`text-4xl font-bold ${Number(healthPercent) >= 99 ? 'text-[#00D678]' : Number(healthPercent) >= 95 ? 'text-amber-500' : 'text-red-500'}`}>
            {healthPercent}%
          </span>
          <span className="text-black/60">healthy</span>
        </div>
        {problemCount > 0 && (
          <p className="text-sm text-black/50 mt-2">
            {problemCount} endpoint{problemCount !== 1 ? 's' : ''} with errors
          </p>
        )}
      </div>

      {/* Alerting Status Section */}
      {alertStatuses && alertStatuses.length > 0 && (
        <div className="mb-8">
          <h2 className="text-lg font-semibold text-black mb-4 flex items-center gap-2">
            <Activity className="w-5 h-5" />
            Alerting Status
          </h2>
          <div className="border-2 border-black/10 rounded-sm overflow-hidden">
            <div className="grid grid-cols-12 gap-4 px-5 py-3 bg-black/[0.02] border-b border-black/10 text-xs font-semibold text-black/60 uppercase tracking-wide">
              <div className="col-span-5">Endpoint</div>
              <div className="col-span-2">Status</div>
              <div className="col-span-3">Baseline</div>
              <div className="col-span-2 text-right">Current</div>
            </div>
            {alertStatuses.map((alert) => (
              <div
                key={alert.path}
                className={`grid grid-cols-12 gap-4 px-5 py-3 border-b border-black/5 last:border-b-0 ${
                  alert.status === 'critical' ? 'bg-red-500/[0.03]' :
                  alert.status === 'warning' ? 'bg-amber-500/[0.03]' : ''
                }`}
              >
                <div className="col-span-5">
                  <code className="font-mono text-sm text-black truncate">{alert.path}</code>
                </div>
                <div className="col-span-2 flex items-center gap-2">
                  {getStatusBadge(alert.status)}
                  {alert.since_spiking && (
                    <span className="text-xs text-black/40">{alert.since_spiking}</span>
                  )}
                </div>
                <div className="col-span-3 text-sm text-black/60">
                  {alert.status === 'learning' ? (
                    <span className="flex items-center gap-1">
                      <TrendingUp className="w-3 h-3" />
                      {alert.sample_count}/{alert.samples_needed} samples
                    </span>
                  ) : alert.baseline_mean !== undefined ? (
                    <span className="font-mono">{alert.baseline_mean}% ± {alert.baseline_stddev}%</span>
                  ) : (
                    <span className="text-black/30">—</span>
                  )}
                </div>
                <div className="col-span-2 text-right font-mono text-sm">
                  {alert.current_rate !== undefined ? (
                    <span className={
                      alert.status === 'critical' ? 'text-red-600 font-semibold' :
                      alert.status === 'warning' ? 'text-amber-600 font-semibold' :
                      'text-black/70'
                    }>
                      {alert.current_rate.toFixed(1)}%
                    </span>
                  ) : (
                    <span className="text-black/30">—</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {endpoints.length === 0 ? (
        <div className="py-16 text-center border-2 border-dashed border-black/10">
          <p className="text-black/40">No HTTP requests recorded yet</p>
        </div>
      ) : (
        <div className="border-2 border-black/10 rounded-sm overflow-hidden">
          {/* Header row */}
          <div className="grid grid-cols-12 gap-4 px-5 py-3 bg-black/[0.02] border-b border-black/10 text-xs font-semibold text-black/60 uppercase tracking-wide">
            <div className="col-span-6">Endpoint</div>
            <div className="col-span-2 text-right">Requests</div>
            <div className="col-span-2 text-right">Error Rate</div>
            <div className="col-span-2 text-right">Avg Duration</div>
          </div>

          {/* Data rows */}
          {endpoints.map((endpoint) => (
            <EndpointRow key={endpoint.path} endpoint={endpoint} appId={appId} />
          ))}
        </div>
      )}
    </Layout>
  )
}
