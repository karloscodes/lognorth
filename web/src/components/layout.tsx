import { ReactNode, useEffect, useRef, useState } from 'react'
import avatarUrl from '@/assets/avatar.jpg'
import { Link, router, usePage } from '@inertiajs/react'
import type { Application } from '@/lib/shared'
import { typing } from '@/lib/keys'

interface LayoutProps {
  children: ReactNode
  currentPath?: string
  appId?: number
  appName?: string
  applications?: Application[]
}

// The screen name shown in the status bar, from the URL.
function screenName(path: string): string {
  if (path === '/') return 'home'
  if (path.startsWith('/settings')) return 'settings'
  if (path === '/apps') return 'apps'
  if (/\/issues\/[^/]+/.test(path)) return 'issue'
  if (path.endsWith('/issues')) return 'issues'
  if (/\/log\/[^/]+/.test(path)) return 'log'
  if (path.endsWith('/log')) return 'logs'
  if (path.endsWith('/endpoints')) return 'endpoints'
  return ''
}

// Keys that act on what is on screen, shown in the status bar.
const SCREEN_KEYS: Record<string, [string, string][]> = {
  issues: [['j/k', 'move'], ['enter', 'open'], ['m', 'mute'], ['e', 'seen']],
  issue: [['esc', 'back'], ['m', 'mute']],
  logs: [['/', 'search']],
  log: [['esc', 'back']],
}

function Logo() {
  return (
    <span className="text-[17px] font-bold tracking-tight">
      <span className="text-accent">[</span>lognorth<span className="text-accent">]</span>
    </span>
  )
}

// Where switching apps lands: the same section, if the app has it.
function sectionPath(appId: number): string {
  const path = window.location.pathname
  if (path.includes('/log')) return `/apps/${appId}/log`
  if (path.includes('/endpoints')) return `/apps/${appId}/endpoints`
  return `/apps/${appId}/issues`
}

function FeedbackPanel({ onClose }: { onClose: () => void }) {
  return (
    <div className="pane bg-background fixed bottom-11 right-4 z-50 w-80 px-5 pb-4 pt-6 text-sm">
      <span className="pane-title">feedback</span>
      <div className="flex gap-4">
        <img src={avatarUrl} alt="Carlos" className="h-12 w-12 border border-border object-cover grayscale" style={{ objectPosition: 'center 1%' }} />
        <p className="text-text">Something broken or an idea?</p>
      </div>
      <div className="mt-3 flex flex-col gap-1">
        <a href="mailto:karloscodes@suppyhq.com?subject=LogNorth feedback" className="font-bold hover:underline">email me →</a>
        <a href="https://github.com/karloscodes/karloscodes/issues/new/choose" target="_blank" rel="noopener noreferrer" className="text-subtle hover:text-foreground hover:underline">open an issue →</a>
      </div>
      <button onClick={onClose} className="absolute right-3 top-2 text-xs text-dim hover:text-foreground">esc</button>
    </div>
  )
}

function HelpOverlay({ onClose, appScoped }: { onClose: () => void; appScoped: boolean }) {
  const rows = (items: [string, string][]) => (
    <dl className="grid grid-cols-[10ch_1fr] gap-x-4 gap-y-1.5 text-[13px]">
      {items.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="font-bold text-foreground">{k}</dt>
          <dd className="text-subtle">{v}</dd>
        </div>
      ))}
    </dl>
  )
  return (
    <div className="fixed inset-0 z-[70] grid place-items-center bg-background/80" onClick={onClose}>
      <div className="pane w-[min(520px,calc(100vw-32px))] bg-background px-6 pb-5 pt-7" onClick={(e) => e.stopPropagation()}>
        <span className="pane-title">keys</span>
        <h4 className="mb-2 text-xs text-dim">navigate · ctrl+b, then</h4>
        {rows([
          ...(appScoped ? ([['1 2 3', 'issues, logs, endpoints'], ['h / l', 'previous, next tab'], ['w', 'switch app']] as [string, string][]) : []),
          ['0', 'home'],
          ['a', 'apps'],
          ['s', 'settings'],
          ['enter', 'go to the highlight'],
          ['esc', 'cancel'],
        ])}
        <h4 className="mb-2 mt-4 text-xs text-dim">on the screen</h4>
        {rows([
          ['j / k', 'down, up'],
          ['enter', 'open'],
          ['esc', 'back'],
          ['m', 'mute issue'],
          ['e / E', 'mark seen, mark all seen'],
          ['/', 'search logs'],
          ['?', 'these keys'],
        ])}
      </div>
    </div>
  )
}

