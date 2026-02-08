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
    <div className="min-h-screen bg-white flex items-center justify-center">
      <div className="w-full max-w-sm px-6">
        {/* Logo */}
        <div className="text-center mb-12">
          <span className="text-4xl font-bold tracking-tight">LogNorth</span>
          <p className="text-sm font-medium text-black/60 mt-2">Smarter logs. Self-hosted.</p>
        </div>

        <form action="/login" method="POST" className="space-y-5">
          {flash?.type === 'error' && flash.message && (
            <div className="bg-red-500/10 border-2 border-red-500/20 text-red-600 px-4 py-3 text-sm font-medium">
              {flash.message}
            </div>
          )}

          {flash?.type === 'success' && flash.message && (
            <div className="bg-green-500/10 border-2 border-green-500/20 text-green-700 px-4 py-3 text-sm font-medium">
              {flash.message}
            </div>
          )}

          <div>
            <label htmlFor="email" className="block text-sm font-semibold text-black mb-2">
              Email
            </label>
            <input
              type="email"
              id="email"
              name="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-4 py-3 border-2 border-black/20 focus:outline-none focus:ring-2 focus:ring-black focus:border-transparent text-black placeholder:text-black/40"
              placeholder="you@example.com"
              autoFocus
              required
            />
          </div>

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
              placeholder="Enter your password"
              required
            />
          </div>

          <button
            type="submit"
            className="w-full py-3 bg-black text-white font-semibold hover:bg-black/80 disabled:opacity-50 transition-colors"
          >
            Sign in
          </button>
        </form>
      </div>
    </div>
  )
}
