import {ReactNode} from 'react'
import {FolderOpen, Lock, LockOpen, Plus, ShieldCheck, SquareTerminal} from 'lucide-react'
import {backend, ServiceStatus, Site} from '../lib/api'
import {phpMyAdminURL, shortPath} from '../lib/format'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Badge, Button, Empty, PageHeader, ServiceIcon} from '../components/ui'

function ServiceCard({
  kind,
  name,
  sub,
  badge,
  meta,
  actions,
}: {
  kind: 'apache' | 'mysql' | 'php'
  name: string
  sub: string
  badge: ReactNode
  meta: ReactNode
  actions: ReactNode
}) {
  return (
    <div className="card flex flex-col gap-3.5 p-5">
      <div className="flex items-center gap-2.5">
        <ServiceIcon kind={kind} />
        <div className="min-w-0 grow">
          <div className="text-[15px] font-semibold">{name}</div>
          <div className="truncate font-mono text-xs text-muted">{sub}</div>
        </div>
        {badge}
      </div>
      <div className="flex min-h-4 gap-4 font-mono text-xs text-body">{meta}</div>
      <div className="flex flex-wrap gap-2">{actions}</div>
    </div>
  )
}

function StatusBadge({s}: {s?: ServiceStatus}) {
  if (!s) return <Badge tone="off">Unknown</Badge>
  if (s.error) return <Badge tone="eol">Error</Badge>
  return s.running ? <Badge tone="ok">Running</Badge> : <Badge tone="off">Stopped</Badge>
}

export function ServiceToggle({s}: {s?: ServiceStatus}) {
  const {run} = useApp()
  if (!s) return null
  return s.running ? (
    <Button onClick={() => run(() => backend.StopService(s.name))}>Stop</Button>
  ) : (
    <Button onClick={() => run(() => backend.StartService(s.name))}>Start</Button>
  )
}

export function SiteLine({site, last}: {site: Site; last: boolean}) {
  const eol = site.php.startsWith('7.') || site.php === '8.0' || site.php === '8.1'
  return (
    <div className={`flex items-center gap-3 py-3 ${last ? '' : 'border-b border-line-soft'}`}>
      {site.secure ? <Lock size={16} color="#1C6E45" aria-label="HTTPS" /> : <LockOpen size={16} color="#8A8E97" aria-label="HTTP only" />}
      <button type="button" className="grow truncate border-0 bg-transparent p-0 text-left font-medium text-ink hover:text-link" onClick={() => backend.OpenURL(site.url)}>
        {site.domain}
      </button>
      <span className={`rounded-md px-2 py-0.5 font-mono text-xs ${eol ? 'bg-eol-bg text-eol' : 'bg-php-bg text-php'}`}>PHP {site.php}</span>
      <span className="w-[260px] truncate font-mono text-xs text-muted" title={site.path}>
        {shortPath(site.path)}
      </span>
    </div>
  )
}

