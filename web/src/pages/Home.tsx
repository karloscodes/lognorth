import { Layout } from '@/components/layout'
import type { Application } from '@/lib/shared'

interface Props {
  applications?: Application[]
}

export default function Home({ applications = [] }: Props) {
  const totalErrors = applications.reduce((sum, app) => sum + (app.stats?.errors || 0), 0)

  // No apps yet - onboarding
  if (applications.length === 0) {
    return (
      <Layout currentPath="/">
        <div className="py-20 max-w-md">
          <h1 className="text-3xl font-bold text-black mb-3">
            Know when things break.
          </h1>
          <p className="text-black/60 mb-6">
            Your errors. Your server. Your data.
          </p>
          <a
            href="/apps"
            className="inline-flex px-5 py-2.5 bg-black text-white font-semibold hover:bg-black/80 transition-colors"
          >
            Add your first app
          </a>
        </div>
      </Layout>
    )
  }

  return (
    <Layout currentPath="/" applications={applications}>
      {/* Hero Status - THE answer to "do I have a problem?" */}
      <div className="mb-12 py-8 border-b-2 border-black/5">
        {totalErrors > 0 ? (
          <div>
            <p className="text-6xl font-black text-red-600 tracking-tight">
              <span className="font-mono tabular-nums">{totalErrors}</span>
            </p>
            <p className="text-xl font-semibold text-black mt-2">
              error{totalErrors !== 1 ? 's' : ''} to fix
            </p>
          </div>
        ) : (
          <div>
            <p className="text-6xl font-black text-[#00D678] tracking-tight">
              0
            </p>
            <p className="text-xl font-semibold text-black mt-2">
              errors. Nice work.
            </p>
          </div>
        )}
      </div>

      {/* Apps list */}
      <div className="space-y-3">
        {applications.map((app) => {
          const errors = app.stats?.errors || 0
          const hasErrors = errors > 0

          return (
            <a
              key={app.id}
              href={`/apps/${app.id}/issues`}
              className={`group flex items-center justify-between p-5 border-2 transition-all ${
                hasErrors
                  ? 'border-red-500/30 bg-red-50 hover:border-red-500'
                  : 'border-black/10 hover:border-black/30 hover:bg-black/[0.01]'
              }`}
            >
              <span className="font-bold text-black text-lg">{app.name}</span>

              {hasErrors ? (
                <span className="font-mono tabular-nums text-2xl font-black text-red-600">
                  {errors}
                </span>
              ) : (
                <span className="w-3 h-3 bg-[#00D678]" />
              )}
            </a>
          )
        })}
      </div>

      <div className="mt-8">
        <a href="/apps" className="text-sm font-semibold text-black/50 hover:text-black transition-colors">
          + Add app
        </a>
      </div>
    </Layout>
  )
}
