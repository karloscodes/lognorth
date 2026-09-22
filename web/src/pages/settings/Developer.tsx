import { useState } from 'react'
import { SettingsLayout } from '@/components/settings-layout'

interface Props {
  agentKeyPrefix?: string
}

export default function SettingsDeveloper({ agentKeyPrefix: initialPrefix = '' }: Props) {
  const [copiedKey, setCopiedKey] = useState(false)
  const [copiedStep, setCopiedStep] = useState<number | null>(null)
  const [newAgentKey, setNewAgentKey] = useState<string | null>(null)
  const [currentPrefix, setCurrentPrefix] = useState(initialPrefix)
  const [agentKeyError, setAgentKeyError] = useState<string | null>(null)

  const copyKey = (key: string) => {
    navigator.clipboard.writeText(key)
    setCopiedKey(true)
    setTimeout(() => setCopiedKey(false), 2000)
  }

  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://your-instance.com'
  const connectCommand = `/lognorth:connect ${origin} ${newAgentKey ?? 'lgn-agent-...'}`

  const copyStep = (index: number, command: string) => {
    navigator.clipboard.writeText(command)
    setCopiedStep(index)
    setTimeout(() => setCopiedStep(null), 2000)
  }

  const setupSteps = [
    '/plugin marketplace add karloscodes/lognorth-releases',
    '/plugin install lognorth',
    connectCommand,
  ]

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
      <h2 className="text-[22px] font-bold tracking-tight text-foreground mb-2">AI agents</h2>
      <p className="text-subtle text-sm mb-6">
        Let Claude Code, Cursor, or any MCP client read your logs. Read-only, every app.
      </p>

      {newAgentKey && (
        <div className="mb-6 p-4 bg-accent/10 border border-accent/30">
          <p className="text-sm font-bold text-foreground mb-2">Your new agent key:</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 px-3 py-2 bg-card border border-line font-mono text-sm break-all">
              {newAgentKey}
            </code>
            <button
              onClick={() => copyKey(newAgentKey)}
              className="px-3 py-2 bg-foreground text-background text-sm font-bold hover:bg-text"
            >
              {copiedKey ? 'Copied!' : 'Copy'}
            </button>
          </div>
          <p className="text-sm text-subtle mt-2">
            Save this key now. You won't be able to see it again.
          </p>
        </div>
      )}

      {agentKeyError && (
        <div className="mb-6 p-3 bg-error/10 text-error text-sm font-medium border border-error">
          {agentKeyError}
        </div>
      )}

      {currentPrefix ? (
        <div className="space-y-4">
          <div className="flex items-center gap-4">
            <div>
              <p className="text-sm font-bold text-foreground">Active key</p>
              <code className="text-sm font-mono text-subtle">{currentPrefix}...</code>
            </div>
          </div>
          <div className="flex gap-3">
            <button
              type="button"
              onClick={regenerateAgentKey}
              className="px-4 py-2 border border-border text-sm font-bold text-text hover:border-foreground hover:text-foreground transition-colors"
            >
              Regenerate
            </button>
            <button
              type="button"
              onClick={revokeAgentKey}
              className="px-4 py-2 border border-red-500/30 text-sm font-bold text-error hover:border-red-500 hover:bg-error/10 transition-colors"
            >
              Revoke
            </button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          onClick={createAgentKey}
          className="px-4 py-2 bg-foreground text-background text-sm font-bold hover:bg-text transition-colors"
        >
          Create agent key
        </button>
      )}

      <div className="mt-6 p-4 bg-raise/40 border border-line">
        <p className="text-sm font-bold text-foreground mb-2">Setup</p>
        <p className="text-sm text-subtle mb-3">
          {currentPrefix
            ? 'Three commands in Claude Code, one at a time. The last one carries your key, which is only shown once, so regenerate it if you no longer have it.'
            : 'Create an agent key above. Then run these three commands in Claude Code, one at a time.'}
        </p>
        <div className="space-y-2">
          {setupSteps.map((command, i) => (
            <div key={i} className="flex items-start gap-2">
              <span className="font-mono text-sm text-dim pt-1 tabular-nums">{i + 1}</span>
              <code className="flex-1 px-3 py-1 bg-card border border-line text-sm font-mono text-text break-all">
                {command}
              </code>
              <button
                type="button"
                onClick={() => copyStep(i, command)}
                className="shrink-0 px-3 py-1 border border-border text-xs font-bold text-text hover:border-foreground hover:text-foreground transition-colors"
              >
                {copiedStep === i ? 'Copied!' : 'Copy'}
              </button>
            </div>
          ))}
        </div>
        <p className="text-sm text-subtle mt-3">
          Run them one at a time. If Claude Code asks you to "Enter marketplace source", type only{' '}
          <code className="font-mono">karloscodes/lognorth-releases</code> there.
        </p>
        <p className="text-sm text-subtle mt-2">
          Restart Claude Code and it has four read-only tools over your log. Other clients take the same
          endpoint:{' '}
          <code className="font-mono">{origin}/mcp</code>.{' '}
          <a
            href="https://lognorth.com/docs/integrations/mcp/"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground"
          >
            Docs →
          </a>
        </p>
      </div>
    </SettingsLayout>
  )
}
