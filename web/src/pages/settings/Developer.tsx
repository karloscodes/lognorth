import { useState } from 'react'
import { SettingsLayout } from '@/components/settings-layout'

interface Props {
  agentKeyPrefix?: string
  githubConfigured?: boolean
  githubLogin?: string
  openrouterConfigured?: boolean
  openrouterLabel?: string
  openrouterModel?: string
  openrouterModels?: string[]
  openrouterDefault?: string
}

const ORIGIN = typeof window !== 'undefined' ? window.location.origin : 'https://your-instance.com'

export default function SettingsDeveloper({
  agentKeyPrefix: initialPrefix = '',
  githubConfigured: initialGH = false,
  githubLogin: initialGHLogin = '',
  openrouterConfigured: initialOR = false,
  openrouterLabel: initialORLabel = '',
  openrouterModel: initialORModel = '',
  openrouterModels = [],
  openrouterDefault = '',
}: Props) {
  const [tab, setTab] = useState<'machine' | 'server'>('machine')

  return (
    <SettingsLayout currentPath="/settings/developer">
      <header className="mb-6">
        <p className="text-xs font-bold uppercase tracking-wide text-foreground/40 mb-2">Developer</p>
        <h2 className="text-2xl font-bold text-foreground">
          Fix issues from your logs<span className="text-accent">.</span>
        </h2>
        <p className="text-foreground/50 text-sm mt-2">Pick where the agent runs.</p>
      </header>

      <div className="grid grid-cols-2 border border-foreground/15 mb-8">
        <TabButton active={tab === 'machine'} onClick={() => setTab('machine')}>
          On your machine
        </TabButton>
        <TabButton active={tab === 'server'} onClick={() => setTab('server')} divider>
          On your server
        </TabButton>
      </div>

      {tab === 'machine' ? (
        <AgentKeyTab initialPrefix={initialPrefix} />
      ) : (
        <ServerTab
          initialGH={initialGH}
          initialGHLogin={initialGHLogin}
          initialOR={initialOR}
          initialORLabel={initialORLabel}
          initialORModel={initialORModel}
          models={openrouterModels}
          defaultModel={openrouterDefault}
        />
      )}
    </SettingsLayout>
  )
}

