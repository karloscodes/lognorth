import { ReactNode, useState, useRef, useEffect } from 'react'
import avatarUrl from '@/assets/avatar.jpg'
import { Link, router, usePage } from '@inertiajs/react'
import { Check, ChevronDown, ArrowLeft } from 'lucide-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import type { Application } from '@/lib/shared'

interface LayoutProps {
  children: ReactNode
  currentPath?: string
  appId?: number
  appName?: string
  applications?: Application[]
}

const LogNorthLogo = () => (
  <span className="text-2xl font-black font-wordmark tracking-tight">
    <span className="text-accent">[</span>lognorth<span className="text-accent">]</span>
  </span>
)

function AppSelector({ appId, appName, applications }: { appId: number; appName: string; applications: Application[] }) {
  const [open, setOpen] = useState(false)

  const handleSelect = (id: number | null) => {
    setOpen(false)
    if (id === null) {
      router.visit('/')
      return
    }

    const currentPath = window.location.pathname
    let newPath = `/apps/${id}/issues`

    if (currentPath.includes('/issues')) {
      newPath = `/apps/${id}/issues`
    } else if (currentPath.includes('/log')) {
      newPath = `/apps/${id}/log`
    }

    router.visit(newPath)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button className="flex items-center gap-2 px-3 py-1.5 border border-foreground/20 hover:border-foreground transition-colors text-sm font-semibold">
          {appName}
          <ChevronDown className="w-4 h-4 opacity-50" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-48 p-1" align="start">
        <div className="flex flex-col">
          {applications.map((app) => (
            <button
              key={app.id}
              onClick={() => handleSelect(app.id)}
              className={`flex items-center gap-2 px-3 py-2 text-sm text-left transition-colors ${
                app.id === appId
                  ? 'bg-foreground text-background'
                  : 'hover:bg-foreground/[0.05]'
              }`}
            >
              <Check className={`w-4 h-4 ${app.id === appId ? 'opacity-100' : 'opacity-0'}`} />
              {app.name}
            </button>
          ))}
          <div className="h-px bg-foreground/10 my-1" />
          <button
            onClick={() => handleSelect(null)}
            className="flex items-center gap-2 px-3 py-2 text-sm text-left hover:bg-foreground/[0.02] text-foreground/50"
          >
            <ArrowLeft className="w-4 h-4" />
            All apps
          </button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function LogoutButton() {
  const handleLogout = () => {
    router.post('/logout')
  }

  return (
    <button
      onClick={handleLogout}
      className="text-sm font-bold text-foreground"
    >
      Logout
    </button>
  )
}

function FeedbackWidget() {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div ref={ref} className="fixed bottom-8 right-8 z-50 flex flex-col items-end gap-3">
      <div className="group">
        {open && (
          <div className="bg-card border border-foreground/10 shadow-lg p-4 w-80 text-sm mb-3">
            <p className="text-foreground/70 text-sm leading-relaxed mb-3">
              Something broken or an idea?
            </p>
            <div className="flex flex-col gap-2">
              <a
                href="mailto:karloscodes@suppyhq.com?subject=LogNorth feedback"
                className="font-semibold text-sm hover:underline"
              >
                email me →
              </a>
              <a
                href="https://github.com/karloscodes/karloscodes/issues/new/choose"
                target="_blank"
                rel="noopener noreferrer"
                className="text-sm text-foreground/50 hover:text-foreground hover:underline transition-colors"
              >
                open an issue →
              </a>
            </div>
          </div>
        )}
        <div className="flex justify-end">
          <div
            onClick={() => setOpen(!open)}
            className="rounded-full overflow-hidden w-14 h-14 ring-2 ring-white shadow-lg hover:scale-105 transition-transform cursor-pointer"
          >
            <img
              src={avatarUrl}
              alt="Carlos"
              className="w-full h-full object-cover"
              style={{ objectPosition: 'center 1%' }}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

export function Layout({ children, currentPath, appId, appName, applications }: LayoutProps) {
  const { props } = usePage<{ isDemo?: boolean; licenseValid?: boolean }>()
  const isDemo = props.isDemo ?? false
  const licenseValid = props.licenseValid ?? true
  const isAppScoped = appId && appName && applications

  // Get current app stats for nav indicator
  const currentApp = applications?.find(a => a.id === appId)
  const hasErrors = currentApp?.stats?.errors && currentApp.stats.errors > 0

  return (
    <div className="min-h-screen bg-card">
      {isDemo && (
        <div className="bg-red-600 text-white text-center text-xs font-bold py-1.5 tracking-wide uppercase sticky top-0 z-[60]">
          This is a demo. Everything is read-only.
        </div>
      )}
      {!isDemo && !licenseValid && (
        <div className="bg-[#e11d48] text-white text-center text-xs font-medium py-1.5 sticky top-0 z-[60]">
          No valid license — events are being received but not processed.{' '}
          <Link href="/settings" className="underline hover:no-underline">
            Enter your license key →
          </Link>
        </div>
      )}
      {/* Navigation */}
      <nav className="border-b border-foreground/10 sticky top-0 bg-card z-50">
        <div className="max-w-6xl mx-auto px-8">
          <div className="flex h-16 items-center gap-10">
            <Link href="/" className="flex items-center gap-3">
              <LogNorthLogo />
            </Link>

            {isAppScoped ? (
              <>
                <div className="h-6 w-px bg-foreground/10" />

                <AppSelector appId={appId} appName={appName} applications={applications} />

                <nav className="flex items-center gap-6">
                  <Link
                    href={`/apps/${appId}/issues`}
                    className={`flex items-center gap-2 py-5 text-sm font-bold text-foreground border-b-2 ${
                      currentPath?.includes('/issues')
                        ? 'border-foreground'
                        : 'border-transparent'
                    }`}
                  >
                    Issues
                    {hasErrors && (
                      <span className="w-2 h-2 rounded-full bg-[#e11d48]" />
                    )}
                  </Link>
                  <Link
                    href={`/apps/${appId}/log`}
                    className={`py-5 text-sm font-bold text-foreground border-b-2 ${
                      currentPath?.includes('/log')
                        ? 'border-foreground'
                        : 'border-transparent'
                    }`}
                  >
                    Logs
                  </Link>
                  <Link
                    href={`/apps/${appId}/endpoints`}
                    className={`py-5 text-sm font-bold text-foreground border-b-2 ${
                      currentPath?.includes('/endpoints')
                        ? 'border-foreground'
                        : 'border-transparent'
                    }`}
                  >
                    Endpoints
                  </Link>
                </nav>
              </>
            ) : (
              <nav className="flex items-center gap-6">
                <Link
                  href="/"
                  className={`py-5 text-sm font-bold text-foreground border-b-2 ${
                    currentPath === '/'
                      ? 'border-foreground'
                      : 'border-transparent'
                  }`}
                >
                  Home
                </Link>
              </nav>
            )}

            <div className="flex-1" />

            <div className="flex items-center gap-6">
              <Link
                href="/apps"
                className="text-sm font-bold text-foreground"
              >
                Apps
              </Link>

              <Link
                href="/settings"
                className="text-sm font-bold text-foreground"
              >
                Settings
              </Link>

              {!isDemo && (
                <>
                  <div className="h-5 w-px bg-foreground/10" />
                  <LogoutButton />
                </>
              )}
            </div>
          </div>
        </div>
      </nav>

      {/* Main Content */}
      <main className="max-w-6xl mx-auto px-8 py-10">
        {children}
      </main>

      {!isDemo && <FeedbackWidget />}
    </div>
  )
}
