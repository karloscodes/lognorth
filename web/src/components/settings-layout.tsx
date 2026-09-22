import { ReactNode } from 'react'
import { Link } from '@inertiajs/react'
import { Layout } from '@/components/layout'

const NAV = [
  { path: '/settings/license', label: 'License' },
  { path: '/settings/alerts', label: 'Alerts' },
  { path: '/settings/data', label: 'Data' },
  { path: '/settings/developer', label: 'Developer' },
]

export function SettingsLayout({
  children,
  currentPath,
}: {
  children: ReactNode
  currentPath: string
}) {
  return (
    <Layout currentPath="/settings">
      <div className="flex flex-col gap-10 md:flex-row md:gap-12">
        <nav className="md:w-48 md:shrink-0" aria-label="Settings">
          <h1 className="sr-only">Settings</h1>
          <p className="mb-3 text-xs text-dim">~ $ ls settings/</p>
          <ul className="flex gap-4 overflow-x-auto md:flex-col md:gap-0 md:overflow-visible">
            {NAV.map((item) => {
              const active = currentPath.startsWith(item.path)
              return (
                <li key={item.path} className="shrink-0">
                  <Link
                    href={item.path}
                    className={`block whitespace-nowrap py-1 text-[13px] ${active ? 'font-bold text-foreground' : 'text-subtle hover:text-foreground'}`}
                  >
                    <span className={`hidden md:inline ${active ? 'text-accent' : 'text-transparent'}`}>&gt; </span>
                    {item.label.toLowerCase()}
                  </Link>
                </li>
              )
            })}
          </ul>
        </nav>
        <div className="max-w-xl flex-1">{children}</div>
      </div>
    </Layout>
  )
}
