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
      <div className="flex flex-col md:flex-row gap-12">
        <nav className="md:w-44 md:shrink-0">
          <h1 className="text-2xl font-bold text-foreground mb-4 md:mb-6">Settings</h1>
          <ul className="flex md:flex-col gap-1 md:gap-1 overflow-x-auto md:overflow-visible">
            {NAV.map((item) => {
              const active = currentPath.startsWith(item.path)
              return (
                <li key={item.path} className="shrink-0">
                  <Link
                    href={item.path}
                    className={`block py-1.5 px-2 -mx-2 text-sm font-semibold whitespace-nowrap ${
                      active
                        ? 'text-foreground border-l-2 border-foreground md:pl-3'
                        : 'text-foreground/50 hover:text-foreground md:pl-3'
                    }`}
                  >
                    {item.label}
                  </Link>
                </li>
              )
            })}
          </ul>
        </nav>
        <div className="flex-1 max-w-xl">{children}</div>
      </div>
    </Layout>
  )
}
