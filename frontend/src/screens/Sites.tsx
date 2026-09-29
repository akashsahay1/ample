import {useEffect, useMemo, useRef, useState} from 'react'
import {ExternalLink, Folder, FolderPlus, Globe, MoreHorizontal, SquareTerminal, X} from 'lucide-react'
import {backend, PHPVersion, Site} from '../lib/api'
import {basename, copyText, frameworkLabel, shortPath, slug} from '../lib/format'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Button, Empty, IconButton, PageHeader, Toggle} from '../components/ui'
import ConflictBanner from '../components/ConflictBanner'

const ROW = 'grid grid-cols-[minmax(0,1.3fr)_minmax(0,1.6fr)_130px_110px_150px] items-center gap-4 px-5'

function MoreMenu({site, onUnlink}: {site: Site; onUnlink: () => void}) {
  const {toast} = useApp()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    const key = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    window.addEventListener('mousedown', close)
    window.addEventListener('keydown', key)
    return () => {
      window.removeEventListener('mousedown', close)
      window.removeEventListener('keydown', key)
    }
  }, [open])
  const item = 'block w-full border-0 bg-transparent px-3 py-2 text-left text-[13px] hover:bg-subtle focus:bg-subtle'
  return (
    <div className="relative" ref={ref}>
      <IconButton label="More actions" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(o => !o)}>
        <MoreHorizontal size={16} />
      </IconButton>
      {open && (
        <div role="menu" className="card absolute top-9 right-0 z-20 w-48 overflow-hidden py-1 shadow-lg">
          <button role="menuitem" type="button" className={item} onClick={() => (copyText(site.path), toast('success', 'Path copied'), setOpen(false))}>
            Copy path
          </button>
          <button role="menuitem" type="button" className={item} onClick={() => (copyText(site.url), toast('success', 'URL copied'), setOpen(false))}>
            Copy URL
          </button>
          {site.docRoot && site.docRoot !== site.path && (
            <button role="menuitem" type="button" className={item} onClick={() => (backend.OpenFolder(site.docRoot), setOpen(false))}>
              Open document root
            </button>
          )}
          {site.linked && (
            <button role="menuitem" type="button" className={`${item} text-danger`} onClick={() => (setOpen(false), onUnlink())}>
              Unlink site…
            </button>
          )}
        </div>
      )}
    </div>
  )
}

function SiteRow({site, php, defaultPhp, last}: {site: Site; php: PHPVersion[]; defaultPhp: string; last: boolean}) {
  const {run, confirm} = useApp()
  const installed = php.filter(p => p.installed)
  const eol = php.find(p => p.version === site.php)?.eol ?? false
  const value = site.isolated ? site.php : ''
  return (
    <div className={`${ROW} h-[60px] ${last ? '' : 'border-b border-line-soft'}`}>
      <div className="min-w-0">
        <div className="truncate font-semibold">{site.domain}</div>
        <div className="truncate text-xs text-muted">
          {site.linked ? 'Linked · ' : ''}
          {site.linked && site.framework === 'php' ? 'plain PHP' : frameworkLabel(site.framework)}
        </div>
      </div>
      <span className="selectable truncate font-mono text-xs text-body" title={site.path}>
        {shortPath(site.path)}
      </span>
      <select
        className={`sel-php ${eol ? 'eol' : ''}`}
        aria-label={`PHP version for ${site.domain}`}
        value={value}
        onChange={e => {
          const v = e.target.value
          run(() => backend.SetSitePHP(site.name, v), `${site.domain} now uses PHP ${v || defaultPhp}`)
        }}
      >
        <option value="">Default ({defaultPhp || '—'})</option>
        {installed.map(p => (
          <option key={p.version} value={p.version}>
            {p.version}
            {p.eol ? ' (EOL)' : ''}
          </option>
        ))}
        {site.isolated && !installed.some(p => p.version === site.php) && <option value={site.php}>{site.php} (missing)</option>}
      </select>
      <Toggle
        on={site.secure}
        label={`HTTPS for ${site.domain}`}
        onChange={v => run(() => backend.SetSiteSecure(site.name, v), v ? `${site.domain} is now served over HTTPS` : `HTTPS disabled for ${site.domain}`)}
      />
      <div className="flex justify-end gap-1.5">
        <IconButton label="Open in browser" onClick={() => backend.OpenURL(site.url)}>
          <ExternalLink size={16} />
        </IconButton>
        <IconButton label="Open folder" onClick={() => run(() => backend.OpenFolder(site.path))}>
          <Folder size={16} />
        </IconButton>
        <IconButton label="Open terminal" onClick={() => run(() => backend.OpenTerminal(site.path))}>
          <SquareTerminal size={16} />
        </IconButton>
        <MoreMenu
          site={site}
          onUnlink={async () => {
            const ok = await confirm({
              title: `Unlink ${site.domain}?`,
              body: (
                <>
                  AMPLS will stop serving <b>{site.domain}</b>. The folder <span className="font-mono">{site.path}</span> is not deleted.
                </>
              ),
              confirmLabel: 'Unlink',
              danger: true,
            })
            if (ok) await run(() => backend.Unlink(site.name), `${site.domain} unlinked`)
          }}
        />
      </div>
    </div>
  )
}

