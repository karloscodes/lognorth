import { useState } from 'react'
import { SettingsLayout } from '@/components/settings-layout'
import { Switch } from '@/components/ui/switch'

interface SettingsData {
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
  retention_days: number
  license_key: string
  license_status: string
  license_email: string
}

interface Props {
  settings?: SettingsData
  hasPassword?: boolean
  hasTelegramToken?: boolean
  userEmail?: string
}

export default function SettingsAlerts({ settings, hasPassword = false, hasTelegramToken = false, userEmail = '' }: Props) {
  // The server never sends the bot token back, only whether one is saved.
  const [telegramTokenSaved, setTelegramTokenSaved] = useState(hasTelegramToken)
  const [form, setForm] = useState({
    notify_channel: settings?.notify_channel || '',
    ntfy_url: settings?.ntfy_url || '',
    smtp_host: settings?.smtp_host || '',
    smtp_port: settings?.smtp_port || 587,
    smtp_username: settings?.smtp_username || '',
    smtp_password: '',
    smtp_from: settings?.smtp_from || '',
    notify_email: settings?.notify_email || userEmail,
    telegram_bot_token: '',
    telegram_chat_id: settings?.telegram_chat_id || '',
    quiet_hours_enabled: settings?.quiet_hours_enabled || false,
    quiet_start: settings?.quiet_start ?? 23,
    quiet_end: settings?.quiet_end ?? 7,
    base_url: settings?.base_url || '',
  })
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const change = (field: string, value: string | number | boolean) => {
    setForm((prev) => ({ ...prev, [field]: value }))
    setMessage(null)
  }

  const handleSave = async () => {
    if (!settings) return
    setSaving(true)
    setMessage(null)
    try {
      const res = await fetch('/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ...settings,
          ...form,
        }),
      })
      const data = await res.json()
      if (data.success) {
        if (form.telegram_bot_token) setTelegramTokenSaved(true)
        setForm((prev) => ({ ...prev, smtp_password: '', telegram_bot_token: '' }))
        setMessage({ type: 'success', text: 'Alerts saved' })
      } else {
        setMessage({ type: 'error', text: data.error || 'Failed to save' })
      }
    } catch {
      setMessage({ type: 'error', text: 'Failed to save' })
    } finally {
      setSaving(false)
    }
  }

  const handleTest = async () => {
    setTesting(true)
    setMessage(null)
    try {
      const res = await fetch('/settings/test', { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setMessage({ type: 'success', text: data.message })
      } else {
        setMessage({ type: 'error', text: data.error || 'Test failed' })
      }
    } catch {
      setMessage({ type: 'error', text: 'Failed to send test notification' })
    } finally {
      setTesting(false)
    }
  }

  const isNtfyConfigured = form.notify_channel === 'ntfy' && form.ntfy_url
  const isEmailConfigured =
    form.notify_channel === 'email' && form.smtp_host && form.smtp_port && form.smtp_from && form.notify_email
  const isTelegramConfigured =
    form.notify_channel === 'telegram' && (form.telegram_bot_token || telegramTokenSaved) && form.telegram_chat_id
  const canTest = isNtfyConfigured || isEmailConfigured || isTelegramConfigured

  return (
    <SettingsLayout currentPath="/settings/alerts">
      <h2 className="text-2xl font-bold text-foreground mb-2">Alerts</h2>
      <p className="text-foreground/50 text-sm mb-8">How you get told when something breaks.</p>

      <div className="space-y-6">
        <div>
          <label className="block text-sm font-semibold text-foreground mb-3">Channel</label>
          <div className="space-y-2">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="radio"
                name="notify_channel"
                value="ntfy"
                checked={form.notify_channel === 'ntfy'}
                onChange={(e) => change('notify_channel', e.target.value)}
                className="w-4 h-4 border-2 border-foreground/20"
              />
              <div>
                <span className="block font-semibold text-foreground">ntfy.sh</span>
                <span className="block text-sm text-foreground/50">Free push notifications to your phone or desktop</span>
              </div>
            </label>
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="radio"
                name="notify_channel"
                value="email"
                checked={form.notify_channel === 'email'}
                onChange={(e) => change('notify_channel', e.target.value)}
                className="w-4 h-4 border-2 border-foreground/20"
              />
              <div>
                <span className="block font-semibold text-foreground">Email (SMTP)</span>
                <span className="block text-sm text-foreground/50">Use your own mail server</span>
              </div>
            </label>
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="radio"
                name="notify_channel"
                value="telegram"
                checked={form.notify_channel === 'telegram'}
                onChange={(e) => change('notify_channel', e.target.value)}
                className="w-4 h-4 border-2 border-foreground/20"
              />
              <div>
                <span className="block font-semibold text-foreground">Telegram</span>
                <span className="block text-sm text-foreground/50">Send alerts via Telegram bot</span>
              </div>
            </label>
          </div>
        </div>

        {form.notify_channel === 'ntfy' && (
          <div className="space-y-4 pt-2">
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">ntfy topic URL</label>
              <input
                type="url"
                value={form.ntfy_url}
                onChange={(e) => change('ntfy_url', e.target.value)}
                placeholder="https://ntfy.sh/your-secret-topic"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              <p className="text-sm text-foreground/50 mt-2">
                Get the free{' '}
                <a href="https://ntfy.sh" target="_blank" rel="noopener noreferrer" className="underline hover:text-foreground">
                  ntfy app
                </a>
                , pick a secret topic name, paste the URL here. Done.
              </p>
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Base URL</label>
              <input
                type="url"
                value={form.base_url}
                onChange={(e) => change('base_url', e.target.value)}
                placeholder="https://lognorth.example.com"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              <span className="text-sm text-foreground/50">For links in notifications</span>
            </div>
          </div>
        )}

        {form.notify_channel === 'email' && (
          <div className="space-y-5 pt-2">
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Send to</label>
              <input
                type="email"
                value={form.notify_email}
                onChange={(e) => change('notify_email', e.target.value)}
                placeholder="you@example.com"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Base URL</label>
              <input
                type="url"
                value={form.base_url}
                onChange={(e) => change('base_url', e.target.value)}
                placeholder="https://lognorth.example.com"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              <span className="text-sm text-foreground/50">For links in email notifications</span>
            </div>
            <hr className="border-foreground/10" />
            <p className="text-sm font-semibold text-foreground">SMTP Server</p>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Host</label>
              <input
                type="text"
                value={form.smtp_host}
                onChange={(e) => change('smtp_host', e.target.value)}
                placeholder="smtp.example.com"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Port</label>
              <input
                type="number"
                value={form.smtp_port}
                onChange={(e) => change('smtp_port', parseInt(e.target.value) || 587)}
                placeholder="587"
                className="w-24 px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              <span className="text-sm text-foreground/80 ml-2">587 (TLS) or 465 (SSL)</span>
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Username</label>
              <input
                type="text"
                value={form.smtp_username}
                onChange={(e) => change('smtp_username', e.target.value)}
                placeholder="your-username"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Password</label>
              <input
                type="password"
                value={form.smtp_password}
                onChange={(e) => change('smtp_password', e.target.value)}
                placeholder={hasPassword ? '••••••••' : 'Enter password'}
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              {hasPassword && <span className="text-sm text-foreground/50">Leave blank to keep current</span>}
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">From address</label>
              <input
                type="email"
                value={form.smtp_from}
                onChange={(e) => change('smtp_from', e.target.value)}
                placeholder="alerts@example.com"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
            </div>
          </div>
        )}

        {form.notify_channel === 'telegram' && (
          <div className="space-y-4 pt-2">
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Bot Token</label>
              <input
                type="text"
                value={form.telegram_bot_token}
                onChange={(e) => change('telegram_bot_token', e.target.value)}
                placeholder={telegramTokenSaved ? '••••••••  (saved, leave blank to keep)' : '123456789:ABCdefGHI...'}
                className="w-full px-3 py-2 border border-foreground/20 text-sm font-mono focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              <p className="text-sm text-foreground/50 mt-2">
                Create a bot with{' '}
                <a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer" className="underline hover:text-foreground">
                  @BotFather
                </a>
                , copy the token.
              </p>
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Chat ID</label>
              <input
                type="text"
                value={form.telegram_chat_id}
                onChange={(e) => change('telegram_chat_id', e.target.value)}
                placeholder="-1001234567890"
                className="w-full px-3 py-2 border border-foreground/20 text-sm font-mono focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              <p className="text-sm text-foreground/50 mt-2">
                Your user ID or group chat ID. Use{' '}
                <a href="https://t.me/userinfobot" target="_blank" rel="noopener noreferrer" className="underline hover:text-foreground">
                  @userinfobot
                </a>{' '}
                to find yours.
              </p>
            </div>
            <div>
              <label className="block text-sm font-semibold text-foreground mb-1.5">Base URL</label>
              <input
                type="url"
                value={form.base_url}
                onChange={(e) => change('base_url', e.target.value)}
                placeholder="https://lognorth.example.com"
                className="w-full px-3 py-2 border border-foreground/20 text-sm focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
              />
              <span className="text-sm text-foreground/50">For links in notifications</span>
            </div>
          </div>
        )}
      </div>

      {form.notify_channel && (
        <div className="mt-10 pt-6 border-t border-foreground/10">
          <div className="flex items-center justify-between mb-6">
            <div>
              <h3 className="text-lg font-bold text-foreground">Quiet hours</h3>
              <p className="text-foreground/50 text-sm">No alerts during these hours. Times are UTC, the server's clock.</p>
            </div>
            <Switch
              checked={form.quiet_hours_enabled}
              onCheckedChange={(checked) => change('quiet_hours_enabled', checked)}
            />
          </div>
          {form.quiet_hours_enabled && (
            <div className="flex items-center justify-center gap-8 py-8 bg-foreground/[0.02] border border-foreground/10">
              <HourPicker value={form.quiet_start} onChange={(v) => change('quiet_start', v)} />
              <span className="text-3xl text-foreground/30 mt-6">→</span>
              <HourPicker value={form.quiet_end} onChange={(v) => change('quiet_end', v)} />
            </div>
          )}
        </div>
      )}

      <div className="mt-8 flex items-center gap-3 flex-wrap">
        <button
          onClick={handleSave}
          disabled={saving}
          className="bg-foreground text-background font-semibold px-4 py-2 hover:bg-foreground/80 disabled:opacity-50 text-sm"
        >
          {saving ? 'Saving...' : 'Save alerts'}
        </button>
        {form.notify_channel && (
          <button
            onClick={handleTest}
            disabled={testing || !canTest}
            className="px-4 py-2 border border-foreground/20 text-sm font-semibold text-foreground/70 hover:border-foreground/40 hover:text-foreground disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {testing ? 'Testing...' : 'Test notification'}
          </button>
        )}
        {message && (
          <span className={`text-sm font-medium ${message.type === 'success' ? 'text-foreground/60' : 'text-red-600'}`}>
            {message.text}
          </span>
        )}
      </div>
    </SettingsLayout>
  )
}

function HourPicker({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const increment = () => onChange((value + 1) % 24)
  const decrement = () => onChange((value - 1 + 24) % 24)
  return (
    <div className="flex flex-col items-center">
      <button
        type="button"
        onClick={increment}
        className="p-2 text-foreground/40 hover:text-foreground transition-colors"
      >
        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 15l7-7 7 7" />
        </svg>
      </button>
      <div className="flex items-baseline">
        <span className="text-5xl font-mono font-black tabular-nums text-foreground">
          {value.toString().padStart(2, '0')}
        </span>
        <span className="text-2xl font-mono font-bold text-foreground/30 ml-1">:00</span>
      </div>
      <button
        type="button"
        onClick={decrement}
        className="p-2 text-foreground/40 hover:text-foreground transition-colors"
      >
        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
    </div>
  )
}
