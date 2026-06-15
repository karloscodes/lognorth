import { useState } from 'react'

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
    <div className="min-h-screen bg-card flex items-center justify-center">
      <div className="w-full max-w-sm px-6">
        {/* Logo */}
        <div className="text-center mb-8">
          <span className="text-4xl font-black font-wordmark tracking-tight text-foreground"><span className="text-accent">[</span>lognorth<span className="text-accent">]</span></span>
          <p className="text-sm text-foreground/50 mt-2">Logging you own.</p>
        </div>

        {flash?.type === 'error' && flash.message && (
          <div className="bg-[#e11d48]/10 border-2 border-[#e11d48]/20 text-[#e11d48] px-4 py-3 text-sm font-medium mb-6">
            {flash.message}
          </div>
        )}

        {flash?.type === 'success' && flash.message && (
          <div className="bg-accent/10 border-2 border-accent/20 text-foreground px-4 py-3 text-sm font-medium mb-6">
            {flash.message}
          </div>
        )}

        <form action="/login" method="POST" className="space-y-5">
          <div>
            <label htmlFor="email" className="block text-sm font-semibold text-foreground mb-2">
              Email
            </label>
            <input
              type="email"
              id="email"
              name="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
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
              placeholder="••••••••"
              required
            />
          </div>

          <button
            type="submit"
            className="w-full py-3 bg-foreground text-background font-semibold hover:bg-foreground/80 disabled:opacity-50 transition-colors"
          >
            Sign in
          </button>
        </form>
      </div>
    </div>
  )
}
