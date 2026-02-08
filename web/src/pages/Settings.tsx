import { useState } from 'react'
import { Layout } from '@/components/layout'

interface SettingsData {
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_from: string
  notify_new_errors: boolean
  notify_regressions: boolean
  notify_spikes: boolean
  spike_threshold: number
  notify_email: string
  base_url: string
}

interface Props {
  settings?: SettingsData
  hasPassword?: boolean
  section?: string
  userEmail?: string
}

export default function Settings({ settings, hasPassword = false, section = 'notifications', userEmail = '' }: Props) {
  const [form, setForm] = useState({
    smtp_host: settings?.smtp_host || '',
    smtp_port: settings?.smtp_port || 587,
    smtp_username: settings?.smtp_username || '',
    smtp_password: '',
    smtp_from: settings?.smtp_from || '',
    notify_new_errors: settings?.notify_new_errors || false,
    notify_regressions: settings?.notify_regressions || false,
    notify_spikes: settings?.notify_spikes || false,
    spike_threshold: settings?.spike_threshold || 10,
    notify_email: settings?.notify_email || userEmail,
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
      const res = await fetch('/settings/test-smtp', { method: 'POST' })
      const data = await res.json()
      if (data.success) {
        setMessage({ type: 'success', text: data.message })
      } else {
        setMessage({ type: 'error', text: data.error || 'Test failed' })
      }
    } catch {
      setMessage({ type: 'error', text: 'Failed to test SMTP' })
    } finally {
      setTesting(false)
    }
  }

  const isEmailConfigured = form.smtp_host && form.smtp_port && form.smtp_from && form.notify_email

  const sections = [
    { id: 'notifications', label: 'Notifications' },
    { id: 'email', label: 'Email' },
  ]

  return (
    <Layout currentPath="/settings">
      <div className="flex gap-12">
        {/* Sidebar */}
        <nav className="w-36 flex-shrink-0">
          <ul className="space-y-1">
            {sections.map((s) => (
              <li key={s.id}>
                <a
                  href={`/settings?section=${s.id}`}
                  className={`block px-3 py-2.5 text-sm font-semibold transition-colors ${
                    section === s.id
                      ? 'bg-black text-white'
                      : 'text-black/80 hover:text-black hover:bg-black/[0.04]'
                  }`}
                >
                  {s.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>

        {/* Content */}
        <div className="flex-1 max-w-md">
          {section === 'notifications' && (
            <>
              <h1 className="text-2xl font-bold text-black mb-2">Notifications</h1>
              <p className="text-black/80 mb-8">Get emailed when things break.</p>

              <div className="space-y-6">
                <div className="space-y-4">
                  <label className="flex items-start gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={form.notify_new_errors}
                      onChange={(e) => handleChange('notify_new_errors', e.target.checked)}
                      className="mt-0.5 w-4 h-4 border-2 border-black/20 rounded-sm checked:bg-black checked:border-black"
                    />
                    <div>
                      <span className="block font-semibold text-black">New errors</span>
                      <span className="block text-sm text-black/80">First occurrence of an error ever</span>
                    </div>
                  </label>

                  <label className="flex items-start gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={form.notify_regressions}
                      onChange={(e) => handleChange('notify_regressions', e.target.checked)}
                      className="mt-0.5 w-4 h-4 border-2 border-black/20 rounded-sm checked:bg-black checked:border-black"
                    />
                    <div>
                      <span className="block font-semibold text-black">Regressions</span>
                      <span className="block text-sm text-black/80">Error returns after being quiet for 24+ hours</span>
                    </div>
                  </label>

                  <label className="flex items-start gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={form.notify_spikes}
                      onChange={(e) => handleChange('notify_spikes', e.target.checked)}
                      className="mt-0.5 w-4 h-4 border-2 border-black/20 rounded-sm checked:bg-black checked:border-black"
                    />
                    <div>
                      <span className="block font-semibold text-black">Error rate spikes</span>
                      <span className="block text-sm text-black/80">Endpoint error rate jumps above threshold</span>
                    </div>
                  </label>
                </div>

                {form.notify_spikes && (
                  <div>
                    <label className="block text-sm font-semibold text-black mb-1.5">
                      Spike threshold
                    </label>
                    <div className="flex items-center gap-2">
                      <input
                        type="number"
                        min="1"
                        max="100"
                        value={form.spike_threshold}
                        onChange={(e) => handleChange('spike_threshold', parseInt(e.target.value) || 10)}
                        className="w-20 px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black focus:border-transparent"
                      />
                      <span className="text-sm text-black/80">% error rate</span>
                    </div>
                  </div>
                )}

                <div className="pt-2">
                  <p className="text-sm font-semibold text-black mb-2">Channels</p>
                  {isEmailConfigured ? (
                    <div className="flex items-center gap-2">
                      <span className="inline-flex items-center px-2 py-1 text-sm font-medium border-2 border-black bg-black text-white">
                        Email
                      </span>
                    </div>
                  ) : (
                    <p className="text-sm text-black/80">
                      <a href="/settings?section=email" className="underline hover:text-black">Set up email</a> to receive notifications.
                    </p>
                  )}
                </div>
              </div>

              {message && (
                <div
                  className={`mt-6 p-3 text-sm font-medium ${
                    message.type === 'success'
                      ? 'bg-[#00D678]/10 text-black'
                      : 'bg-red-500/10 text-red-600'
                  }`}
                >
                  {message.text}
                </div>
              )}

              <div className="mt-6">
                <button
                  onClick={handleSave}
                  disabled={saving}
                  className="px-4 py-2 bg-black text-white text-sm font-semibold hover:bg-black/80 disabled:opacity-40 transition-colors"
                >
                  {saving ? 'Saving...' : 'Save'}
                </button>
              </div>
            </>
          )}

          {section === 'email' && (
            <>
              <h1 className="text-2xl font-bold text-black mb-2">Email</h1>
              <p className="text-black/80 mb-8">Send notifications via email.</p>

              <div className="space-y-5">
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
                  <span className="text-sm text-black/80">For links in email notifications</span>
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
                    <span className="text-sm text-black/80">Leave blank to keep current</span>
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

              {message && (
                <div
                  className={`mt-6 p-3 text-sm font-medium ${
                    message.type === 'success'
                      ? 'bg-[#00D678]/10 text-black'
                      : 'bg-red-500/10 text-red-600'
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
                <button
                  onClick={handleTest}
                  disabled={testing || !isEmailConfigured}
                  className="px-4 py-2 border-2 border-black/20 text-sm font-semibold text-black/80 hover:border-black/40 hover:text-black disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  {testing ? 'Sending...' : 'Send test email'}
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </Layout>
  )
}
