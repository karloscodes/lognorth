import { useState } from 'react'
import { AuthShell, FIELD, LABEL, SUBMIT, Notice } from '@/components/auth-shell'

interface Flash {
  type?: 'error' | 'success'
  message?: string
}

interface Props {
  email?: string
  flash?: Flash
}

export default function Onboarding({ email, flash }: Props) {
  const [emailValue, setEmailValue] = useState(email || '')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  const tooShort = password.length > 0 && password.length < 8
  const mismatch = password.length >= 8 && confirmPassword.length > 0 && password !== confirmPassword

  return (
    <AuthShell title="~ $ lognorth setup">
      {flash?.type === 'error' && flash.message && <Notice kind="error">{flash.message}</Notice>}

      <h1 className="text-lg font-bold text-foreground">Create your account</h1>
      <p className="mb-5 mt-1 text-[13px] text-subtle">Set up your admin account to get started.</p>

      <form action="/setup" method="POST" className="space-y-4">
        <div>
          <label htmlFor="email" className={LABEL}>
            email
          </label>
          <input
            type="email"
            id="email"
            name="email"
            value={emailValue}
            onChange={(e) => setEmailValue(e.target.value)}
            className={FIELD}
            placeholder="you@example.com"
            autoFocus
            required
          />
        </div>
        <div>
          <label htmlFor="password" className={LABEL}>
            password
          </label>
          <input
            type="password"
            id="password"
            name="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className={FIELD}
            placeholder="At least 8 characters"
            required
            minLength={8}
          />
        </div>
        <div>
          <label htmlFor="confirm_password" className={LABEL}>
            confirm password
          </label>
          <input
            type="password"
            id="confirm_password"
            name="confirm_password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            className={FIELD}
            placeholder="Type it again"
            required
            minLength={8}
          />
        </div>
        <button type="submit" disabled={!emailValue || password.length < 8 || password !== confirmPassword} className={SUBMIT}>
          Create account
        </button>
        {tooShort && <p className="text-center text-xs text-dim">Password must be at least 8 characters.</p>}
        {mismatch && <p className="text-center text-xs text-dim">Passwords don't match.</p>}
      </form>
    </AuthShell>
  )
}
