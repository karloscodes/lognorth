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
        <h2 className="text-[22px] font-bold tracking-tight text-foreground">License</h2>
        {licenseStatus === 'valid' && (
          <span className="text-sm font-bold text-accent">
            ✓ Valid {licenseEmail && <span className="text-subtle font-normal">· {licenseEmail}</span>}
          </span>
        )}
        {licenseStatus === 'invalid' && (
          <span className="text-sm font-bold text-error">Invalid</span>
        )}
        {licenseStatus === 'expired' && (
          <span className="text-sm font-bold text-error">Expired</span>
        )}
      </div>
      <p className="text-subtle text-sm mb-6">
        Required to process events. Without one, events still arrive but aren't processed.
      </p>

      <div>
        <label className="block text-sm font-bold text-foreground mb-1.5">License key</label>
        <input
          type="text"
          value={licenseKey}
          onChange={(e) => {
            setLicenseKey(e.target.value)
            setMessage(null)
          }}
          placeholder="Paste your license key"
          className="w-full px-3 py-2 border border-border text-sm font-mono focus:outline-none focus:border-foreground placeholder:text-dim"
        />
        <div className="flex items-center gap-3 mt-3">
          <button
            onClick={handleSave}
            disabled={saving}
            className="bg-foreground text-background font-bold px-4 py-2 hover:bg-text disabled:opacity-50 text-sm"
          >
            {saving ? 'Saving...' : 'Save license'}
          </button>
          {licenseStatus !== 'valid' && (
            <a
              href={POLAR_CHECKOUT}
              target="_blank"
              rel="noopener noreferrer"
              className="font-bold text-sm hover:underline"
            >
              Get your license →
            </a>
          )}
          {message && (
            <span
              className={`text-sm font-medium ${
                message.type === 'success' ? 'text-subtle' : 'text-error'
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
