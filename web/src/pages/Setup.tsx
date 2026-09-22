import { useState } from 'react'
import { Layout } from '@/components/layout'
import { AuthShell, FIELD, SUBMIT, Notice } from '@/components/auth-shell'
import { uptimeLabel, percent, type UptimeState } from '@/lib/uptime'

interface APIKey {
  id: number
  name: string
  key: string
  key_prefix: string
  active: boolean
  today_total: number
  last_seen: number | null
  ping_url?: string
  ping_disabled?: boolean
  is_down?: boolean
  uptime?: UptimeState
}

// The server sends one flash message: { type, message }.
interface Flash {
  type?: string
  message?: string
}

interface Props {
  apiKeys?: APIKey[]
  flash?: Flash
}

// "[copy]" that confirms for 2 seconds, and only once the clipboard took it.
function CopyButton({ text, className = '' }: { text: string; className?: string }) {
  const [copied, setCopied] = useState(false)
  const copy = () =>
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  return (
    <button type="button" onClick={copy} className={`key shrink-0 ${className}`}>
      {copied ? 'copied' : 'copy'}
    </button>
  )
}

export default function Setup({ apiKeys = [], flash }: Props) {
  const newKey = flash?.type === 'newKey' ? flash.message : undefined
  const flashError = flash?.type === 'error' ? flash.message : undefined
  const activeApps = apiKeys.filter((k) => k.active)

  // First app: a quiet page without the header.
  if (activeApps.length === 0 && !newKey) {
    return <FirstApp error={flashError} />
  }

  return (
    <Layout currentPath="/apps">
      <div className="mb-8">
        <h1 className="text-[26px] font-bold leading-tight tracking-tight text-foreground">Apps</h1>
        <p className="mt-1 text-[13px] text-subtle">
          <span className="tabular-nums">{activeApps.length}</span> app{activeApps.length !== 1 ? 's' : ''} configured
        </p>
      </div>

      {flashError && <p className="mb-6 border border-error px-4 py-2.5 text-[13px] text-error">{flashError}</p>}

      {newKey && (
        <div className="pane mb-8 border-accent px-4 pb-4 pt-6">
          <span className="pane-title text-accent">new api key</span>
          <p className="mb-3 text-[13px] text-text">Copy it now. You won't see it again.</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 select-all break-all border border-border bg-raise/40 px-3 py-2 text-[13px] text-foreground">{newKey}</code>
            <CopyButton text={newKey} />
          </div>
        </div>
      )}

      <div className="pane pb-0 pt-3.5">
        <span className="pane-title">apps</span>
        {activeApps.map((app) => (
          <AppRow key={app.id} app={app} />
        ))}
        <AddApp />
      </div>
    </Layout>
  )
}

