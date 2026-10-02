import {useState} from 'react'
import {AlertTriangle} from 'lucide-react'
import {backend, ENV_NAMES, PortConflict, STOPPABLE_KINDS} from '../lib/api'
import {useApp} from '../state/AppState'
import {Button} from './ui'

const SERVICE_LABEL: Record<string, string> = {apache: 'Apache', mysql: 'MySQL'}

function portList(ports: number[]): string {
  if (ports.length === 1) return `Port ${ports[0]} is`
  return `Ports ${ports.slice(0, -1).join(', ')} and ${ports[ports.length - 1]} are`
}

function where(root?: string) {
  return root ? (
    <>
      {' '}
      (processes running from <span className="font-mono break-all">{root}</span>)
    </>
  ) : null
}

/** What stopping each environment does, for the confirm dialog. */
function stopBody(kind: string, root?: string) {
  const name = ENV_NAMES[kind] ?? kind
  const what =
    kind === 'xampp' ? (
      <>XAMPP's own Apache and MySQL servers</>
    ) : kind === 'herd' ? (
      <>Laravel Herd's own nginx, PHP and service processes</>
    ) : (
      <>{name}'s own web and database servers</>
    )
  const again = kind === 'xampp' ? 'the XAMPP Control Panel' : kind === 'herd' ? 'the Herd app' : name
  return (
    <>
      Apnoro will stop {what}
      {where(root)}. No other programs are stopped, and {name}'s files, settings, sites and databases are not changed.
      <br />
      <br />
      You can start it again any time from {again}.
    </>
  )
}

/** Confirm, then stop another environment's servers; optionally start Apnoro afterwards. */
export function useStopEnvironment() {
  const {confirm, run, toast} = useApp()
  return async (kind: string, opts: {root?: string; startAfter?: boolean} = {}) => {
    const name = ENV_NAMES[kind] ?? kind
    const ok = await confirm({
      title: `Stop ${name}?`,
      body: stopBody(kind, opts.root),
      confirmLabel: `Stop ${name}`,
    })
    if (!ok) return false
    const done = await run(async () => {
      await backend.StopEnvironment(kind)
      return true
    }, `${name} stopped`)
    if (done && opts.startAfter) {
      try {
        await backend.StartAll()
        toast('success', 'Apnoro services started')
      } catch (e) {
        toast('error', `${name} stopped, but Apnoro could not start: ${e instanceof Error ? e.message : String(e)}`)
      }
    }
    return !!done
  }
}

function ServiceConflict({service, items}: {service: string; items: PortConflict[]}) {
  const {run, conflicts, toast} = useApp()
  const stopEnv = useStopEnvironment()
  const [busy, setBusy] = useState(false)
  const first = items[0]
  const env = first.env
  const owner = env ? `${ENV_NAMES[env] ?? env} ${SERVICE_LABEL[service] ?? service}` : first.process || 'another program'
  const label = SERVICE_LABEL[service] ?? service
  const stoppable = !!env && STOPPABLE_KINDS.includes(env)
  const ports = items.map(c => c.port)

  const switchPorts = async () => {
    setBusy(true)
    try {
      const msg = await run(async () => {
        const s = await backend.GetSettings()
        if (service === 'mysql') {
          s.mysqlPort = s.mysqlPort === 3307 ? 3308 : 3307
        } else {
          s.httpPort = s.httpPort === 8080 ? 8081 : 8080
          s.httpsPort = s.httpsPort === 8443 ? 8444 : 8443
        }
        await backend.SaveSettings(s)
        // if the other service is still blocked RestartAll would fail on it; restart only this one
        const otherBlocked = conflicts.some(c => c.service !== service)
        if (otherBlocked) await backend.RestartService(service)
        else await backend.RestartAll()
        return service === 'mysql' ? `MySQL now uses port ${s.mysqlPort}` : `Apache now uses ports ${s.httpPort} / ${s.httpsPort}`
      })
      if (msg) toast('success', msg)
    } finally {
      setBusy(false)
    }
  }

  // derive the environment root from the exe path (e.g. C:\xampp\apache\bin\httpd.exe -> C:\xampp)
  const root = env === 'xampp' ? first.path.replace(/\\(apache|mysql)\\bin\\[^\\]+$/i, '') : undefined

  return (
    <div role="alert" className="flex items-center gap-3 rounded-xl border border-[#F0D48A] bg-[#FFF6E0] px-4 py-3">
      <AlertTriangle size={18} color="#8A5A00" aria-hidden className="shrink-0" />
      <div className="flex min-w-0 grow flex-col gap-0.5 text-[13px] text-[#5A3B00]">
        <div>
          <strong>
            {portList(ports)} used by {owner}.
          </strong>{' '}
          Apnoro {label} can't start until {ports.length > 1 ? "they're" : "it's"} free.
        </div>
        {first.path && (
          <div className="selectable truncate font-mono text-xs text-[#7A5410]" title={first.path}>
            {first.path}
          </div>
        )}
      </div>
      <Button size="md" pending={busy} onClick={switchPorts}>
        {service === 'mysql' ? 'Use port 3307' : 'Use 8080 / 8443'}
      </Button>
      {stoppable && (
        <Button size="md" variant="primary" onClick={() => stopEnv(env, {root, startAfter: true})}>
          Stop {ENV_NAMES[env] ?? env}
        </Button>
      )}
    </div>
  )
}

/** Shown at the top of Dashboard, Sites and Import while another program holds an Apnoro port. */
export default function ConflictBanner() {
  const {conflicts} = useApp()
  if (!conflicts.length) return null
  const byService = new Map<string, PortConflict[]>()
  for (const c of conflicts) byService.set(c.service, [...(byService.get(c.service) ?? []), c])
  return (
    <div className="flex flex-col gap-2">
      {[...byService.entries()].map(([svc, items]) => (
        <ServiceConflict key={svc} service={svc} items={items} />
      ))}
    </div>
  )
}

/** Sidebar summary: null when nothing is blocked. */
export function blockedSummary(conflicts: PortConflict[], stoppedServices: string[]): {label: string; sub: string} | null {
  const blocked = conflicts.filter(c => stoppedServices.includes(c.service))
  if (!blocked.length) return null
  const services = [...new Set(blocked.map(c => SERVICE_LABEL[c.service] ?? c.service))]
  const first = blocked[0]
  const owner = first.env ? (ENV_NAMES[first.env] ?? first.env) : first.process || 'another program'
  const ports = [...new Set(blocked.filter(c => c.service === first.service).map(c => c.port))]
  return {
    label: `${services.join(' & ')} blocked`,
    sub: `Port ${ports[0]} in use by ${owner}`,
  }
}
