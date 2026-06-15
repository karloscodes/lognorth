import { useState } from 'react'
import { SettingsLayout } from '@/components/settings-layout'

interface Props {
  agentKeyPrefix?: string
}

export default function SettingsDeveloper({ agentKeyPrefix: initialPrefix = '' }: Props) {
  const [copiedKey, setCopiedKey] = useState(false)
  const [newAgentKey, setNewAgentKey] = useState<string | null>(null)
  const [currentPrefix, setCurrentPrefix] = useState(initialPrefix)
  const [agentKeyError, setAgentKeyError] = useState<string | null>(null)

  const copyKey = (key: string) => {
    navigator.clipboard.writeText(key)
    setCopiedKey(true)
    setTimeout(() => setCopiedKey(false), 2000)
  }

  const createAgentKey = async () => {
    setAgentKeyError(null)
    try {
      const res = await fetch('/settings/agent-key', { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setNewAgentKey(data.key)
        setCurrentPrefix(data.prefix)
        navigator.clipboard.writeText(data.key)
        setCopiedKey(true)
      } else {
        setAgentKeyError(data.error)
      }
    } catch {
      setAgentKeyError('Failed to create agent key')
    }
  }

  const regenerateAgentKey = async () => {
    setAgentKeyError(null)
    try {
      const res = await fetch('/settings/agent-key/regenerate', { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setNewAgentKey(data.key)
        setCurrentPrefix(data.prefix)
        navigator.clipboard.writeText(data.key)
        setCopiedKey(true)
      } else {
        setAgentKeyError(data.error)
      }
    } catch {
      setAgentKeyError('Failed to regenerate agent key')
    }
  }

  const revokeAgentKey = async () => {
    setAgentKeyError(null)
    try {
      const res = await fetch('/settings/agent-key/revoke', { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setNewAgentKey(null)
        setCurrentPrefix('')
      } else {
        setAgentKeyError(data.error)
      }
    } catch {
      setAgentKeyError('Failed to revoke agent key')
    }
  }

  return (
    <SettingsLayout currentPath="/settings/developer">
      <h2 className="text-2xl font-bold text-foreground mb-2">Agent API</h2>
      <p className="text-foreground/50 text-sm mb-6">
        Let AI coding assistants query your logs. Read-only access to all apps.
      </p>

      {newAgentKey && (
        <div className="mb-6 p-4 bg-accent/10 border border-accent/30">
          <p className="text-sm font-semibold text-foreground mb-2">Your new agent key:</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 px-3 py-2 bg-card border border-foreground/10 font-mono text-sm break-all">
              {newAgentKey}
            </code>
            <button
              onClick={() => copyKey(newAgentKey)}
              className="px-3 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80"
            >
              {copiedKey ? 'Copied!' : 'Copy'}
            </button>
          </div>
          <p className="text-sm text-foreground/50 mt-2">
            Save this key now. You won't be able to see it again.
          </p>
        </div>
      )}

      {agentKeyError && (
        <div className="mb-6 p-3 bg-[#e11d48]/10 text-[#e11d48] text-sm font-medium border border-[#e11d48]">
          {agentKeyError}
        </div>
      )}

      {currentPrefix ? (
        <div className="space-y-4">
          <div className="flex items-center gap-4">
            <div>
              <p className="text-sm font-semibold text-foreground">Active key</p>
              <code className="text-sm font-mono text-foreground/60">{currentPrefix}...</code>
            </div>
          </div>
          <div className="flex gap-3">
            <button
              type="button"
              onClick={regenerateAgentKey}
              className="px-4 py-2 border border-foreground/20 text-sm font-semibold text-foreground/70 hover:border-foreground/40 hover:text-foreground transition-colors"
            >
              Regenerate
            </button>
            <button
              type="button"
              onClick={revokeAgentKey}
              className="px-4 py-2 border border-red-500/30 text-sm font-semibold text-red-600 hover:border-red-500 hover:bg-[#e11d48]/10 transition-colors"
            >
              Revoke
            </button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          onClick={createAgentKey}
          className="px-4 py-2 bg-foreground text-background text-sm font-semibold hover:bg-foreground/80 transition-colors"
        >
          Create agent key
        </button>
      )}

      <div className="mt-6 p-4 bg-foreground/[0.02] border border-foreground/10">
        <p className="text-sm font-semibold text-foreground mb-2">Setup</p>
        <p className="text-sm text-foreground/60 mb-3">Add to your shell profile:</p>
        <code className="block text-sm font-mono text-foreground/80 whitespace-pre-wrap">
          {`export LOGNORTH_URL="${typeof window !== 'undefined' ? window.location.origin : 'https://your-instance.com'}"
export LOGNORTH_API_KEY="lgn-agent-..."`}
        </code>
        <p className="text-sm text-foreground/50 mt-3">
          <a
            href="https://lognorth.com/docs/integrations/ai-agents/"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground"
          >
            Documentation →
          </a>
        </p>
      </div>
    </SettingsLayout>
  )
}
