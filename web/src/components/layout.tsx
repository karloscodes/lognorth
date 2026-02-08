import { ReactNode, useState } from 'react'
import { Link, router } from '@inertiajs/react'
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
  <span className="text-2xl font-bold tracking-tight">
    LogNorth
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
        <button className="flex items-center gap-2 px-3 py-1.5 border border-black/20 hover:border-black/40 transition-colors text-sm font-semibold">
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
              className={`flex items-center gap-2 px-3 py-2 text-sm text-left rounded-sm transition-colors ${
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
            className="flex items-center gap-2 px-3 py-2 text-sm text-left rounded-sm hover:bg-black/[0.05] text-black/70"
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
      className="text-sm font-semibold text-black/80 hover:text-black transition-colors"
    >
      Logout
    </button>
  )
}

export function Layout({ children, currentPath, appId, appName, applications }: LayoutProps) {
  const isAppScoped = appId && appName && applications

  const isActive = (path: string) => {
    if (!currentPath) return false
    if (path === '/') return currentPath === '/'
    return currentPath.startsWith(path)
  }

  const navLinkClass = (path: string) =>
    `relative py-4 text-sm font-semibold transition-colors ${
      isActive(path) ? 'text-black' : 'text-black/80 hover:text-black'
    }`

  return (
    <div className="min-h-screen bg-white">
      {/* Navigation */}
      <nav className="border-b border-black/10 sticky top-0 bg-white z-50">
        <div className="max-w-5xl mx-auto px-6">
          <div className="flex h-14 items-center gap-8">
            <Link href="/" className="mr-2">
              <LogNorthLogo />
            </Link>

            {isAppScoped ? (
              <>
                <AppSelector appId={appId} appName={appName} applications={applications} />

                <div className="flex items-center gap-1">
                  <Link
                    href={`/apps/${appId}/issues`}
                    className={`px-3 py-1.5 text-sm font-semibold transition-colors ${
                      currentPath?.includes('/issues')
                        ? 'bg-black text-white'
                        : 'text-black/80 hover:text-black'
                    }`}
                  >
                    Issues
                  </Link>
                  <Link
                    href={`/apps/${appId}/log`}
                    className={`px-3 py-1.5 text-sm font-semibold transition-colors ${
                      currentPath?.includes('/log')
                        ? 'bg-black text-white'
                        : 'text-black/80 hover:text-black'
                    }`}
                  >
                    Logs
                  </Link>
                  <Link
                    href={`/apps/${appId}/endpoints`}
                    className={`px-3 py-1.5 text-sm font-semibold transition-colors ${
                      currentPath?.includes('/endpoints')
                        ? 'bg-black text-white'
                        : 'text-black/80 hover:text-black'
                    }`}
                  >
                    Endpoints
                  </Link>
                </div>
              </>
            ) : (
              <>
                <Link href="/" className={navLinkClass('/')}>
                  Home
                  {currentPath === '/' && <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-black" />}
                </Link>
                <Link href="/issues" className={navLinkClass('/issues')}>
                  Issues
                  {currentPath === '/issues' && <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-black" />}
                </Link>
              </>
            )}

            <div className="flex-1" />

            <Link href="/apps" className={navLinkClass('/apps')}>
              Apps
              {isActive('/apps') && <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-black" />}
            </Link>

            <Link
              href="/settings"
              className={`text-sm font-semibold transition-colors ${
                isActive('/settings') ? 'text-black' : 'text-black/80 hover:text-black'
              }`}
            >
              Settings
            </Link>

            <LogoutButton />
          </div>
        </div>
      </nav>

      {/* Main Content */}
      <main className="max-w-5xl mx-auto px-6 py-8">
        {children}
      </main>

      {/* Footer */}
      <footer className="border-t border-black/10 mt-16">
        <div className="max-w-5xl mx-auto px-6 py-4" />
      </footer>
    </div>
  )
}
