import { useState } from 'react'
import { Link } from '@inertiajs/react'
import { SettingsLayout } from '@/components/settings-layout'
import { FIELD, Notice } from '@/components/auth-shell'

interface StatusPage {
  id: number
  name: string
  slug: string
  domain: string // "" when it has none
  apps: string[] // names of the apps on it
}

interface Props {
  pages?: StatusPage[]
  host?: string // the domain LogNorth runs on, for the DNS step
  flash?: { type?: string; message?: string }
}

export default function SettingsStatus({ pages = [], host, flash }: Props) {
  return (
    <SettingsLayout currentPath="/settings/status">
      <h2 className="mb-2 text-[22px] font-bold tracking-tight text-foreground">Status pages</h2>
      <p className="mb-6 text-sm text-subtle">
        One public page per product: its apps' uptime, outages, and your notes. Pick each app's page on the{' '}
        <Link href="/apps" className="text-foreground hover:underline">
          Apps page
        </Link>
        .
      </p>

      {flash?.message && (flash.type === 'error' || flash.type === 'success') && <Notice kind={flash.type}>{flash.message}</Notice>}

      <div className="space-y-5">
        {pages.map((page) => (
          <PageRow key={page.id} page={page} host={host} />
        ))}
        <NewPage first={pages.length === 0} />
      </div>

      <div className="pane mt-10 px-4 pb-4 pt-6 text-[13px]">
        <span className="pane-title">own domain</span>
        <p className="text-subtle">A page can also answer at a domain of its own, like status.example.com. This LogNorth serves it, next to {host || 'its own domain'}. Three steps make it work:</p>
        <ol className="mt-2 list-decimal space-y-1.5 pl-5 text-text">
          <li>DNS sends the visitor to this server: a CNAME record from the domain to {host || 'the domain LogNorth runs on'}.</li>
          <li>Your proxy sends the domain to LogNorth, and gets its HTTPS certificate. On Chasen: chasen domains add.</li>
          <li>LogNorth reads the domain of each visit and shows the page that has it, at /.</li>
        </ol>
        <p className="mt-3 text-subtle">Click [edit] on a page and enter the domain. That is step 3. The page then shows the exact record and command for steps 1 and 2.</p>
        <p className="mt-3 text-dim">On its domain the page answers at /. The dashboard and the login don't.</p>
      </div>
    </SettingsLayout>
  )
}

// DomainSteps tells what makes a page's own domain answer: a DNS record, then the proxy.
function DomainSteps({ domain, host }: { domain: string; host?: string }) {
  return (
    <div className="mt-3 border-t border-dashed border-line pt-3">
      <p className="text-xs text-subtle">To show this page at {domain}:</p>
      <p className="mt-2 text-xs text-subtle">1. At your DNS provider, add this record. It sends visitors of {domain} to this server.</p>
      <table className="mt-2 w-full text-left text-xs">
        <thead className="text-dim">
          <tr>
            <th className="pr-4 font-normal">type</th>
            <th className="pr-4 font-normal">name</th>
            <th className="font-normal">target</th>
          </tr>
        </thead>
        <tbody className="text-foreground">
          <tr>
            <td className="pr-4">CNAME</td>
            <td className="break-all pr-4">{domain}</td>
            <td className="break-all">{host || 'the domain LogNorth runs on'}</td>
          </tr>
        </tbody>
      </table>
      <p className="mt-3 text-xs text-subtle">
        2. Then tell your proxy to send {domain} to LogNorth. On Chasen, run this in the folder of your LogNorth deploy. HTTPS comes with it.
      </p>
      <code className="mt-1 block break-all text-xs text-foreground">chasen domains add {domain}</code>
      <p className="mt-3 text-xs text-dim">3. Done: LogNorth sees {domain} on the visit and shows this page.</p>
    </div>
  )
}

function PageRow({ page, host }: { page: StatusPage; host?: string }) {
  const [editing, setEditing] = useState(false)
  const confirmDelete = (e: React.FormEvent) => {
    if (!confirm(`Delete the ${page.name} status page? Its apps stay, on no page.`)) e.preventDefault()
  }

  return (
    <div className="pane px-4 pb-4 pt-6 text-[13px]">
      <span className="pane-title font-bold text-foreground">{page.name}</span>
      <span className="pane-right">
        <button type="button" onClick={() => setEditing(!editing)} className="text-xs text-dim hover:text-foreground">
          [{editing ? 'close' : 'edit'}]
        </button>
      </span>

      <p className="flex flex-wrap gap-x-3 gap-y-1">
        <a href={`/status/${page.slug}`} className="text-foreground hover:underline">
          /status/{page.slug}
        </a>
        {page.domain && (
          <a href={`https://${page.domain}`} className="text-foreground hover:underline">
            {page.domain}
          </a>
        )}
      </p>
      <p className="mt-1 text-xs text-dim">{page.apps.length > 0 ? page.apps.join(' · ') : 'No apps yet. Pick them on the Apps page.'}</p>

      {page.domain && <DomainSteps domain={page.domain} host={host} />}

      {editing && (
        <div className="mt-3 border-t border-dashed border-line pt-3">
          <form action={`/settings/status/${page.id}`} method="POST" className="flex flex-col gap-2">
            <PageFields idPrefix={`page-${page.id}`} name={page.name} domain={page.domain} />
            <div className="flex items-center gap-3">
              <button type="submit" className="key">
                save
              </button>
            </div>
          </form>
          <form action={`/settings/status/${page.id}/delete`} method="POST" onSubmit={confirmDelete} className="mt-3">
            <button type="submit" className="text-xs text-dim hover:text-error">
              delete this page
            </button>
          </form>
        </div>
      )}
    </div>
  )
}

function NewPage({ first }: { first: boolean }) {
  const [open, setOpen] = useState(first)

  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)} className="key">
        + new status page
      </button>
    )
  }

  return (
    <form action="/settings/status" method="POST" className="pane flex flex-col gap-2 px-4 pb-4 pt-6 text-[13px]">
      <span className="pane-title">new status page</span>
      <PageFields idPrefix="new-page" name="" domain="" />
      <div className="flex items-center gap-3">
        <button type="submit" className="bg-foreground px-4 py-1.5 text-[13px] font-bold text-background hover:bg-text">
          Create
        </button>
        {!first && (
          <button type="button" onClick={() => setOpen(false)} className="text-xs text-dim hover:text-foreground">
            cancel
          </button>
        )}
      </div>
    </form>
  )
}

function PageFields({ idPrefix, name, domain }: { idPrefix: string; name: string; domain: string }) {
  return (
    <>
      <label htmlFor={`${idPrefix}-name`} className="text-xs text-subtle">
        name
      </label>
      <input id={`${idPrefix}-name`} type="text" name="name" defaultValue={name} placeholder="Fusionaly" maxLength={60} required className={FIELD} />
      <label htmlFor={`${idPrefix}-domain`} className="text-xs text-subtle">
        own domain <span className="text-dim">(optional)</span>
      </label>
      <input id={`${idPrefix}-domain`} type="text" name="domain" defaultValue={domain} placeholder="status.fusionaly.com" className={FIELD} />
    </>
  )
}
