import { useState } from 'react'
import { SettingsLayout } from '@/components/settings-layout'

interface SettingsData {
  retention_days: number
  notify_channel: string
  ntfy_url: string
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_from: string
  notify_email: string
  telegram_chat_id: string
  quiet_hours_enabled: boolean
  quiet_start: number
  quiet_end: number
  base_url: string
}

interface Props {
  settings?: SettingsData
}

const PRESETS = [
  { value: 7, label: '7d' },
  { value: 30, label: '30d' },
  { value: 90, label: '90d', default: true },
  { value: 180, label: '6mo' },
  { value: 365, label: '1yr' },
  { value: 0, label: 'Forever' },
]

export default function SettingsData({ settings }: Props) {
  const [retentionDays, setRetentionDays] = useState(settings?.retention_days || 90)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const handleSave = async () => {
    if (!settings) return
    setSaving(true)
    setMessage(null)
    try {
      const res = await fetch('/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...settings, retention_days: retentionDays }),
      })
      const data = await res.json()
      if (data.success) {
        setMessage({ type: 'success', text: 'Retention saved' })
      } else {
        setMessage({ type: 'error', text: data.error || 'Failed to save' })
      }
    } catch {
      setMessage({ type: 'error', text: 'Failed to save' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <SettingsLayout currentPath="/settings/data">
      <h2 className="text-[22px] font-bold tracking-tight text-foreground mb-2">Data retention</h2>
      <p className="text-subtle text-sm mb-6">
        How long to keep your logs. Older events are automatically deleted.
      </p>

      <div className="flex flex-wrap gap-1 border border-border px-2 py-1.5 text-[13px] w-fit">
        {PRESETS.map((opt) => {
          const on = retentionDays === opt.value
          return (
            <button
              key={opt.value}
              type="button"
              aria-pressed={on}
              onClick={() => {
                setRetentionDays(opt.value)
                setMessage(null)
              }}
              className={`px-1.5 py-0.5 ${on ? 'font-bold text-foreground' : 'text-subtle hover:text-foreground'}`}
            >
              {on && <span className="text-accent">[</span>}
              {opt.label.toLowerCase()}
              {on && <span className="text-accent">]</span>}
              {opt.default && !on && <span className="ml-1 text-dim">*</span>}
            </button>
          )
        })}
      </div>
      <p className="text-sm text-subtle mt-3">
        Shorter retention = smaller database. 90 days (*) is plenty for most.
      </p>

      <div className="mt-6 flex items-center gap-3 flex-wrap">
        <button
          onClick={handleSave}
          disabled={saving}
          className="bg-foreground text-background font-bold px-4 py-2 hover:bg-text disabled:opacity-50 text-sm"
        >
          {saving ? 'Saving...' : 'Save retention'}
        </button>
        {message && (
          <span className={`text-sm font-medium ${message.type === 'success' ? 'text-subtle' : 'text-error'}`}>
            {message.text}
          </span>
        )}
      </div>
    </SettingsLayout>
  )
}
