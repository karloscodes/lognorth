import { useState } from 'react'
import { AuthShell, FIELD, LABEL, SUBMIT, Notice } from '@/components/auth-shell'

interface Flash {
  type?: 'error' | 'success'
  message?: string
}

interface Props {
  flash?: Flash
}

export default function Login({ flash }: Props) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  return (
    <AuthShell title="~ $ lognorth login">
      {flash?.type === 'error' && flash.message && <Notice kind="error">{flash.message}</Notice>}
      {flash?.type === 'success' && flash.message && <Notice kind="success">{flash.message}</Notice>}

      <form action="/login" method="POST" className="space-y-4">
        <div>
          <label htmlFor="email" className={LABEL}>
            email
          </label>
          <input
            type="email"
            id="email"
            name="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
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
            placeholder="••••••••"
            required
          />
        </div>
        <button type="submit" className={SUBMIT}>
          Sign in
        </button>
      </form>
    </AuthShell>
  )
}
