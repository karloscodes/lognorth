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
  <span className="text-2xl font-black font-mono tracking-tight" style={{ WebkitTextStroke: '1px currentColor' }}>
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
      className="text-sm font-bold text-black"
    >
      Logout
    </button>
  )
}

interface NotificationItem {
  id: string
  title: string
  body: string
  expires: string
}

function getDismissedIds(): string[] {
  return JSON.parse(localStorage.getItem('lognorth_dismissed_notifications') || '[]')
}

function getNotificationsByState(notifications: NotificationItem[]): { unread: NotificationItem[]; read: NotificationItem[] } {
  const now = new Date().toISOString().slice(0, 10)
  const dismissed = getDismissedIds()
  const valid = notifications.filter(n => n.expires >= now)
  return {
    unread: valid.filter(n => !dismissed.includes(n.id)),
    read: valid.filter(n => dismissed.includes(n.id)),
  }
}

function dismissNotification(id: string) {
  const dismissed: string[] = JSON.parse(localStorage.getItem('lognorth_dismissed_notifications') || '[]')
  if (!dismissed.includes(id)) {
    localStorage.setItem('lognorth_dismissed_notifications', JSON.stringify([...dismissed, id]))
  }
}

function FeedbackWidget({ isDemo, notifications = [] }: { isDemo: boolean; notifications: NotificationItem[] }) {
  const [open, setOpen] = useState(false)
  const [state, setState] = useState(() => getNotificationsByState(notifications))
  const [expandedRead, setExpandedRead] = useState<string | null>(null)
  const hasNotifications = state.unread.length > 0
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  const handleDismiss = (id: string) => {
    dismissNotification(id)
    setState(prev => {
      const dismissed = prev.unread.find(n => n.id === id)
      return {
        unread: prev.unread.filter(n => n.id !== id),
        read: dismissed ? [dismissed, ...prev.read] : prev.read,
      }
    })
  }

  return (
    <div ref={ref} className="fixed bottom-8 right-8 z-50 flex flex-col items-end gap-3">
      <div className="group">
        {open && (
          <div className="bg-white border border-black/10 shadow-lg rounded-xl p-4 w-80 text-sm mb-3 max-h-96 overflow-y-auto">
            {state.unread.map((n, i) => (
              <div key={n.id} className={i > 0 ? 'mt-3 pt-3 border-t border-black/10' : ''}>
                <div className="flex items-start justify-between gap-2">
                  <p className="font-semibold text-sm text-black mb-1">{n.title}</p>
                  <button onClick={() => handleDismiss(n.id)} className="text-black/30 hover:text-black text-xs shrink-0">✕</button>
                </div>
                <p className="text-black/70 text-xs leading-relaxed whitespace-pre-wrap break-all">{n.body}</p>
              </div>
            ))}
            {state.read.map((n, i) => (
              <div
                key={n.id}
                className={`${state.unread.length > 0 || i > 0 ? 'mt-3 pt-3 border-t border-black/10' : ''} cursor-pointer`}
                onClick={() => setExpandedRead(expandedRead === n.id ? null : n.id)}
              >
                <p className="text-sm text-black/40 hover:text-black/60 transition-colors">{n.title}</p>
                {expandedRead === n.id && (
                  <p className="text-black/40 text-xs leading-relaxed whitespace-pre-wrap break-all mt-1">{n.body}</p>
                )}
              </div>
            ))}
            {(state.unread.length > 0 || state.read.length > 0) && <div className="mt-3 pt-3 border-t border-black/10" />}
            <p className="text-black/70 text-sm leading-relaxed mb-3">
              Something broken or an idea?
            </p>
            <div className="flex flex-col gap-2">
              {!isDemo && (
                <a
                  href="mailto:me@karloscodes.com?subject=LogNorth feedback"
                  className="font-semibold text-sm hover:underline"
                >
                  email me →
                </a>
              )}
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
          <div className="relative">
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
            {hasNotifications && (
              <span className="absolute top-0 right-0 w-4 h-4 bg-green-500 rounded-full border-2 border-white animate-pulse" />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export function Layout({ children, currentPath, appId, appName, applications }: LayoutProps) {
  const { props } = usePage<{ isDemo?: boolean; notifications?: NotificationItem[]; hasLicense?: boolean }>()
  const isDemo = props.isDemo ?? false
  const notifications = props.notifications ?? []
  const hasLicense = props.hasLicense ?? false
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
      {isDemo && (
        <div className="bg-red-600 text-white text-center text-xs font-bold py-1.5 tracking-wide uppercase sticky top-0 z-[60]">
          This is a demo. Everything is read-only.
        </div>
      )}
      {!isDemo && !hasLicense && (
        <div className="bg-black text-white text-center text-xs font-medium py-1.5 sticky top-0 z-[60]">
          Beta ends April 30, 2026. A license key will be required after that.{' '}
          <Link href="/settings" className="underline hover:no-underline">
            Enter your license key →
          </Link>
        </div>
      )}
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
                    className={`flex items-center gap-2 py-5 text-sm font-bold text-black border-b-2 ${
                      currentPath?.includes('/issues')
                        ? 'border-black'
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
                    className={`py-5 text-sm font-bold text-black border-b-2 ${
                      currentPath?.includes('/log')
                        ? 'border-black'
                        : 'border-transparent'
                    }`}
                  >
                    Logs
                  </Link>
                  <Link
                    href={`/apps/${appId}/endpoints`}
                    className={`py-5 text-sm font-bold text-black border-b-2 ${
                      currentPath?.includes('/endpoints')
                        ? 'border-black'
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
                  className={`py-5 text-sm font-bold text-black border-b-2 ${
                    currentPath === '/'
                      ? 'border-black'
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
                className="text-sm font-bold text-black"
              >
                Apps
              </Link>

              <Link
                href="/settings"
                className="text-sm font-bold text-black"
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

      {!isDemo && <FeedbackWidget isDemo={isDemo} notifications={notifications} />}
    </div>
  )
}
