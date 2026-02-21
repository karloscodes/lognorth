import { useState } from 'react'

interface Flash {
  type?: 'error' | 'success'
  message?: string
}

interface Props {
  step: 'user_account' | 'password'
  email?: string
  flash?: Flash
}

export default function Onboarding({ step, email, flash }: Props) {
  const [emailValue, setEmailValue] = useState(email || '')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  return (
    <div className="min-h-screen bg-white flex items-center justify-center">
      <div className="w-full max-w-sm px-6">
        {/* Logo */}
        <div className="text-center mb-8">
          <span className="text-4xl font-black font-mono tracking-tight" style={{ WebkitTextStroke: '0.5px currentColor' }}><span className="text-accent">[</span>lognorth<span className="text-accent">]</span></span>
          <p className="text-sm font-medium text-black/60 mt-2">Error tracking you own.</p>
        </div>

        {/* Progress */}
        <div className="flex items-center justify-center gap-2 mb-8">
          <div className={`w-3 h-3 rounded-full ${step === 'user_account' ? 'bg-black' : 'bg-black/20'}`} />
          <div className={`w-3 h-3 rounded-full ${step === 'password' ? 'bg-black' : 'bg-black/20'}`} />
        </div>

        {flash?.type === 'error' && flash.message && (
          <div className="bg-[#e11d48]/10 border-2 border-[#e11d48]/20 text-[#e11d48] px-4 py-3 text-sm font-medium mb-6">
            {flash.message}
          </div>
        )}

        {step === 'user_account' && (
          <>
            <h1 className="text-xl font-bold text-black text-center mb-2">Create your account</h1>
            <p className="text-sm text-black/60 text-center mb-8">
              Set up your admin account to get started.
            </p>

            <form action="/setup/user" method="POST" className="space-y-5">
              <div>
                <label htmlFor="email" className="block text-sm font-semibold text-black mb-2">
                  Email address
                </label>
                <input
                  type="email"
                  id="email"
                  name="email"
                  value={emailValue}
                  onChange={(e) => setEmailValue(e.target.value)}
                  className="w-full px-4 py-3 border-2 border-black/20 focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent text-black placeholder:text-black/40"
                  placeholder="you@example.com"
                  autoFocus
                  required
                />
              </div>

              <button
                type="submit"
                disabled={!emailValue}
                className="w-full py-3 bg-black text-white font-semibold hover:bg-black/80 disabled:bg-black/30 disabled:cursor-not-allowed transition-colors"
              >
                Continue
              </button>
            </form>
          </>
        )}

        {step === 'password' && (
          <>
            <h1 className="text-xl font-bold text-black text-center mb-2">Set your password</h1>
            <p className="text-sm text-black/60 text-center mb-8">
              Choose a secure password for <span className="font-medium text-black">{email}</span>
            </p>

            <form action="/setup/password" method="POST" className="space-y-5">
              <div>
                <label htmlFor="password" className="block text-sm font-semibold text-black mb-2">
                  Password
                </label>
                <input
                  type="password"
                  id="password"
                  name="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full px-4 py-3 border-2 border-black/20 focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent text-black placeholder:text-black/40"
                  placeholder="At least 8 characters"
                  autoFocus
                  required
                  minLength={8}
                />
              </div>

              <div>
                <label htmlFor="confirm_password" className="block text-sm font-semibold text-black mb-2">
                  Confirm password
                </label>
                <input
                  type="password"
                  id="confirm_password"
                  name="confirm_password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  className="w-full px-4 py-3 border-2 border-black/20 focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent text-black placeholder:text-black/40"
                  placeholder="Re-enter your password"
                  required
                  minLength={8}
                />
              </div>

              <button
                type="submit"
                disabled={password.length < 8 || password !== confirmPassword}
                className="w-full py-3 bg-black text-white font-semibold hover:bg-black/80 disabled:bg-black/30 disabled:cursor-not-allowed transition-colors"
              >
                Create account
              </button>
              {password.length > 0 && password.length < 8 && (
                <p className="text-xs text-black/50 text-center mt-2">Password must be at least 8 characters</p>
              )}
              {password.length >= 8 && confirmPassword.length > 0 && password !== confirmPassword && (
                <p className="text-xs text-black/50 text-center mt-2">Passwords don't match</p>
              )}
            </form>
          </>
        )}
      </div>
    </div>
  )
}