export default function Dashboard() {
  const {overview, run, go, setNewProjectOpen} = useApp()
  const sites = useLoad(() => backend.ListSites())
  const settings = useLoad(() => backend.GetSettings())
  const svc = (n: string) => overview?.services.find(s => s.name === n)
  const apache = svc('apache')
  const mysql = svc('mysql')
  const anyRunning = !!overview?.services.some(s => s.running)
  const sitesDir = settings.data?.parked?.[0] ?? ''
  const php = overview?.phpVersions ?? []
  const list = sites.data ?? []

  return (
    <>
      <PageHeader title="Dashboard" subtitle="Your local stack at a glance.">
        {anyRunning ? (
          <Button onClick={() => run(() => backend.StopAll(), 'All services stopped')}>Stop all</Button>
        ) : (
          <Button onClick={() => run(() => backend.StartAll(), 'All services started')}>Start all</Button>
        )}
        <Button variant="primary" onClick={() => run(() => backend.RestartAll(), 'Services restarted')}>
          Restart all
        </Button>
      </PageHeader>

      <div className="grid grid-cols-3 gap-4">
        <ServiceCard
          kind="apache"
          name="Apache"
          sub={`httpd ${apache?.version || '2.4'} · mod_fcgid`}
          badge={<StatusBadge s={apache} />}
          meta={
            apache?.error ? (
              <span className="truncate text-eol" title={apache.error}>{apache.error}</span>
            ) : (
              <>
                {(apache?.ports ?? []).map(p => <span key={p}>:{p}</span>)}
                {apache?.running && <span>PID {apache.pid}</span>}
              </>
            )
          }
          actions={
            <>
              <Button onClick={() => run(() => backend.RestartService('apache'), 'Apache restarted')}>Restart</Button>
              <ServiceToggle s={apache} />
              <Button onClick={() => overview && run(() => backend.OpenFolder(overview.home + '\\conf'))}>Config</Button>
            </>
          }
        />
        <ServiceCard
          kind="mysql"
          name="MySQL"
          sub={mysql?.version ? `${mysql.version.split('.').slice(0, 2).join('.')} LTS` : '8.4 LTS'}
          badge={<StatusBadge s={mysql} />}
          meta={
            mysql?.error ? (
              <span className="truncate text-eol" title={mysql.error}>{mysql.error}</span>
            ) : (
              <>
                {(mysql?.ports ?? []).map(p => <span key={p}>:{p}</span>)}
                <span>root</span>
                {mysql?.running && <span>PID {mysql.pid}</span>}
              </>
            )
          }
          actions={
            <>
              <Button onClick={() => run(() => backend.RestartService('mysql'), 'MySQL restarted')}>Restart</Button>
              <ServiceToggle s={mysql} />
              <Button onClick={() => backend.OpenURL(phpMyAdminURL(apache?.ports?.[0]))}>phpMyAdmin</Button>
            </>
          }
        />
        <ServiceCard
          kind="php"
          name="PHP"
          sub={overview?.defaultPhp ? `default ${overview.defaultPhp} · FastCGI` : 'no default · FastCGI'}
          badge={<Badge tone="php">{php.length} installed</Badge>}
          meta={php.length ? php.map(v => <span key={v}>{v}</span>) : <span className="text-muted">No PHP installed yet</span>}
          actions={
            <>
              <Button onClick={() => go('php')}>Manage versions</Button>
              <Button
                disabled={!overview?.defaultPhp}
                onClick={async () => {
                  if (!overview?.defaultPhp) return
                  const p = await backend.PHPIniPath(overview.defaultPhp)
                  await run(() => backend.OpenPath(p))
                }}
              >
                php.ini
              </Button>
            </>
          }
        />
      </div>

      <div className="grid min-h-0 grow grid-cols-3 gap-4">
        <section className="card col-span-2 flex flex-col gap-3 p-5">
          <div className="flex items-center">
            <h2 className="m-0 grow text-[15px] font-semibold">Sites</h2>
            {list.length > 0 && (
              <button type="button" className="border-0 bg-transparent p-0 text-[13px] font-medium text-link hover:text-eol" onClick={() => go('sites')}>
                View all {list.length}
              </button>
            )}
          </div>
          {list.length === 0 && !sites.loading ? (
            <Empty title="No sites yet">
              Every folder inside a parked directory is served at <span className="font-mono">folder.{settings.data?.tld ?? 'test'}</span>.
              {sitesDir && (
                <>
                  {' '}
                  Drop a project into <span className="font-mono">{shortPath(sitesDir)}</span> or create a new project.
                </>
              )}
            </Empty>
          ) : (
            <div className="flex flex-col">
              {list.slice(0, 6).map((s, i, a) => (
                <SiteLine key={s.name} site={s} last={i === a.length - 1} />
              ))}
            </div>
          )}
        </section>
        <section className="card flex flex-col gap-2.5 self-start p-5">
          <h2 className="m-0 text-[15px] font-semibold">Quick actions</h2>
          <Button className="h-11! justify-start" icon={<FolderOpen size={16} />} disabled={!sitesDir} onClick={() => run(() => backend.OpenFolder(sitesDir))}>
            Open Sites folder
          </Button>
          <Button className="h-11! justify-start" icon={<Plus size={16} />} onClick={() => setNewProjectOpen(true)}>
            New project
          </Button>
          <Button className="h-11! justify-start" icon={<SquareTerminal size={16} />} disabled={!sitesDir} onClick={() => run(() => backend.OpenTerminal(sitesDir))}>
            Open terminal here
          </Button>
          <Button
            className="h-11! justify-start"
            icon={<ShieldCheck size={16} />}
            disabled={overview?.caTrusted}
            onClick={() => run(() => backend.TrustCA(), 'Local HTTPS certificate trusted')}
          >
            {overview?.caTrusted ? 'HTTPS certificate trusted' : 'Trust local HTTPS certificate'}
          </Button>
        </section>
      </div>
    </>
  )
}
