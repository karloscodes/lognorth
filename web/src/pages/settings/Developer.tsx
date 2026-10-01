import { useState } from 'react'
import { Link, usePage } from '@inertiajs/react'
import { SettingsLayout } from '@/components/settings-layout'

interface Props {
  agentKeyPrefix?: string
}

export default function SettingsDeveloper({ agentKeyPrefix: initialPrefix = '' }: Props) {
  const licenseValid = usePage<{ licenseValid?: boolean }>().props.licenseValid ?? true
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
  const copyStep = (index: number, command: string) => {
    navigator.clipboard.writeText(command)
    setCopiedStep(index)
    setTimeout(() => setCopiedStep(null), 2000)
  }

  // One command installs north, connects it, and adds LogNorth to the coding
  // agents it finds. Without the key at hand (it shows once), it asks for it.
  const terminalCommand = `curl -fsSL https://lognorth.com/cli | sh -s -- ${origin}${newAgentKey ? ` ${newAgentKey}` : ''}`

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

      {!licenseValid && (
        <p className="mb-6 border border-border p-3 text-sm text-text">
          Agent access needs a license. Everything else is free.{' '}
          <Link href="/settings/license" className="font-bold text-foreground underline hover:no-underline">
            Add your license key →
          </Link>
        </p>
      )}

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
        <p className="text-sm font-bold text-foreground mb-2">Connect your coding agent</p>
        <p className="text-sm text-subtle mb-3">
          {currentPrefix
            ? 'One command in your terminal. It installs north, checks the key, and adds LogNorth to Claude Code, Codex, and Gemini CLI.'
            : 'Create an agent key above, then run one command in your terminal.'}
          {currentPrefix && !newAgentKey && ' It asks for the agent key.'}
        </p>
        <div className="flex items-start gap-2">
          <code className="flex-1 break-all border border-line bg-card px-3 py-1 font-mono text-sm text-text">{terminalCommand}</code>
          <button
            type="button"
            onClick={() => copyStep(0, terminalCommand)}
            className="shrink-0 border border-border px-3 py-1 text-xs font-bold text-text transition-colors hover:border-foreground hover:text-foreground"
          >
            {copiedStep === 0 ? 'Copied!' : 'Copy'}
          </button>
        </div>
        <p className="mt-3 text-sm text-subtle">
          Restart your agent and ask it what is broken in production. The key stays in one file on your machine, never in
          an agent config. macOS or Linux, no sudo. <code className="font-mono">north tail</code> and{' '}
          <code className="font-mono">north top</code> read the log from the terminal too.
        </p>
        <p className="mt-2 text-sm text-subtle">
          Other MCP clients connect to <code className="font-mono">{origin}/mcp</code> with the agent key as a Bearer
          token.{' '}
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