function AgentKeyTab({ initialPrefix }: { initialPrefix: string }) {
  const [copiedKey, setCopiedKey] = useState(false)
  const [newAgentKey, setNewAgentKey] = useState<string | null>(null)
  const [currentPrefix, setCurrentPrefix] = useState(initialPrefix)
  const [error, setError] = useState<string | null>(null)

  const copyKey = (key: string) => {
    navigator.clipboard.writeText(key)
    setCopiedKey(true)
    setTimeout(() => setCopiedKey(false), 2000)
  }

  const request = async (path: string, onSuccess: (data: any) => void) => {
    setError(null)
    try {
      const res = await fetch(path, { method: 'POST' })
      const data = await res.json()
      if (data.success) onSuccess(data)
      else setError(data.error)
    } catch {
      setError('Something went wrong. Try again.')
    }
  }

  const create = () =>
    request('/settings/agent-key', (data) => {
      setNewAgentKey(data.key)
      setCurrentPrefix(data.prefix)
      navigator.clipboard.writeText(data.key)
      setCopiedKey(true)
    })

  const regenerate = () =>
    request('/settings/agent-key/regenerate', (data) => {
      setNewAgentKey(data.key)
      setCurrentPrefix(data.prefix)
      navigator.clipboard.writeText(data.key)
      setCopiedKey(true)
    })

  const revoke = () =>
    request('/settings/agent-key/revoke', () => {
      setNewAgentKey(null)
      setCurrentPrefix('')
    })

  return (
    <section>
      <h3 className="text-lg font-bold text-foreground">Read your logs from any tool</h3>
      <p className="text-sm text-foreground/60 mt-1">
        Claude, opencode, Codex and other CLI agents read your logs through one key. You drive, they read.
        Read-only, every app.
      </p>

      {newAgentKey && (
        <div className="mt-5 p-4 bg-accent/10 border border-accent/30">
          <p className="text-sm font-semibold text-foreground mb-2">Your new agent key</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 px-3 py-2 bg-background border border-foreground/10 font-mono text-sm break-all">
              {newAgentKey}
            </code>
            <button
              onClick={() => copyKey(newAgentKey)}
              className="px-3 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80"
            >
              {copiedKey ? 'Copied!' : 'Copy'}
            </button>
          </div>
          <p className="text-sm text-foreground/50 mt-2">Save it now. You won't see it again.</p>
        </div>
      )}

      {error && (
        <div className="mt-5 p-3 bg-red-600/10 text-red-600 text-sm font-medium border border-red-600">
          {error}
        </div>
      )}

      <div className="mt-5">
        {currentPrefix ? (
          <div className="space-y-4">
            <div className="flex items-center gap-2 text-sm">
              <span className="w-2 h-2 rounded-full bg-[#00D678]" />
              <span className="text-foreground">
                Active key <code className="font-mono text-foreground/60">{currentPrefix}...</code>
              </span>
            </div>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={regenerate}
                className="px-4 py-2 border border-foreground/20 text-sm font-semibold text-foreground/70 hover:border-foreground/40 hover:text-foreground transition-colors"
              >
                Regenerate
              </button>
              <button
                type="button"
                onClick={revoke}
                className="px-4 py-2 border border-red-500/30 text-sm font-semibold text-red-600 hover:border-red-500 hover:bg-red-600/10 transition-colors"
              >
                Revoke
              </button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={create}
            className="px-4 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80 transition-colors"
          >
            Create agent key
          </button>
        )}
      </div>

      <div className="mt-6 p-4 bg-foreground/[0.02] border border-foreground/10">
        <p className="text-sm font-semibold text-foreground mb-2">Setup</p>
        <code className="block text-sm font-mono text-foreground/80 whitespace-pre-wrap">
          {`export LOGNORTH_URL="${ORIGIN}"\nexport LOGNORTH_API_KEY="lgn-agent-..."`}
        </code>
        <a
          href="https://lognorth.com/docs/integrations/ai-agents/"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-block text-sm text-foreground/50 underline hover:text-foreground mt-3"
        >
          Read the docs →
        </a>
      </div>
    </section>
  )
}

function ServerTab({
  initialGH,
  initialGHLogin,
  initialOR,
  initialORLabel,
  initialORModel,
  models,
  defaultModel,
}: {
  initialGH: boolean
  initialGHLogin: string
  initialOR: boolean
  initialORLabel: string
  initialORModel: string
  models: string[]
  defaultModel: string
}) {
  return (
    <section className="space-y-10">
      <div>
        <h3 className="text-lg font-bold text-foreground">Let lognorth open PRs</h3>
        <p className="text-sm text-foreground/60 mt-1">
          lognorth reads issues and opens pull requests with fixes. It needs two keys, both stored encrypted.
          Each app then picks its repo on its own Fixes tab.
        </p>
      </div>

      <GitHubCard initialConfigured={initialGH} initialLogin={initialGHLogin} />
      <OpenRouterCard
        initialConfigured={initialOR}
        initialLabel={initialORLabel}
        initialModel={initialORModel}
        models={models}
        defaultModel={defaultModel}
      />
    </section>
  )
}

