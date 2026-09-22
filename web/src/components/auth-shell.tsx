import { ReactNode } from 'react'

// Text inputs on the form pages: a thin border that turns white on focus.
export const FIELD =
  'w-full border border-border bg-raise/40 px-3 py-2 text-[13px] text-foreground placeholder:text-dim focus:border-foreground focus:outline-none'

export const LABEL = 'mb-1.5 block text-xs text-subtle'

export const SUBMIT =
  'w-full bg-accent py-2.5 text-[13px] font-bold text-accent-foreground hover:bg-foreground disabled:cursor-not-allowed disabled:opacity-40'

// The pages you see before the app: sign in, create the account, add the first app.
// A centered wordmark over one pane, with the pane's title set in its border.
export function AuthShell({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-sm px-6">
        <div className="mb-10 text-center">
          <span className="text-4xl font-bold tracking-tight">
            <span className="text-accent">[</span>lognorth<span className="text-accent">]</span>
          </span>
          <p className="mt-2 text-[13px] text-subtle">Logging you own.</p>
        </div>
        <div className="pane px-5 pb-5 pt-7">
          <span className="pane-title">{title}</span>
          {children}
        </div>
      </div>
    </div>
  )
}

export function Notice({ kind, children }: { kind: 'error' | 'success'; children: ReactNode }) {
  return (
    <p className={`mb-4 border px-3 py-2 text-[13px] ${kind === 'error' ? 'border-error text-error' : 'border-accent text-foreground'}`}>{children}</p>
  )
}
