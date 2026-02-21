import { useState } from 'react'
import { Layout } from '@/components/layout'
import { Switch } from '@/components/ui/switch'

interface SettingsData {
  notify_channel: string
  ntfy_url: string
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_from: string
  notify_email: string
  telegram_bot_token: string
  telegram_chat_id: string
  quiet_hours_enabled: boolean
  quiet_start: number
  quiet_end: number
  base_url: string
}

interface Props {
  settings?: SettingsData
  hasPassword?: boolean
  userEmail?: string
}

export default function Settings({ settings, hasPassword = false, userEmail = '' }: Props) {
  const [form, setForm] = useState({
    notify_channel: settings?.notify_channel || '',
    ntfy_url: settings?.ntfy_url || '',
    smtp_host: settings?.smtp_host || '',
    smtp_port: settings?.smtp_port || 587,
    smtp_username: settings?.smtp_username || '',
    smtp_password: '',
    smtp_from: settings?.smtp_from || '',
    notify_email: settings?.notify_email || userEmail,
    telegram_bot_token: settings?.telegram_bot_token || '',
    telegram_chat_id: settings?.telegram_chat_id || '',
    quiet_hours_enabled: settings?.quiet_hours_enabled || false,
    quiet_start: settings?.quiet_start ?? 23,
    quiet_end: settings?.quiet_end ?? 7,
    base_url: settings?.base_url || '',
  })
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const handleChange = (field: string, value: string | number | boolean) => {
    setForm((prev) => ({ ...prev, [field]: value }))
    setMessage(null)
  }

  const handleSave = async () => {
    setSaving(true)
    setMessage(null)
    try {
      const res = await fetch('/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      const data = await res.json()
      if (data.success) {
        setMessage({ type: 'success', text: 'Settings saved' })
        setForm((prev) => ({ ...prev, smtp_password: '' }))
      } else {
        setMessage({ type: 'error', text: data.error || 'Failed to save' })
      }
    } catch {
      setMessage({ type: 'error', text: 'Failed to save settings' })
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
  const isEmailConfigured = form.notify_channel === 'email' && form.smtp_host && form.smtp_port && form.smtp_from && form.notify_email
  const isTelegramConfigured = form.notify_channel === 'telegram' && form.telegram_bot_token && form.telegram_chat_id
  const canTest = isNtfyConfigured || isEmailConfigured || isTelegramConfigured

  return (
    <Layout currentPath="/settings">
      <div className="max-w-lg">
        <h1 className="text-2xl font-bold text-black mb-2">Notifications</h1>
        <p className="text-black/50 text-sm mb-8">Know the moment something breaks.</p>

        <div className="space-y-6">
          {/* Channel selector */}
          <div>
            <label className="block text-sm font-semibold text-black mb-3">
              Notification channel
            </label>
            <div className="space-y-2">
              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="radio"
                  name="notify_channel"
                  value="ntfy"
                  checked={form.notify_channel === 'ntfy'}
                  onChange={(e) => handleChange('notify_channel', e.target.value)}
                  className="w-4 h-4 border-2 border-black/20"
                />
                <div>
                  <span className="block font-semibold text-black">ntfy.sh</span>
                  <span className="block text-sm text-black/50">Free push notifications to your phone or desktop</span>
                </div>
              </label>

              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="radio"
                  name="notify_channel"
                  value="email"
                  checked={form.notify_channel === 'email'}
                  onChange={(e) => handleChange('notify_channel', e.target.value)}
                  className="w-4 h-4 border-2 border-black/20"
                />
                <div>
                  <span className="block font-semibold text-black">Email (SMTP)</span>
                  <span className="block text-sm text-black/50">Use your own mail server</span>
                </div>
              </label>

              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="radio"
                  name="notify_channel"
                  value="telegram"
                  checked={form.notify_channel === 'telegram'}
                  onChange={(e) => handleChange('notify_channel', e.target.value)}
                  className="w-4 h-4 border-2 border-black/20"
                />
                <div>
                  <span className="block font-semibold text-black">Telegram</span>
                  <span className="block text-sm text-black/50">Send alerts via Telegram bot</span>
                </div>
              </label>
            </div>
          </div>

          {/* ntfy configuration */}
          {form.notify_channel === 'ntfy' && (
            <div className="space-y-4 pt-2">
              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  ntfy topic URL
                </label>
                <input
                  type="url"
                  value={form.ntfy_url}
                  onChange={(e) => handleChange('ntfy_url', e.target.value)}
                  placeholder="https://ntfy.sh/your-secret-topic"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                <p className="text-sm text-black/50 mt-2">
                  Get the free <a href="https://ntfy.sh" target="_blank" rel="noopener noreferrer" className="underline hover:text-black">ntfy app</a>, pick a secret topic name, paste the URL here. Done.
                </p>
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Base URL
                </label>
                <input
                  type="url"
                  value={form.base_url}
                  onChange={(e) => handleChange('base_url', e.target.value)}
                  placeholder="https://lognorth.example.com"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                <span className="text-sm text-black/50">For links in notifications</span>
              </div>
            </div>
          )}

          {/* Email/SMTP configuration */}
          {form.notify_channel === 'email' && (
            <div className="space-y-5 pt-2">
              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Send to
                </label>
                <input
                  type="email"
                  value={form.notify_email}
                  onChange={(e) => handleChange('notify_email', e.target.value)}
                  placeholder="you@example.com"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Base URL
                </label>
                <input
                  type="url"
                  value={form.base_url}
                  onChange={(e) => handleChange('base_url', e.target.value)}
                  placeholder="https://lognorth.example.com"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                <span className="text-sm text-black/50">For links in email notifications</span>
              </div>

              <hr className="border-black/10" />

              <p className="text-sm font-semibold text-black">SMTP Server</p>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Host
                </label>
                <input
                  type="text"
                  value={form.smtp_host}
                  onChange={(e) => handleChange('smtp_host', e.target.value)}
                  placeholder="smtp.example.com"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Port
                </label>
                <input
                  type="number"
                  value={form.smtp_port}
                  onChange={(e) => handleChange('smtp_port', parseInt(e.target.value) || 587)}
                  placeholder="587"
                  className="w-24 px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                <span className="text-sm text-black/80 ml-2">587 (TLS) or 465 (SSL)</span>
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Username
                </label>
                <input
                  type="text"
                  value={form.smtp_username}
                  onChange={(e) => handleChange('smtp_username', e.target.value)}
                  placeholder="your-username"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Password
                </label>
                <input
                  type="password"
                  value={form.smtp_password}
                  onChange={(e) => handleChange('smtp_password', e.target.value)}
                  placeholder={hasPassword ? '••••••••' : 'Enter password'}
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                {hasPassword && (
                  <span className="text-sm text-black/50">Leave blank to keep current</span>
                )}
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  From address
                </label>
                <input
                  type="email"
                  value={form.smtp_from}
                  onChange={(e) => handleChange('smtp_from', e.target.value)}
                  placeholder="alerts@example.com"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
              </div>
            </div>
          )}

          {/* Telegram configuration */}
          {form.notify_channel === 'telegram' && (
            <div className="space-y-4 pt-2">
              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Bot Token
                </label>
                <input
                  type="text"
                  value={form.telegram_bot_token}
                  onChange={(e) => handleChange('telegram_bot_token', e.target.value)}
                  placeholder="123456789:ABCdefGHI..."
                  className="w-full px-3 py-2 border border-black/20 text-sm font-mono focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                <p className="text-sm text-black/50 mt-2">
                  Create a bot with <a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer" className="underline hover:text-black">@BotFather</a>, copy the token.
                </p>
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Chat ID
                </label>
                <input
                  type="text"
                  value={form.telegram_chat_id}
                  onChange={(e) => handleChange('telegram_chat_id', e.target.value)}
                  placeholder="-1001234567890"
                  className="w-full px-3 py-2 border border-black/20 text-sm font-mono focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                <p className="text-sm text-black/50 mt-2">
                  Your user ID or group chat ID. Use <a href="https://t.me/userinfobot" target="_blank" rel="noopener noreferrer" className="underline hover:text-black">@userinfobot</a> to find yours.
                </p>
              </div>

              <div>
                <label className="block text-sm font-semibold text-black mb-1.5">
                  Base URL
                </label>
                <input
                  type="url"
                  value={form.base_url}
                  onChange={(e) => handleChange('base_url', e.target.value)}
                  placeholder="https://lognorth.example.com"
                  className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent placeholder:text-black/40"
                />
                <span className="text-sm text-black/50">For links in notifications</span>
              </div>
            </div>
          )}

        </div>

        {/* Quiet Hours - separate section */}
        {form.notify_channel && (
          <div className="mt-12">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-lg font-bold text-black">Quiet hours</h2>
                <p className="text-black/50 text-sm">Sleep in peace. No alerts during these hours.</p>
              </div>
              <Switch
                checked={form.quiet_hours_enabled}
                onCheckedChange={(checked) => handleChange('quiet_hours_enabled', checked)}
              />
            </div>

            {form.quiet_hours_enabled && (
              <div className="flex items-center justify-center gap-8 py-8 bg-black/[0.02] border border-black/10">
                <HourPicker
                  value={form.quiet_start}
                  onChange={(v) => handleChange('quiet_start', v)}
                />
                <span className="text-3xl text-black/30 mt-6">→</span>
                <HourPicker
                  value={form.quiet_end}
                  onChange={(v) => handleChange('quiet_end', v)}
                />
              </div>
            )}
          </div>
        )}

        {message && (
          <div
            className={`mt-6 p-3 text-sm font-medium ${
              message.type === 'success'
                ? 'bg-accent/10 text-black border border-accent/20'
                : 'bg-rose-50 text-[#e11d48] border border-[#e11d48]'
            }`}
          >
            {message.text}
          </div>
        )}

        <div className="flex gap-3 mt-6">
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-2 bg-black text-white text-sm font-semibold hover:bg-black/80 disabled:opacity-40 transition-colors"
          >
            {saving ? 'Saving...' : 'Save'}
          </button>
          {form.notify_channel && (
            <button
              onClick={handleTest}
              disabled={testing || !canTest}
              className="px-4 py-2 border border-black/20 text-sm font-semibold text-black/70 hover:border-black/40 hover:text-black disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {testing ? 'Testing...' : 'Test notification'}
            </button>
          )}
        </div>
      </div>
    </Layout>
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
        className="p-2 text-black/40 hover:text-black transition-colors"
      >
        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 15l7-7 7 7" />
        </svg>
      </button>
      <div className="flex items-baseline">
        <span className="text-5xl font-mono font-black tabular-nums text-black">
          {value.toString().padStart(2, '0')}
        </span>
        <span className="text-2xl font-mono font-bold text-black/30 ml-1">:00</span>
      </div>
      <button
        type="button"
        onClick={decrement}
        className="p-2 text-black/40 hover:text-black transition-colors"
      >
        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
    </div>
  )
}