export function Layout({ children, currentPath, appId, appName, applications }: LayoutProps) {
  const { props, url } = usePage<{ isDemo?: boolean; licenseValid?: boolean }>()
  const isDemo = props.isDemo ?? false
  const licenseValid = props.licenseValid ?? true
  const appScoped = Boolean(appId && appName && applications)
  const path = currentPath ?? url.split('?')[0]
  // The status bar names the real screen: an issue page passes "/issues" as its tab.
  const screen = screenName(url.split('?')[0])

  const tabs = appScoped
    ? [
        { key: '1', label: 'issues', href: `/apps/${appId}/issues`, on: path.includes('/issues') },
        { key: '2', label: 'logs', href: `/apps/${appId}/log`, on: path.includes('/log') },
        { key: '3', label: 'endpoints', href: `/apps/${appId}/endpoints`, on: path.includes('/endpoints') },
      ]
    : []

  // Navigate mode (ctrl+b), the app picker, help and feedback.
  const [prefix, setPrefix] = useState(false)
  const [picking, setPicking] = useState(false)
  const [pickSel, setPickSel] = useState(0)
  const [tabSel, setTabSel] = useState(-1)
  const [help, setHelp] = useState(false)
  const [feedback, setFeedback] = useState(false)
  const [hints, setHints] = useState<{ key: string; left: number; top: number }[]>([])
  const pickerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const c = document.documentElement.classList
    c.toggle('prefix', prefix)
    c.toggle('picking', picking)
  }, [prefix, picking])

  // Hint badges on everything with a data-key, placed from their screen position.
  const placeHints = () => {
    const found: { key: string; left: number; top: number }[] = []
    document.querySelectorAll<HTMLElement>('[data-key]').forEach((el) => {
      const r = el.getBoundingClientRect()
      if (!r.width) return
      found.push({ key: el.dataset.key!, left: Math.max(4, r.left - 10), top: Math.max(2, r.top - 12) })
    })
    setHints(found)
  }

  const openPrefix = () => {
    setHelp(false)
    setFeedback(false)
    setTabSel(tabs.findIndex((t) => t.on))
    setPrefix(true)
    placeHints()
  }
  const closePrefix = () => {
    setPrefix(false)
    setTabSel(-1)
    setHints([])
  }
  const go = (href: string) => {
    closePrefix()
    router.visit(href)
  }
  const openPicker = () => {
    if (!applications?.length) return
    closePrefix()
    setPickSel(Math.max(0, applications.findIndex((a) => a.id === appId)))
    setPicking(true)
  }
  const choose = (id: number) => {
    setPicking(false)
    if (id !== appId) router.visit(sectionPath(id))
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const isPrefix = e.ctrlKey && !e.metaKey && !e.altKey && e.key.toLowerCase() === 'b'

      if (picking && applications) {
        e.preventDefault()
        if (e.key === 'j' || e.key === 'ArrowDown') setPickSel((s) => Math.min(applications.length - 1, s + 1))
        if (e.key === 'k' || e.key === 'ArrowUp') setPickSel((s) => Math.max(0, s - 1))
        if (e.key === 'Enter') choose(applications[pickSel].id)
        if (e.key === 'Escape' || isPrefix) setPicking(false)
        return
      }

      if (prefix) {
        e.preventDefault()
        if (isPrefix || e.key === 'Escape') return closePrefix()
        const tab = tabs.find((t) => t.key === e.key)
        if (tab) return go(tab.href)
        if (tabs.length && ['l', 'n', 'ArrowRight'].includes(e.key)) return setTabSel((s) => Math.min(tabs.length - 1, s + 1))
        if (tabs.length && ['h', 'p', 'ArrowLeft'].includes(e.key)) return setTabSel((s) => Math.max(0, s - 1))
        if (e.key === 'Enter') return tabSel >= 0 && tabs[tabSel] ? go(tabs[tabSel].href) : closePrefix()
        if (e.key === '0') return go('/')
        if (e.key === 'a') return go('/apps')
        if (e.key === 's') return go('/settings')
        if (e.key === 'w') return openPicker()
        if (e.key === '?') {
          closePrefix()
          setHelp(true)
        }
        return
      }

      if (isPrefix) {
        e.preventDefault()
        return openPrefix()
      }
      if (typing(e)) return
      if (e.key === 'Escape') {
        if (help) return setHelp(false)
        if (feedback) return setFeedback(false)
      }
      if (e.key === '?') setHelp((h) => !h)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  // A click anywhere outside the header and status bar leaves navigate mode;
  // a click outside the picker closes it.
  useEffect(() => {
    const onDown = (e: PointerEvent) => {
      const t = e.target as HTMLElement
      if (prefix && !t.closest('header, .status-bar')) closePrefix()
      if (picking && !pickerRef.current?.contains(t)) setPicking(false)
    }
    document.addEventListener('pointerdown', onDown)
    return () => document.removeEventListener('pointerdown', onDown)
  })

  const screenKeys = SCREEN_KEYS[screen] ?? []

  return (
    <div className="min-h-screen bg-background pb-0 md:pb-7">
      {isDemo && (
        <div className="border-b border-line bg-raise text-xs text-subtle">
          <div className="mx-auto flex h-[30px] max-w-6xl items-center justify-between gap-4 px-4 md:px-8">
            <span>
              demo · read-only<span className="hidden md:inline">. Your install shows your own apps.</span>
            </span>
            <a href="https://lognorth.com" className="whitespace-nowrap border-b border-border text-foreground">Buy LogNorth · $99</a>
          </div>
        </div>
      )}
      {!isDemo && !licenseValid && (
        <div className="bg-error text-xs text-foreground">
          <div className="mx-auto flex min-h-[30px] max-w-6xl items-center px-4 py-1 md:px-8">
            <span>
              No valid license. Events arrive, but LogNorth does not process them yet.{' '}
              <Link href="/settings" className="font-bold underline hover:no-underline">Enter your license key →</Link>
            </span>
          </div>
        </div>
      )}

      <header className="sticky top-0 z-50 border-b border-line bg-background">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-5 px-4 md:px-8">
          <Link href="/" data-key="0" className="shrink-0">
            <Logo />
          </Link>

          {appScoped && applications && (
            <>
              <span className="hidden h-[22px] w-px bg-border md:block" />
              <div ref={pickerRef} className="relative hidden md:block">
                <button
                  data-key="w"
                  onClick={() => (picking ? setPicking(false) : openPicker())}
                  className="flex items-center gap-2 whitespace-nowrap border border-border px-2.5 py-1 text-[13px] hover:border-foreground"
                >
                  <i className="h-[7px] w-[7px] bg-accent" />
                  {appName}
                  <span className="text-dim">▾</span>
                </button>
                {picking && (
                  <div className="pane absolute left-0 top-[calc(100%+10px)] z-50 w-[360px] bg-background px-1.5 pb-2 pt-5" role="menu" aria-label="Switch app">
                    <span className="pane-title">apps</span>
                    {applications.map((app, i) => (
                      <button
                        key={app.id}
                        role="menuitem"
                        onClick={() => choose(app.id)}
                        className={`grid w-full grid-cols-[1fr_auto] gap-3 px-2.5 py-1.5 text-left text-[13px] ${
                          i === pickSel ? 'bg-foreground text-background' : 'text-text hover:bg-raise'
                        }`}
                      >
                        <span>
                          {app.name}
                          {app.id === appId && <span className={i === pickSel ? '' : 'text-dim'}> *</span>}
                        </span>
                        <span className={i === pickSel ? '' : 'text-dim'}>{app.id === appId ? 'here' : ''}</span>
                      </button>
                    ))}
                    <div className="mx-2 my-1.5 border-t border-dashed border-border" />
                    <Link href="/" className="block px-2.5 py-1.5 text-[13px] text-subtle hover:bg-raise hover:text-foreground">
                      all apps →
                    </Link>
                  </div>
                )}
              </div>

              <nav className="flex gap-1.5 overflow-x-auto text-[13px] scrollbar-hide" aria-label="Sections">
                {tabs.map((t, i) => (
                  <Link
                    key={t.key}
                    href={t.href}
                    className={`whitespace-nowrap px-1.5 py-1 ${t.on ? 'font-bold text-foreground' : 'text-subtle hover:text-foreground'} ${
                      prefix && i === tabSel ? 'bg-raise' : ''
                    }`}
                  >
                    <i
                      className={`mr-1 hidden not-italic md:inline ${
                        prefix ? 'bg-accent px-1 font-bold text-accent-foreground' : 'text-dim'
                      }`}
                    >
                      {t.key}
                    </i>
                    <b className={`font-bold ${t.on ? 'text-accent' : 'text-transparent'}`}>[</b>
                    {t.label}
                    <b className={`font-bold ${t.on ? 'text-accent' : 'text-transparent'}`}>]</b>
                  </Link>
                ))}
              </nav>
            </>
          )}

          <span className="flex-1" />

          <nav className="flex items-center gap-4 text-[13px] text-subtle" aria-label="Account">
            <Link href="/apps" data-key="a" className={`hidden md:inline hover:text-foreground ${screen === 'apps' ? 'text-foreground' : ''}`}>
              apps
            </Link>
            <Link href="/settings" data-key="s" className={`hover:text-foreground ${screen === 'settings' ? 'text-foreground' : ''}`}>
              settings
            </Link>
            {!isDemo && (
              <button onClick={() => router.post('/logout')} className="hover:text-foreground">
                logout
              </button>
            )}
          </nav>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-10 md:px-8">{children}</main>

      {/* Hint badges: only while navigate mode is open. */}
      {prefix && (
        <div className="pointer-events-none fixed inset-0 z-[55] hidden md:block" aria-hidden="true">
          {hints.map((h) => (
            <span key={`${h.key}-${h.left}`} className="absolute bg-accent px-[5px] py-[3px] text-[11px] font-bold leading-none text-accent-foreground" style={{ left: h.left, top: h.top }}>
              {h.key}
            </span>
          ))}
        </div>
      )}

      {/* Status bar: mode, screen, and the keys that work right now. Hidden on phones. */}
      <div className="status-bar fixed inset-x-0 bottom-0 z-40 hidden h-7 border-t border-line bg-raise text-xs text-subtle md:flex">
        <span className={`flex items-center px-3 font-bold uppercase ${prefix || picking ? 'bg-accent text-accent-foreground' : 'bg-border text-foreground'}`}>
          {picking ? 'apps' : prefix ? 'navigate' : 'normal'}
        </span>
        {screen && <span className="flex items-center border-r border-line px-3 text-foreground">{screen}</span>}
        {isDemo && <span className="flex items-center border-r border-line px-3 text-foreground">[RO] demo</span>}
        {appName && (
          <span className="flex items-center px-3">
            {appName}
          </span>
        )}
        <span className="flex-1" />
        <span className="flex items-center gap-3.5 px-3">
          {(prefix
            ? ([
                ...(tabs.length ? ([['1-3', 'tab'], ['h/l', 'move'], ['w', 'apps']] as [string, string][]) : []),
                ['0', 'home'],
                ['s', 'settings'],
                ['esc', 'cancel'],
              ] as [string, string][])
            : picking
              ? ([['j/k', 'move'], ['enter', 'switch'], ['esc', 'cancel']] as [string, string][])
              : screenKeys
          ).map(([k, v]) => (
            <span key={k}>
              <kbd className="mr-1 font-[inherit] text-foreground">{k}</kbd>
              {v}
            </span>
          ))}
        </span>
        {!prefix && !picking && (
          <>
            <button onClick={() => setHelp(true)} className="flex items-center border-l border-line px-3 hover:text-foreground">
              <kbd className="mr-1 font-[inherit] text-foreground">?</kbd>keys
            </button>
            <button onClick={(e) => { e.stopPropagation(); openPrefix() }} className="flex items-center border-l border-line px-3 hover:text-foreground">
              <kbd className="mr-1 font-[inherit] text-foreground">ctrl+b</kbd>navigate
            </button>
            {!isDemo && (
              <button onClick={() => setFeedback((f) => !f)} className="flex items-center border-l border-line px-3 hover:text-foreground">
                feedback
              </button>
            )}
          </>
        )}
      </div>

      {help && <HelpOverlay onClose={() => setHelp(false)} appScoped={appScoped} />}
      {feedback && <FeedbackPanel onClose={() => setFeedback(false)} />}
    </div>
  )
}
