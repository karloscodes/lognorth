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
      {/* Status */}
      <div className="mb-10">
        {totalErrors > 0 ? (
          <div>
            <p className="text-3xl font-bold text-red-600">
              <span className="font-mono tabular-nums">{totalErrors}</span> errors
            </p>
            <p className="text-sm text-black/50 mt-1">Time to fix something.</p>
          </div>
        ) : (
          <div>
            <p className="text-3xl font-bold text-[#00D678]">
              All clear
            </p>
            <p className="text-sm text-black/50 mt-1">Nice work.</p>
          </div>
        )}
      </div>

      {/* Apps list */}
      <div className="space-y-3">
        {applications.map((app) => {
          const errors = app.stats?.errors || 0

          return (
            <a
              key={app.id}
              href={`/apps/${app.id}/issues`}
              className={`flex items-center justify-between p-5 border-2 transition-colors ${
                errors > 0
                  ? 'border-red-200 bg-red-50 hover:border-red-300'
                  : 'border-black/10 hover:border-black/20'
              }`}
            >
              <span className="font-bold text-black text-lg">{app.name}</span>

              {errors > 0 ? (
                <span className="font-mono tabular-nums text-lg font-bold text-red-600">
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
        <a href="/apps" className="text-sm font-semibold text-black/50 hover:text-black">
          + Add app
        </a>
      </div>
    </Layout>
  )
}