function AppRow({ app }: { app: APIKey }) {
  const [showKey, setShowKey] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const [editingURL, setEditingURL] = useState(false)

  const hasData = app.last_seen !== null
  const recent = app.last_seen !== null && Date.now() / 1000 - app.last_seen < 3600

  // Uptime: a ping every minute when the app has a URL.
  const uptime = uptimeLabel(app.uptime)

  const confirmDelete = (e: React.FormEvent) => {
    if (!confirm(`Delete "${app.name}"? This will stop tracking for this app.`)) e.preventDefault()
  }

  return (
    <div className="group border-t border-line px-[18px] py-3.5 first-of-type:border-t-0">
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
        <span
          className={`h-2.5 w-2.5 shrink-0 ${recent ? 'bg-accent' : hasData ? 'bg-border' : 'border border-dashed border-subtle'}`}
          title={recent ? 'Events in the last hour' : hasData ? 'No events in the last hour' : 'Waiting for events'}
        />

        <div className="min-w-0 flex-1">
          {renaming ? (
            <form action={`/apps/api-keys/${app.id}/rename`} method="POST" className="flex items-center gap-2">
              <input
                type="text"
                name="name"
                defaultValue={app.name}
                placeholder="App name"
                maxLength={60}
                autoFocus
                onKeyDown={(e) => e.key === 'Escape' && setRenaming(false)}
                className={`${FIELD} max-w-xs font-bold`}
              />
              <button type="submit" className="key">
                save
              </button>
              <button type="button" onClick={() => setRenaming(false)} className="text-xs text-dim hover:text-foreground">
                cancel
              </button>
            </form>
          ) : (
            <p className="flex items-baseline gap-3">
              <a href={`/apps/${app.id}/issues`} className="text-[15px] font-bold text-foreground hover:underline">
                {app.name}
              </a>
              <button
                type="button"
                onClick={() => setRenaming(true)}
                className="text-xs text-dim opacity-0 hover:text-foreground focus:opacity-100 group-hover:opacity-100"
              >
                rename
              </button>
            </p>
          )}

          <p className="mt-1 flex flex-wrap items-center gap-x-2 text-xs text-dim">
            <span>
              {app.today_total > 0 ? (
                <>
                  <span className="tabular-nums text-subtle">{app.today_total.toLocaleString()}</span> today
                </>
              ) : hasData ? (
                'no events today'
              ) : (
                'waiting for events'
              )}
            </span>
            <span>·</span>
            <span className="flex min-w-0 items-center gap-1.5">
              <i className={`h-[7px] w-[7px] shrink-0 ${uptime.dot}`} />
              <span className={uptime.bad ? 'font-bold text-error' : 'text-subtle'}>{uptime.text}</span>
              {app.uptime?.uptime_24h !== undefined && <span>· {percent(app.uptime.uptime_24h)} 24h</span>}
              {app.ping_url && (
                <span className="max-w-[28ch] truncate" title={app.ping_url}>
                  · {app.ping_url}
                </span>
              )}
            </span>
            <button type="button" onClick={() => setEditingURL(!editingURL)} className="hover:text-foreground">
              [{editingURL ? 'close' : 'edit'}]
            </button>
          </p>
        </div>

        <div className="flex shrink-0 items-center gap-3">
          <button type="button" onClick={() => setShowKey(!showKey)} className="key">
            {showKey ? 'Hide key' : 'API Key'}
          </button>
          <form action={`/apps/api-keys/${app.id}/delete`} method="POST" onSubmit={confirmDelete}>
            <button type="submit" className="text-xs text-dim opacity-0 hover:text-error focus:opacity-100 group-hover:opacity-100" title="Delete app">
              delete
            </button>
          </form>
        </div>
      </div>

      {editingURL && (
        <div className="ml-[30px] mt-3 flex flex-wrap items-center gap-3 border-t border-dashed border-line pt-3">
          <form action={`/apps/api-keys/${app.id}/ping-url`} method="POST" className="flex min-w-[280px] flex-1 items-center gap-2">
            <input type="text" name="url" defaultValue={app.ping_url ?? ''} placeholder="https://yourapp.com" className={FIELD} />
            <button type="submit" className="key">
              save
            </button>
          </form>
          <form action={`/apps/api-keys/${app.id}/toggle-ping`} method="POST">
            <input type="hidden" name="disabled" value={app.ping_disabled ? 'false' : 'true'} />
            <button type="submit" className="text-xs text-subtle hover:text-foreground">
              {app.ping_disabled ? 'turn uptime check on' : 'turn uptime check off'}
            </button>
          </form>
          <p className="w-full text-xs text-dim">LogNorth pings this URL every minute. 3 failures in a row means down.</p>
        </div>
      )}

      {showKey && (
        <div className="ml-[30px] mt-3 flex items-center gap-2">
          <code className="flex-1 select-all break-all border border-border bg-raise/40 px-3 py-2 text-[13px] text-foreground">{app.key}</code>
          <CopyButton text={app.key} />
        </div>
      )}
    </div>
  )
}

function AddApp() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [url, setURL] = useState('')

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="block w-full border-t border-dashed border-border px-[18px] py-3 text-left text-[13px] text-subtle hover:text-foreground"
      >
        + add app
      </button>
    )
  }

  const close = (e: React.KeyboardEvent) => e.key === 'Escape' && setOpen(false)
  return (
    <form action="/apps/api-keys" method="POST" className="flex flex-col gap-2.5 border-t border-dashed border-border px-[18px] py-3.5">
      <div className="flex gap-2.5">
        <input type="text" name="name" placeholder="App name (e.g. shop-prod)" value={name} onChange={(e) => setName(e.target.value)} onKeyDown={close} autoFocus className={FIELD} />
        <button type="submit" disabled={!name.trim()} className="shrink-0 bg-foreground px-4 text-[13px] font-bold text-background hover:bg-text disabled:opacity-40">
          Create
        </button>
        <button type="button" onClick={() => setOpen(false)} className="shrink-0 text-xs text-dim hover:text-foreground">
          cancel
        </button>
      </div>
      <input type="url" name="url" placeholder="App URL for uptime pings (optional)" value={url} onChange={(e) => setURL(e.target.value)} onKeyDown={close} className={FIELD} />
      <p className="text-xs text-dim">One app per environment: shop-prod and shop-preview are two apps.</p>
    </form>
  )
}

function FirstApp({ error }: { error?: string }) {
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  return (
    <AuthShell title="~ $ lognorth apps add">
      <h1 className="mb-5 text-lg font-bold text-foreground">Add your first app</h1>
      {error && <Notice kind="error">{error}</Notice>}
      <form action="/apps/api-keys" method="POST" className="space-y-3">
        <input type="text" name="name" placeholder="App name (e.g. shop-prod)" value={name} onChange={(e) => setName(e.target.value)} autoFocus className={FIELD} />
        <input type="url" name="url" placeholder="App URL for uptime pings (optional)" value={url} onChange={(e) => setURL(e.target.value)} className={FIELD} />
        <button type="submit" disabled={!name.trim()} className={SUBMIT}>
          Create app
        </button>
      </form>
    </AuthShell>
  )
}
