import { useState } from 'react'

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

  return (
    <div className="min-h-screen bg-card flex items-center justify-center">
      <div className="w-full max-w-sm px-6">
        {/* Logo */}
        <div className="text-center mb-8">
          <span className="text-4xl font-black font-wordmark tracking-tight"><span className="text-accent">[</span>lognorth<span className="text-accent">]</span></span>
          <p className="text-sm font-medium text-foreground/60 mt-2">Logging you own.</p>
        </div>

        {flash?.type === 'error' && flash.message && (
          <div className="bg-[#e11d48]/10 border-2 border-[#e11d48]/20 text-[#e11d48] px-4 py-3 text-sm font-medium mb-6">
            {flash.message}
          </div>
        )}

        <h1 className="text-xl font-bold text-foreground text-center mb-2">Create your account</h1>
        <p className="text-sm text-foreground/60 text-center mb-8">
          Set up your admin account to get started.
        </p>

        <form action="/setup" method="POST" className="space-y-5">
          <div>
            <label htmlFor="email" className="block text-sm font-semibold text-foreground mb-2">
              Email address
            </label>
            <input
              type="email"
              id="email"
              name="email"
              value={emailValue}
              onChange={(e) => setEmailValue(e.target.value)}
              className="w-full px-4 py-3 border-2 border-foreground/20 focus:outline-none focus:ring-2 focus:ring-foreground focus:border-transparent text-foreground placeholder:text-foreground/40"
              placeholder="you@example.com"
              autoFocus
              required
            />
          </div>

          <div>
            <label htmlFor="password" className="block text-sm font-semibold text-foreground mb-2">
              Password
            </label>
            <input
              type="password"
              id="password"
              name="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-4 py-3 border-2 border-foreground/20 focus:outline-none focus:ring-2 focus:ring-foreground focus:border-transparent text-foreground placeholder:text-foreground/40"
              placeholder="At least 8 characters"
              required
              minLength={8}
            />
          </div>

          <div>
            <label htmlFor="confirm_password" className="block text-sm font-semibold text-foreground mb-2">
              Confirm password
            </label>
            <input
              type="password"
              id="confirm_password"
              name="confirm_password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="w-full px-4 py-3 border-2 border-foreground/20 focus:outline-none focus:ring-2 focus:ring-foreground focus:border-transparent text-foreground placeholder:text-foreground/40"
              placeholder="Re-enter your password"
              required
              minLength={8}
            />
          </div>

          <button
            type="submit"
            disabled={!emailValue || password.length < 8 || password !== confirmPassword}
            className="w-full py-3 bg-foreground text-background font-semibold hover:bg-foreground/80 disabled:bg-foreground/30 disabled:cursor-not-allowed transition-colors"
          >
            Create account
          </button>
          {password.length > 0 && password.length < 8 && (
            <p className="text-xs text-foreground/50 text-center mt-2">Password must be at least 8 characters</p>
          )}
          {password.length >= 8 && confirmPassword.length > 0 && password !== confirmPassword && (
            <p className="text-xs text-foreground/50 text-center mt-2">Passwords don't match</p>
          )}
        </form>
      </div>
    </div>
  )
}
