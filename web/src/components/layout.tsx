import { ReactNode, useState } from 'react'
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
  <span className="text-2xl font-black font-mono tracking-tight" style={{ WebkitTextStroke: '0.5px currentColor' }}>
    <span className="text-lime-500">[</span>lognorth<span className="text-lime-500">]</span>
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
        <button className="flex items-center gap-2 px-3 py-1.5 border border-black/20 hover:border-black transition-colors text-sm font-semibold">
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
                  ? 'bg-black text-white'
                  : 'hover:bg-black/[0.05]'
              }`}
            >
              <Check className={`w-4 h-4 ${app.id === appId ? 'opacity-100' : 'opacity-0'}`} />
              {app.name}
            </button>
          ))}
          <div className="h-px bg-black/10 my-1" />
          <button
            onClick={() => handleSelect(null)}
            className="flex items-center gap-2 px-3 py-2 text-sm text-left hover:bg-black/[0.02] text-black/50"
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
      className="text-sm font-semibold text-black/50 hover:text-black transition-colors"
    >
      Logout
    </button>
  )
}

function FeedbackWidget() {
  const [open, setOpen] = useState(false)

  return (
    <div className="fixed bottom-8 right-8 z-50 flex flex-col items-end gap-3">
      <div
        className="group"
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
      >
        {open && (
          <div className="bg-white border border-black/10 shadow-lg rounded-xl p-4 w-60 text-sm mb-3">
            <p className="text-black/70 text-sm leading-relaxed mb-3">
              Something broken or an idea?
            </p>
            <div className="flex flex-col gap-2">
              <a
                href="mailto:me@karloscodes.com?subject=LogNorth feedback"
                className="font-semibold text-sm hover:underline"
              >
                email me →
              </a>
              <a
                href="https://github.com/karloscodes/karloscodes/issues/new/choose"
                target="_blank"
                rel="noopener noreferrer"
                className="text-sm text-black/50 hover:text-black hover:underline transition-colors"
              >
                open an issue →
              </a>
            </div>
          </div>
        )}
        <div className="flex justify-end">
          <div
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
  const { props } = usePage<{ isDemo?: boolean }>()
  const isDemo = props.isDemo ?? false
  const isAppScoped = appId && appName && applications

  const isActive = (path: string) => {
    if (!currentPath) return false
    if (path === '/') return currentPath === '/'
    return currentPath.startsWith(path)
  }

  // Get current app stats for nav indicator
  const currentApp = applications?.find(a => a.id === appId)
  const hasErrors = currentApp?.stats?.errors && currentApp.stats.errors > 0

  return (
    <div className="min-h-screen bg-white">
      {/* Navigation */}
      <nav className="border-b border-black/10 sticky top-0 bg-white z-50">
        <div className="max-w-6xl mx-auto px-8">
          <div className="flex h-16 items-center gap-10">
            <Link href="/" className="flex items-center gap-3">
              <LogNorthLogo />
            </Link>

            {isAppScoped ? (
              <>
                <div className="h-6 w-px bg-black/10" />

                <AppSelector appId={appId} appName={appName} applications={applications} />

                <nav className="flex items-center gap-6">
                  <Link
                    href={`/apps/${appId}/issues`}
                    className={`flex items-center gap-2 py-5 text-sm font-semibold border-b-2 transition-colors ${
                      currentPath?.includes('/issues')
                        ? 'border-black text-black'
                        : 'border-transparent text-black/50 hover:text-black'
                    }`}
                  >
                    Issues
                    {hasErrors && (
                      <span className="w-2 h-2 rounded-full bg-red-500" />
                    )}
                  </Link>
                  <Link
                    href={`/apps/${appId}/log`}
                    className={`py-5 text-sm font-semibold border-b-2 transition-colors ${
                      currentPath?.includes('/log')
                        ? 'border-black text-black'
                        : 'border-transparent text-black/50 hover:text-black'
                    }`}
                  >
                    Logs
                  </Link>
                  <Link
                    href={`/apps/${appId}/endpoints`}
                    className={`py-5 text-sm font-semibold border-b-2 transition-colors ${
                      currentPath?.includes('/endpoints')
                        ? 'border-black text-black'
                        : 'border-transparent text-black/50 hover:text-black'
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
                  className={`py-5 text-sm font-semibold border-b-2 transition-colors ${
                    currentPath === '/'
                      ? 'border-black text-black'
                      : 'border-transparent text-black/50 hover:text-black'
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
                className={`text-sm font-semibold transition-colors ${
                  isActive('/apps') ? 'text-black' : 'text-black/50 hover:text-black'
                }`}
              >
                Apps
              </Link>

              <Link
                href="/settings"
                className={`text-sm font-semibold transition-colors ${
                  isActive('/settings') ? 'text-black' : 'text-black/50 hover:text-black'
                }`}
              >
                Settings
              </Link>

              {!isDemo && (
                <>
                  <div className="h-5 w-px bg-black/10" />
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

      {/* Feedback Widget */}
      {!isDemo && <FeedbackWidget />}
    </div>
  )
}
