import { useEffect, useRef, useState } from 'react'
import { Copy, Check, Trash2, Key, Activity, Pencil, X } from 'lucide-react'
import { router } from '@inertiajs/react'
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
  const [editingId, setEditingId] = useState<number | null>(null)
  const [draftName, setDraftName] = useState('')
  const editInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editingId !== null) editInputRef.current?.focus()
  }, [editingId])

  const startEdit = (app: APIKey) => {
    setEditingId(app.id)
    setDraftName(app.name)
  }
  const cancelEdit = () => {
    setEditingId(null)
    setDraftName('')
  }
  const saveEdit = (appId: number) => {
    const trimmed = draftName.trim()
    if (!trimmed) return
    router.post(
      `/apps/api-keys/${appId}/rename`,
      { name: trimmed },
      {
        preserveScroll: true,
        onFinish: () => setEditingId(null),
      },
    )
  }

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
            <span className="text-4xl font-black font-mono tracking-tight" style={{ WebkitTextStroke: '0.5px currentColor' }}><span className="text-accent">[</span>lognorth<span className="text-accent">]</span></span>
            <p className="text-sm font-medium text-black/60 mt-2">Error tracking you own.</p>
          </div>

          <h1 className="text-xl font-bold text-black text-center mb-6">Add your first app</h1>

          {flash?.error && (
            <div className="bg-[#e11d48]/10 border-2 border-[#e11d48]/20 text-[#e11d48] px-4 py-3 text-sm font-medium mb-4">
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
      <div>
        {/* Header */}
        <div className="mb-8">
          <h1 className="text-2xl font-bold text-black">Apps</h1>
          <p className="text-sm text-black/50 mt-1">
            <span className="font-mono tabular-nums">{activeApps.length}</span> app{activeApps.length !== 1 ? 's' : ''} configured
          </p>
        </div>

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
        <div className="grid gap-4">
          {activeApps.map((app) => {
            const isActive = app.last_seen && (Date.now() / 1000 - app.last_seen) < 3600
            const isShowingKey = showKeyFor === app.id
            const hasData = app.last_seen !== null

            return (
              <div
                key={app.id}
                className={`bg-white border-2 overflow-hidden group transition-all ${
                  isActive
                    ? 'border-accent/40 border-l-4 border-l-accent hover:border-accent/60'
                    : 'border-black/10 hover:border-black/30'
                }`}
              >
                {/* Main row */}
                <div className="flex items-center gap-5 p-5">
                  {/* Status indicator */}
                  <div className="w-3 h-3 shrink-0">
                    {isActive ? (
                      <span className="block w-full h-full bg-accent" />
                    ) : hasData ? (
                      <span className="block w-full h-full bg-black/20" />
                    ) : (
                      <span className="block w-full h-full border-2 border-dashed border-black/30" />
                    )}
                  </div>

                  {/* App info */}
                  <div className="flex-1 min-w-0">
                    {editingId === app.id ? (
                      <form
                        onSubmit={(e) => { e.preventDefault(); saveEdit(app.id) }}
                        className="flex items-center gap-2"
                      >
                        <input
                          ref={editInputRef}
                          type="text"
                          name="name"
                          value={draftName}
                          maxLength={60}
                          onChange={(e) => setDraftName(e.target.value)}
                          onKeyDown={(e) => { if (e.key === 'Escape') cancelEdit() }}
                          className="flex-1 min-w-0 px-2 py-1 text-lg font-bold text-black border-2 border-accent bg-white focus:outline-none"
                        />
                        <button
                          type="submit"
                          className="px-3 py-1 text-sm font-semibold text-black border border-black/20 hover:border-black/40 transition-colors"
                        >
                          Save
                        </button>
                        <button
                          type="button"
                          onClick={cancelEdit}
                          className="p-1.5 text-black/40 hover:text-black transition-colors"
                          aria-label="Cancel"
                        >
                          <X className="w-4 h-4" />
                        </button>
                      </form>
                    ) : (
                      <div className="flex items-center gap-2">
                        <a href={`/apps/${app.id}/issues`} className="block">
                          <span className="text-lg font-bold text-black hover:underline">{app.name}</span>
                        </a>
                        <button
                          type="button"
                          onClick={() => startEdit(app)}
                          className="p-1 opacity-0 group-hover:opacity-100 text-black/30 hover:text-black transition-all"
                          title="Rename"
                        >
                          <Pencil className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    )}
                    <div className="flex items-center gap-4 mt-1 text-sm text-black/50">
                      <span className="flex items-center gap-1.5">
                        <Activity className="w-3.5 h-3.5" />
                        {app.today_total > 0 ? (
                          <><span className="font-mono tabular-nums">{app.today_total.toLocaleString()}</span> today</>
                        ) : hasData ? (
                          'No activity today'
                        ) : (
                          'Waiting for events'
                        )}
                      </span>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-3 shrink-0">
                    <button
                      type="button"
                      onClick={() => setShowKeyFor(isShowingKey ? null : app.id)}
                      className="flex items-center gap-1.5 text-sm text-black/50 hover:text-black px-3 py-1.5 border border-black/10 hover:border-black/20 transition-colors"
                    >
                      <Key className="w-3.5 h-3.5" />
                      {isShowingKey ? 'Hide' : 'API Key'}
                    </button>
                    <form
                      action={`/apps/api-keys/${app.id}`}
                      method="POST"
                      onSubmit={(e) => {
                        if (!confirm(`Delete "${app.name}"? This will stop tracking for this app.`)) {
                          e.preventDefault()
                        }
                      }}
                    >
                      <input type="hidden" name="_method" value="DELETE" />
                      <button
                        type="submit"
                        className="p-2 opacity-0 group-hover:opacity-100 text-black/30 hover:text-[#e11d48] hover:bg-rose-50 transition-all"
                        title="Delete"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </form>
                  </div>
                </div>

                {/* API Key display */}
                {isShowingKey && (
                  <div className="px-5 pb-5 pt-0">
                    <div className="flex items-center gap-2 p-3 bg-black/[0.02] border border-black/5">
                      <code className="flex-1 text-sm font-mono break-all select-all text-black">
                        {app.key}
                      </code>
                      <button
                        type="button"
                        onClick={() => {
                          navigator.clipboard.writeText(app.key)
                          setCopiedKey(app.id)
                          setTimeout(() => setCopiedKey(null), 2000)
                        }}
                        className="p-2 bg-white border border-black/20 hover:border-black/40 transition-colors"
                      >
                        {copiedKey === app.id ? <Check className="w-4 h-4 text-accent" /> : <Copy className="w-4 h-4 text-black/70" />}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )
          })}

          {/* Add app card */}
          <AddAppCard />
        </div>
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
        className="w-full p-5 text-left border-2 border-dashed border-black/10 hover:border-black/30 hover:bg-black/[0.01] transition-colors"
      >
        <span className="text-lg font-semibold text-black/50">+ Add app</span>
      </button>
    )
  }

  return (
    <div className="p-5 border border-black/10 bg-white">
      <form action="/apps/api-keys" method="POST" className="flex gap-3">
        <input
          type="text"
          name="name"
          placeholder="App name (e.g. Production)"
          value={appName}
          onChange={(e) => setAppName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') setShowForm(false)
          }}
          autoFocus
          className="flex-1 px-4 py-2.5 bg-white border border-black/20 focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
        />
        <button
          type="submit"
          disabled={!appName.trim()}
          className="px-5 py-2.5 bg-black text-white font-semibold hover:bg-black/80 disabled:opacity-40 transition-colors"
        >
          Create
        </button>
        <button
          type="button"
          onClick={() => setShowForm(false)}
          className="px-4 py-2.5 text-black/50 hover:text-black transition-colors"
        >
          Cancel
        </button>
      </form>
    </div>
  )
}