export default function Sites() {
  const {run, confirm, overview, setNewProjectOpen} = useApp()
  const [q, setQ] = useState('')
  const sites = useLoad(() => backend.ListSites())
  const php = useLoad(() => backend.ListPHP())
  const settings = useLoad(() => backend.GetSettings())
  const reloadAll = () => Promise.all([sites.reload(), settings.reload(), php.reload()])
  const tld = settings.data?.tld ?? 'test'
  const parked = settings.data?.parked ?? []
  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    const list = sites.data ?? []
    return s ? list.filter(x => x.domain.includes(s) || x.path.toLowerCase().includes(s) || x.framework.includes(s)) : list
  }, [q, sites.data])
  const example = sites.data?.find(s => s.linked) ?? sites.data?.[0]

  const park = async () => {
    const dir = await backend.SelectDirectory('Park a directory — every subfolder becomes a site')
    if (!dir) return
    await run(() => backend.Park(dir), `Parked ${dir}`)
    await reloadAll()
  }
  const link = async () => {
    const dir = await backend.SelectDirectory('Link a folder as a site')
    if (!dir) return
    const name = slug(basename(dir))
    await run(() => backend.Link(name, dir), `Linked ${name}.${tld}`)
    await reloadAll()
  }
  const unpark = async (dir: string) => {
    const ok = await confirm({
      title: 'Remove parked directory?',
      body: (
        <>
          Sites inside <span className="font-mono">{dir}</span> will no longer be served. No files are deleted.
        </>
      ),
      confirmLabel: 'Remove',
      danger: true,
    })
    if (!ok) return
    await run(() => backend.Unpark(dir), 'Directory removed')
    await reloadAll()
  }

  return (
    <>
      <ConflictBanner />
      <PageHeader
        title="Sites"
        subtitle={
          <>
            Every folder in a parked directory is served at <span className="font-mono">folder.{tld}</span>. Pick a PHP version per site.
          </>
        }
      >
        <Button onClick={link}>Link folder…</Button>
        <Button variant="primary" onClick={() => setNewProjectOpen(true)}>
          New project
        </Button>
      </PageHeader>

      <div className="flex flex-wrap items-center gap-2.5">
        <label htmlFor="site-search" className="sr-only">
          Search sites
        </label>
        <input id="site-search" className="input w-[280px]" placeholder="Search sites" value={q} onChange={e => setQ(e.target.value)} />
        <span className="ml-2 text-[13px] text-muted">Parked:</span>
        {parked.map(p => (
          <span key={p} className="chip" title={p}>
            {shortPath(p)}
            <button type="button" aria-label={`Remove parked directory ${p}`} className="inline-flex h-5 w-5 items-center justify-center rounded-full border-0 bg-transparent text-muted hover:bg-subtle hover:text-ink" onClick={() => unpark(p)}>
              <X size={12} />
            </button>
          </span>
        ))}
        <Button size="sm" onClick={park}>
          + Park directory
        </Button>
      </div>

      <div className="card overflow-hidden">
        <div className={`${ROW} label-caps h-10 bg-subtle`}>
          <span>Site</span>
          <span>Path</span>
          <span>PHP</span>
          <span>HTTPS</span>
          <span className="text-right">Actions</span>
        </div>
        {sites.error && <div className="px-5 py-4 text-[13px] text-eol">{sites.error}</div>}
        {!sites.loading && (sites.data?.length ?? 0) === 0 && !sites.error && (
          <Empty icon={<Globe size={28} />} title="No sites yet">
            Park a directory and every folder inside it is served automatically at <span className="font-mono">folder.{tld}</span>.
            {parked[0] && (
              <>
                {' '}
                Your Sites folder is <span className="font-mono">{parked[0]}</span>.
              </>
            )}
            <div className="mt-4 flex justify-center gap-2">
              {parked[0] && (
                <Button icon={<FolderPlus size={15} />} onClick={() => run(() => backend.OpenFolder(parked[0]))}>
                  Open Sites folder
                </Button>
              )}
              <Button variant="primary" onClick={() => setNewProjectOpen(true)}>
                New project
              </Button>
            </div>
          </Empty>
        )}
        {(sites.data?.length ?? 0) > 0 && filtered.length === 0 && <div className="px-5 py-6 text-[13px] text-muted">No sites match “{q}”.</div>}
        {filtered.map((s, i) => (
          <SiteRow key={s.name} site={s} php={php.data ?? []} defaultPhp={overview?.defaultPhp ?? ''} last={i === filtered.length - 1} />
        ))}
      </div>

      {example && (
        <div className="flex items-center gap-3 rounded-xl bg-shell px-[18px] py-3.5 text-[#E6E7EA]">
          <SquareTerminal size={18} color="#9AA3FF" aria-hidden />
          <span className="text-[13px]">Same thing from a terminal:</span>
          <span className="selectable truncate font-mono text-xs text-[#9AA3FF]">cd {example.path}</span>
          <span className="font-mono text-xs text-warn">ampls isolate 8.3</span>
          <span className="font-mono text-xs text-[#3FA9C9]">ampls secure</span>
          <span className="grow" />
          <span className="text-xs whitespace-nowrap text-shell-muted">
            <span className="font-mono">php</span> in that folder now runs 8.3
          </span>
        </div>
      )}
    </>
  )
}
