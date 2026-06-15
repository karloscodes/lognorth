import { useState } from 'react'
import { SettingsLayout } from '@/components/settings-layout'

interface SettingsData {
  license_key: string
  license_status: string
  license_email: string
  // remaining fields preserved on save so we don't clobber other sections
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
  retention_days: number
}

interface Props {
  settings?: SettingsData
}

const POLAR_CHECKOUT = 'https://buy.polar.sh/polar_cl_HUhnZchaYYtSmTR20EtVi2BHJkoKypUJHgS001PLGBK'

export default function SettingsLicense({ settings }: Props) {
  const [licenseKey, setLicenseKey] = useState(settings?.license_key || '')
  const [licenseStatus, setLicenseStatus] = useState(settings?.license_status || '')
  const [licenseEmail, setLicenseEmail] = useState(settings?.license_email || '')
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
        body: JSON.stringify({ ...settings, license_key: licenseKey }),
      })
      const data = await res.json()
      if (data.success) {
        setLicenseStatus(data.license_status)
        setLicenseEmail(data.license_email || '')
        setMessage({ type: 'success', text: 'License saved' })
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
    <SettingsLayout currentPath="/settings/license">
      <div className="flex items-baseline gap-3 mb-2 flex-wrap">
        <h2 className="text-2xl font-bold text-foreground">License</h2>
        {licenseStatus === 'valid' && (
          <span className="text-sm font-semibold text-accent">
            ✓ Valid {licenseEmail && <span className="text-foreground/50 font-normal">· {licenseEmail}</span>}
          </span>
        )}
        {licenseStatus === 'invalid' && (
          <span className="text-sm font-semibold text-red-600">Invalid</span>
        )}
        {licenseStatus === 'expired' && (
          <span className="text-sm font-semibold text-red-600">Expired</span>
        )}
      </div>
      <p className="text-foreground/50 text-sm mb-6">
        Required to process events. Without one, events still arrive but aren't processed.
      </p>

      <div>
        <label className="block text-sm font-semibold text-foreground mb-1.5">License key</label>
        <input
          type="text"
          value={licenseKey}
          onChange={(e) => {
            setLicenseKey(e.target.value)
            setMessage(null)
          }}
          placeholder="Paste your license key"
          className="w-full px-3 py-2 border border-foreground/20 text-sm font-mono focus:outline-none focus:ring-1 focus:ring-foreground focus:border-transparent placeholder:text-foreground/40"
        />
        <div className="flex items-center gap-3 mt-3">
          <button
            onClick={handleSave}
            disabled={saving}
            className="bg-foreground text-background font-semibold px-4 py-2 hover:bg-foreground/80 disabled:opacity-50 text-sm"
          >
            {saving ? 'Saving...' : 'Save license'}
          </button>
          {licenseStatus !== 'valid' && (
            <a
              href={POLAR_CHECKOUT}
              target="_blank"
              rel="noopener noreferrer"
              className="font-semibold text-sm hover:underline"
            >
              Get your license →
            </a>
          )}
          {message && (
            <span
              className={`text-sm font-medium ${
                message.type === 'success' ? 'text-foreground/60' : 'text-red-600'
              }`}
            >
              {message.text}
            </span>
          )}
        </div>
      </div>
    </SettingsLayout>
  )
}
