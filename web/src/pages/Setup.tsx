import { useState } from 'react'
import { Copy, Check, Trash2 } from 'lucide-react'
import { Layout } from '@/components/layout'

interface APIKey {
  id: number
  name: string
  key: string
  key_prefix: string
  active: boolean
  today_total: number
  last_seen: number | null
}

interface Flash {
  error?: string
  success?: string
  newKey?: string
}

interface Props {
  apiKeys?: APIKey[]
  flash?: Flash
}

export default function Setup({ apiKeys = [], flash }: Props) {
  const [appName, setAppName] = useState('')
  const [copied, setCopied] = useState(false)
  const [showKeyFor, setShowKeyFor] = useState<number | null>(null)
  const [copiedKey, setCopiedKey] = useState<number | null>(null)

  const copyKey = () => {
    if (flash?.newKey) {
      navigator.clipboard.writeText(flash.newKey)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  const activeApps = apiKeys.filter(k => k.active)
  const hasApps = activeApps.length > 0

  // First-time setup: clean page without header
  if (!hasApps && !flash?.newKey) {
    return (
      <div className="min-h-screen bg-white flex items-center justify-center">
        <div className="w-full max-w-sm px-6">
          <div className="text-center mb-8">
            <span className="text-4xl font-bold tracking-tight">LogNorth</span>
            <p className="text-sm font-medium text-black/60 mt-2">Smarter logs. Self-hosted.</p>
          </div>

          <h1 className="text-xl font-bold text-black text-center mb-6">Add your first app</h1>

          {flash?.error && (
            <div className="bg-red-500/10 border-2 border-red-500/20 text-red-600 px-4 py-3 text-sm font-medium mb-4">
              {flash.error}
            </div>
          )}

          <form action="/apps/api-keys" method="POST" className="space-y-4">
            <input
              type="text"
              name="name"
              placeholder="App name (e.g. Production)"
              value={appName}
              onChange={(e) => setAppName(e.target.value)}
              autoFocus
              className="w-full px-4 py-3 border-2 border-black/20 text-sm focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent placeholder:text-black/40"
            />
            <button
              type="submit"
              disabled={!appName.trim()}
              className="w-full py-3 bg-black text-white font-semibold hover:bg-black/80 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              Create app
            </button>
          </form>
        </div>
      </div>
    )
  }

  return (
    <Layout currentPath="/apps">
      <div className="max-w-xl">
        {/* New key alert */}
        {flash?.newKey && (
          <section className="mb-8">
            <div className="border-2 border-black p-5">
              <p className="font-semibold text-black mb-1">Copy your API key</p>
              <p className="text-sm text-black/60 mb-4">You won't see this again.</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 bg-black/[0.03] p-3 text-sm font-mono break-all select-all text-black">
                  {flash.newKey}
                </code>
                <button
                  onClick={copyKey}
                  type="button"
                  className="p-3 border border-black/20 hover:bg-black/[0.02] transition-colors"
                >
                  {copied ? <Check className="w-5 h-5" /> : <Copy className="w-5 h-5 text-black/70" />}
                </button>
              </div>
            </div>
          </section>
        )}

        {/* Apps grid */}
        <section className="mb-8">
          <div className="grid grid-cols-2 gap-4">
            {activeApps.map((app) => {
              const isActive = app.last_seen && (Date.now() / 1000 - app.last_seen) < 3600
              const isShowingKey = showKeyFor === app.id
              return (
                <div
                  key={app.id}
                  className="group relative border-2 border-black/10 hover:border-black/20 p-5 transition-colors"
                >
                  <a href={`/apps/${app.id}/issues`} className="block">
                    <div className="flex items-start justify-between mb-3">
                      <span className="text-lg font-semibold text-black">{app.name}</span>
                      {isActive && <span className="w-2.5 h-2.5 rounded-full bg-[#00D678] mt-1.5" />}
                    </div>
                    <div className="text-sm text-black/80">
                      {app.today_total > 0 ? (
                        <span>{app.today_total.toLocaleString()} requests today</span>
                      ) : app.last_seen ? (
                        <span>No activity today</span>
                      ) : (
                        <span>Waiting for data</span>
                      )}
                    </div>
                  </a>

                  {/* Show key section */}
                  {isShowingKey ? (
                    <div className="mt-3 pt-3 border-t border-black/10">
                      <div className="flex items-center gap-2">
                        <code className="flex-1 bg-black/[0.03] p-2 text-xs font-mono break-all select-all text-black">
                          {app.key}
                        </code>
                        <button
                          type="button"
                          onClick={() => {
                            navigator.clipboard.writeText(app.key)
                            setCopiedKey(app.id)
                            setTimeout(() => setCopiedKey(null), 2000)
                          }}
                          className="p-2 border border-black/20 hover:bg-black/[0.02] transition-colors"
                        >
                          {copiedKey === app.id ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4 text-black/70" />}
                        </button>
                      </div>
                      <button
                        type="button"
                        onClick={() => setShowKeyFor(null)}
                        className="text-xs text-black/60 hover:text-black mt-2"
                      >
                        Hide
                      </button>
                    </div>
                  ) : (
                    <button
                      type="button"
                      onClick={() => setShowKeyFor(app.id)}
                      className="mt-3 text-xs text-black/60 hover:text-black"
                    >
                      Show API key
                    </button>
                  )}

                  <form
                    action={`/apps/api-keys/${app.id}`}
                    method="POST"
                    onSubmit={(e) => {
                      if (!confirm(`Delete "${app.name}"? This will stop tracking for this app.`)) {
                        e.preventDefault()
                      }
                    }}
                    className="absolute top-3 right-3"
                  >
                    <input type="hidden" name="_method" value="DELETE" />
                    <button
                      type="submit"
                      className="p-2 opacity-0 group-hover:opacity-100 text-black/50 hover:text-red-600 transition-all"
                      title="Delete"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </form>
                </div>
              )
            })}

            {/* Add app card */}
            <AddAppCard />
          </div>
        </section>
      </div>
    </Layout>
  )
}

function AddAppCard() {
  const [showForm, setShowForm] = useState(false)
  const [appName, setAppName] = useState('')

  if (!showForm) {
    return (
      <button
        onClick={() => setShowForm(true)}
        className="border-2 border-dashed border-black/20 hover:border-black/40 p-5 text-black/70 hover:text-black transition-colors text-left"
      >
        <span className="text-lg font-semibold">+ Add app</span>
      </button>
    )
  }

  return (
    <div className="border-2 border-black/20 p-5">
      <form action="/apps/api-keys" method="POST">
        <input
          type="text"
          name="name"
          placeholder="App name"
          value={appName}
          onChange={(e) => setAppName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') setShowForm(false)
          }}
          autoFocus
          className="w-full px-3 py-2 bg-white border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40 mb-3"
        />
        <div className="flex gap-2">
          <button
            type="submit"
            disabled={!appName.trim()}
            className="flex-1 px-3 py-2 bg-black text-white text-sm font-medium hover:bg-black/80 disabled:opacity-40 transition-colors"
          >
            Add
          </button>
          <button
            type="button"
            onClick={() => setShowForm(false)}
            className="px-3 py-2 text-sm text-black/60 hover:text-black transition-colors"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  )
}