function GitHubCard({ initialConfigured, initialLogin }: { initialConfigured: boolean; initialLogin: string }) {
  const [configured, setConfigured] = useState(initialConfigured)
  const [login, setLogin] = useState(initialLogin)
  const [token, setToken] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const connect = async () => {
    setError(null)
    setBusy(true)
    try {
      const res = await fetch('/settings/github', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ github_token: token }),
      })
      const data = await res.json()
      if (data.success) {
        setConfigured(true)
        setLogin(data.github_login)
        setToken('')
      } else {
        setError(data.error)
      }
    } catch {
      setError('Failed to reach GitHub. Try again.')
    } finally {
      setBusy(false)
    }
  }

  const disconnect = async () => {
    setError(null)
    setBusy(true)
    try {
      const res = await fetch('/settings/github/disconnect', { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setConfigured(false)
        setLogin('')
      } else {
        setError(data.error)
      }
    } catch {
      setError('Failed to disconnect. Try again.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <p className="text-sm font-bold text-foreground mb-1">GitHub token</p>
      {error && (
        <div className="my-3 p-3 bg-red-600/10 text-red-600 text-sm font-medium border border-red-600">
          {error}
        </div>
      )}
      {configured ? (
        <div className="flex items-center justify-between border border-foreground/10 bg-card px-4 py-3">
          <div className="flex items-center gap-2 text-sm">
            <span className="w-2 h-2 rounded-full bg-[#00D678]" />
            <span className="text-foreground">
              Connected as <span className="font-mono font-semibold">{login}</span>
            </span>
          </div>
          <button
            type="button"
            onClick={disconnect}
            disabled={busy}
            className="text-sm font-semibold text-red-600 hover:underline disabled:opacity-50"
          >
            {busy ? 'Working…' : 'Disconnect'}
          </button>
        </div>
      ) : (
        <div className="space-y-3 max-w-xl">
          <input
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="github_pat_..."
            className="w-full px-3 py-2 bg-background border border-foreground/10 font-mono text-sm focus:border-foreground/40 focus:outline-none"
          />
          <button
            type="button"
            onClick={connect}
            disabled={busy || !token}
            className="px-4 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80 transition-colors disabled:opacity-50"
          >
            {busy ? 'Validating…' : 'Connect token'}
          </button>
        </div>
      )}
      <div className="mt-3 p-4 bg-foreground/[0.02] border border-foreground/10 max-w-xl">
        <p className="text-sm font-semibold text-foreground mb-2">Creating the token</p>
        <ol className="text-sm text-foreground/60 space-y-1 list-decimal list-inside">
          <li>
            GitHub → Settings → Developer settings → <span className="font-mono">Fine-grained tokens</span>.
          </li>
          <li>
            Repository access → <span className="font-semibold text-foreground/80">Only select repositories</span>{' '}
            → just the repos you'll connect. Nothing else.
          </li>
          <li>These permissions, no others:</li>
        </ol>
        <ul className="mt-2 ml-5 text-sm font-mono text-foreground/70 space-y-0.5">
          <li>Metadata — Read</li>
          <li>Contents — Read and write</li>
          <li>Pull requests — Read and write</li>
          <li>Issues — Read</li>
        </ul>
        <p className="text-sm text-foreground/50 mt-3">
          Classic tokens are rejected. Each app can only target a repo this token can reach. Stored encrypted,
          never shown again.
        </p>
      </div>
    </div>
  )
}

function OpenRouterCard({
  initialConfigured,
  initialLabel,
  initialModel,
  models,
  defaultModel,
}: {
  initialConfigured: boolean
  initialLabel: string
  initialModel: string
  models: string[]
  defaultModel: string
}) {
  const [configured, setConfigured] = useState(initialConfigured)
  const [label, setLabel] = useState(initialLabel)
  const [key, setKey] = useState('')
  const [model, setModel] = useState(initialModel || defaultModel)
  const [savedModel, setSavedModel] = useState(initialModel || defaultModel)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const connect = async () => {
    setError(null)
    setBusy(true)
    try {
      const res = await fetch('/settings/openrouter', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ openrouter_key: key, openrouter_model: model }),
      })
      const data = await res.json()
      if (data.success) {
        setConfigured(true)
        setLabel(data.openrouter_label || '')
        setModel(data.openrouter_model || '')
        setSavedModel(data.openrouter_model || '')
        setKey('')
      } else {
        setError(data.error)
      }
    } catch {
      setError('Failed to reach OpenRouter. Try again.')
    } finally {
      setBusy(false)
    }
  }

  const saveModel = async () => {
    setError(null)
    setBusy(true)
    try {
      const res = await fetch('/settings/openrouter/model', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ openrouter_model: model }),
      })
      const data = await res.json()
      if (data.success) {
        setSavedModel(data.openrouter_model || '')
      } else {
        setError(data.error)
      }
    } catch {
      setError('Failed to save. Try again.')
    } finally {
      setBusy(false)
    }
  }

  const disconnect = async () => {
    setError(null)
    setBusy(true)
    try {
      const res = await fetch('/settings/openrouter/disconnect', { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setConfigured(false)
        setLabel('')
      } else {
        setError(data.error)
      }
    } catch {
      setError('Failed to disconnect. Try again.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <p className="text-sm font-bold text-foreground mb-1">OpenRouter key</p>
      <p className="text-sm text-foreground/50 mb-3">Runs the model that writes fixes. Billed to your account.</p>
      {error && (
        <div className="mb-3 p-3 bg-red-600/10 text-red-600 text-sm font-medium border border-red-600">
          {error}
        </div>
      )}
      {configured ? (
        <div className="space-y-4 max-w-xl">
          <div className="flex items-center justify-between border border-foreground/10 bg-card px-4 py-3">
            <div className="flex items-center gap-2 text-sm">
              <span className="w-2 h-2 rounded-full bg-[#00D678]" />
              <span className="text-foreground">
                Connected{label ? <> as <span className="font-mono font-semibold">{label}</span></> : ''}
              </span>
            </div>
            <button
              type="button"
              onClick={disconnect}
              disabled={busy}
              className="text-sm font-semibold text-red-600 hover:underline disabled:opacity-50"
            >
              {busy ? 'Working…' : 'Disconnect'}
            </button>
          </div>
          <div>
            <label className="block text-sm font-semibold text-foreground mb-1">Model</label>
            <div className="flex gap-2">
              <ModelSelect
                value={model}
                onChange={setModel}
                models={models}
                defaultModel={defaultModel}
                className="flex-1"
              />
              <button
                type="button"
                onClick={saveModel}
                disabled={busy || !model || model === savedModel}
                className="px-4 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80 transition-colors disabled:opacity-50"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div className="space-y-3 max-w-xl">
          <input
            type="password"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="sk-or-..."
            className="w-full px-3 py-2 bg-background border border-foreground/10 font-mono text-sm focus:border-foreground/40 focus:outline-none"
          />
          <ModelSelect value={model} onChange={setModel} models={models} defaultModel={defaultModel} className="w-full" />
          <button
            type="button"
            onClick={connect}
            disabled={busy || !key}
            className="px-4 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80 transition-colors disabled:opacity-50"
          >
            {busy ? 'Validating…' : 'Save key'}
          </button>
          <p className="text-sm text-foreground/50">
            Key from{' '}
            <a
              href="https://openrouter.ai/keys"
              target="_blank"
              rel="noopener noreferrer"
              className="underline hover:text-foreground"
            >
              openrouter.ai/keys
            </a>
            . Pick the model the agent runs.
          </p>
        </div>
      )}
    </div>
  )
}

function ModelSelect({
  value,
  onChange,
  models,
  defaultModel,
  className = '',
}: {
  value: string
  onChange: (v: string) => void
  models: string[]
  defaultModel: string
  className?: string
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={`${className} px-3 py-2 bg-background border border-foreground/10 font-mono text-sm text-foreground focus:border-foreground/40 focus:outline-none`}
    >
      {models.map((m) => (
        <option key={m} value={m}>
          {m}
          {m === defaultModel ? ' (default)' : ''}
        </option>
      ))}
    </select>
  )
}

function TabButton({
  active,
  divider,
  onClick,
  children,
}: {
  active: boolean
  divider?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`px-4 py-3 text-sm font-semibold text-center transition-colors ${
        divider ? 'border-l border-foreground/15' : ''
      } ${active ? 'bg-foreground text-background' : 'text-foreground/50 hover:text-foreground'}`}
    >
      {children}
    </button>
  )
}
