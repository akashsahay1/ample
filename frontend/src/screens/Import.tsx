import {useEffect, useMemo, useState} from 'react'
import {AlertCircle, CheckCircle2, Info} from 'lucide-react'
import {backend, ENV_NAMES, errMsg, ExternalEnv, IMPORT_TASK_PREFIX, ImportPlan, ImportSite, MySQLSource, onProgress} from '../lib/api'
import {bytes, shortPath} from '../lib/format'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Badge, Button, Field, Modal, PageHeader, ProgressBar, Toggle} from '../components/ui'
import ConflictBanner, {useStopEnvironment} from '../components/ConflictBanner'

const ROW = 'grid grid-cols-[28px_minmax(0,1.1fr)_minmax(0,1.7fr)_110px_70px_80px] items-center gap-3 px-4'
const ALL_KINDS = ['xampp', 'herd', 'laragon', 'wamp']
const KIND_TITLES: Record<string, string> = {xampp: 'XAMPP', herd: 'Laravel Herd', laragon: 'Laragon', wamp: 'WAMP'}

type Phase = 'idle' | 'scanning' | 'preview' | 'running' | 'done' | 'error'

interface Selection {
  park: Set<string>
  sites: Set<string>
  dbs: Set<string>
  installPhp: boolean
  keepSecure: boolean
  overwrite: boolean
}

function envSummary(e: ExternalEnv): string {
  const parts: string[] = []
  parts.push(`${e.sites} site${e.sites === 1 ? '' : 's'}`)
  if (e.databases) parts.push('databases')
  if (e.php) parts.push(`PHP ${e.php}`)
  if (e.onPath) parts.push('php first on PATH')
  return parts.join(' · ')
}

function EnvCard({env, selected, onImport, onStop}: {env: ExternalEnv; selected: boolean; onImport: () => void; onStop: () => unknown}) {
  const ports = (env.ports ?? []).map(p => `:${p}`).join(' ')
  return (
    <div className={`card flex min-w-0 flex-col gap-2.5 p-4 ${selected ? 'border-brand! shadow-[0_0_0_3px_#FBE6DC]' : ''}`}>
      <div className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 grow truncate font-semibold" title={env.name}>
          {env.name}
        </span>
        {env.running ? <span className="tag bg-ok-bg text-ok">Running</span> : <span className="tag bg-[#EFEDE8] text-muted">Stopped</span>}
      </div>
      <div className="truncate font-mono text-xs text-muted" title={env.path}>
        {shortPath(env.path)}
        {ports && ` · ${ports}`}
      </div>
      <div className="truncate text-xs text-body" title={(env.notes ?? []).join('\n') || undefined}>
        {envSummary(env)}
      </div>
      <div className="flex gap-1.5">
        {env.canImport && (
          <Button size="sm" variant={selected ? 'primary' : 'default'} onClick={onImport}>
            Import…
          </Button>
        )}
        {env.canStop && (
          <Button size="sm" onClick={onStop}>
            Stop
          </Button>
        )}
      </div>
    </div>
  )
}

function MySQLDialog({initial, onClose, onConnected}: {initial: MySQLSource; onClose: () => void; onConnected: (src: MySQLSource, plan: ImportPlan) => void}) {
  const [src, setSrc] = useState<MySQLSource>(initial)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const connect = async () => {
    setBusy(true)
    setErr(null)
    try {
      const plan = await backend.ScanImport('mysql', src)
      onConnected(src, plan)
    } catch (e) {
      setErr(errMsg(e))
    } finally {
      setBusy(false)
    }
  }
  const valid = src.host.trim() !== '' && src.port > 0 && src.port < 65536 && src.user.trim() !== ''
  return (
    <Modal
      title="Connect to a MySQL server"
      onClose={onClose}
      width={460}
      dismissable={!busy}
      footer={
        <>
          <Button onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" pending={busy} disabled={!valid} onClick={connect}>
            Connect
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={e => {
          e.preventDefault()
          if (valid && !busy) connect()
        }}
      >
        <p className="m-0 text-[13px] text-muted">Databases are copied from any MySQL or MariaDB server. The source server is not changed.</p>
        <div className="grid grid-cols-[minmax(0,1fr)_110px] gap-3">
          <Field label="Host">
            <input className="input mono" value={src.host} onChange={e => setSrc({...src, host: e.target.value})} autoFocus />
          </Field>
          <Field label="Port">
            <input className="input mono" inputMode="numeric" value={src.port || ''} onChange={e => setSrc({...src, port: parseInt(e.target.value.replace(/\D/g, ''), 10) || 0})} />
          </Field>
        </div>
        <Field label="User">
          <input className="input mono" value={src.user} onChange={e => setSrc({...src, user: e.target.value})} autoComplete="off" />
        </Field>
        <Field label="Password">
          <input className="input mono" type="password" value={src.password} onChange={e => setSrc({...src, password: e.target.value})} autoComplete="off" />
        </Field>
        {err && (
          <div role="alert" className="flex items-start gap-2 rounded-lg bg-eol-bg px-3 py-2 text-[13px] text-eol">
            <AlertCircle size={15} className="mt-0.5 shrink-0" />
            <span className="selectable min-w-0 break-words">{err}</span>
          </div>
        )}
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}

function initialSelection(plan: ImportPlan): Selection {
  const sites = new Set<string>()
  for (const s of plan.sites ?? []) if (!s.conflict) sites.add(s.path)
  const dbs = new Set<string>()
  for (const d of plan.databases ?? []) if (!d.exists) dbs.add(d.name)
  return {
    park: new Set(plan.parkedDirs ?? []),
    sites,
    dbs,
    installPhp: (plan.missingPhp ?? []).length > 0,
    keepSecure: true,
    overwrite: false,
  }
}

const dirOf = (p: string) => p.replace(/[\\/][^\\/]+[\\/]?$/, '').toLowerCase()

function plural(n: number, one: string, many = one + 's') {
  return `${n} ${n === 1 ? one : many}`
}

function SiteRow({s, sel, parkedBy, missing, onToggle}: {s: ImportSite; sel: Selection; parkedBy?: string; missing: boolean; onToggle: () => void}) {
  if (s.conflict) {
    return (
      <div className={`${ROW} h-[46px] border-b border-line-soft bg-[#FFFBF2] text-[13px]`}>
        <input type="checkbox" className="cb" checked={false} disabled aria-label={`Import ${s.domain} (not possible: ${s.conflict})`} />
        <span className="truncate font-semibold" title={s.domain}>
          {s.domain}
        </span>
        <span className="col-span-3 truncate text-xs text-[#8A5A00]" title={s.conflict}>
          {s.conflict.charAt(0).toUpperCase() + s.conflict.slice(1)}. Rename or skip.
        </span>
        <span className="src-tag">{s.source}</span>
      </div>
    )
  }
  const checked = !!parkedBy || sel.sites.has(s.path)
  return (
    <div className={`${ROW} h-[46px] border-b border-line-soft text-[13px] ${checked ? '' : 'text-muted'}`}>
      <input
        type="checkbox"
        className="cb"
        checked={checked}
        disabled={!!parkedBy}
        title={parkedBy ? `Included because ${shortPath(parkedBy)} is parked` : undefined}
        aria-label={`Import ${s.domain}`}
        onChange={onToggle}
      />
      <span className="truncate font-semibold" title={s.domain}>
        {s.domain}
      </span>
      <span className="selectable truncate font-mono text-xs text-body" title={s.docRoot && s.docRoot !== s.path ? `${s.path}\nserves ${s.docRoot}` : s.path}>
        {shortPath(s.path)}
      </span>
      {s.php ? (
        missing ? (
          sel.installPhp ? (
            <span className="pin missing" title={`PHP ${s.php} will be installed`}>
              {s.php} · install
            </span>
          ) : (
            <span className="pin missing" title={`PHP ${s.php} is not installed; the site will use the default PHP`}>
              {s.php} · default
            </span>
          )
        ) : (
          <span className="pin">{s.php}</span>
        )
      ) : (
        <span className="text-xs text-muted">default</span>
      )}
      {s.secure && sel.keepSecure ? <span className="text-ok">On</span> : <span className="text-muted">Off</span>}
      <span className="src-tag">{s.source}</span>
    </div>
  )
}

export default function Import() {
  const {toast, go, overview} = useApp()
  const envs = useLoad(() => backend.DetectEnvironments())
  const stopEnv = useStopEnvironment()
  const [kind, setKind] = useState<string | null>(null)
  const [plan, setPlan] = useState<ImportPlan | null>(null)
  const [sel, setSel] = useState<Selection | null>(null)
  const [phase, setPhase] = useState<Phase>('idle')
  const [mysqlSrc, setMysqlSrc] = useState<MySQLSource | null>(null)
  const [mysqlOpen, setMysqlOpen] = useState(false)
  const [progress, setProgress] = useState<{message: string; percent: number}>({message: '', percent: 0})
  // the backend's final progress message may carry notes ("Import finished: skipped x; ...")
  const [doneMessage, setDoneMessage] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<{sites: number; dbs: number} | null>(null)

  const list = envs.data ?? []
  const missingKinds = ALL_KINDS.filter(k => !list.some(e => e.kind === k))
  const envName = (k: string | null) => (k === 'mysql' ? plan?.source || 'MySQL server' : list.find(e => e.kind === k)?.name ?? (k ? KIND_TITLES[k] ?? k : ''))
  const shortName = (k: string | null) => (k === 'mysql' ? 'The source server' : k ? ENV_NAMES[k] ?? k : '')

  useEffect(() => {
    if (phase !== 'running' || !kind) return
    return onProgress(p => {
      if (p.task !== IMPORT_TASK_PREFIX + kind) return
      if (p.done) setDoneMessage(p.error ? '' : p.message)
      else setProgress({message: p.message, percent: p.percent})
    })
  }, [phase, kind])

  const scan = async (k: string, src: MySQLSource | null = null) => {
    setKind(k)
    setPlan(null)
    setError(null)
    setResult(null)
    setPhase('scanning')
    try {
      const p = await backend.ScanImport(k, src)
      setPlan(p)
      setSel(initialSelection(p))
      setPhase('preview')
    } catch (e) {
      const m = errMsg(e)
      setError(m)
      setPhase('error')
      toast('error', m)
    }
  }

  const reset = () => {
    setKind(null)
    setPlan(null)
    setSel(null)
    setError(null)
    setResult(null)
    setPhase('idle')
  }

  // which parked dir (if any) includes each site
  const parkedBy = useMemo(() => {
    const m = new Map<string, string>()
    if (!plan || !sel) return m
    for (const s of plan.sites ?? []) {
      const d = [...sel.park].find(p => p.toLowerCase().replace(/[\\/]+$/, '') === dirOf(s.path))
      if (d) m.set(s.path, d)
    }
    return m
  }, [plan, sel])

  const counts = useMemo(() => {
    if (!plan || !sel) return {sites: 0, dbs: 0}
    const sites = (plan.sites ?? []).filter(s => !s.conflict && (parkedBy.has(s.path) || sel.sites.has(s.path))).length
    const dbs = (plan.databases ?? []).filter(d => sel.dbs.has(d.name) && (!d.exists || sel.overwrite)).length
    return {sites, dbs}
  }, [plan, sel, parkedBy])

  const importLabel =
    counts.sites && counts.dbs
      ? `Import ${plural(counts.sites, 'site')} & ${plural(counts.dbs, 'database')}`
      : counts.dbs
        ? `Import ${plural(counts.dbs, 'database')}`
        : `Import ${plural(counts.sites, 'site')}`

  const start = async () => {
    if (!plan || !sel || !kind) return
    const req = {
      kind,
      parkDirs: [...sel.park],
      sites: (plan.sites ?? []).filter(s => !s.conflict && !parkedBy.has(s.path) && sel.sites.has(s.path)).map(s => s.path),
      databases: (plan.databases ?? []).filter(d => sel.dbs.has(d.name) && (!d.exists || sel.overwrite)).map(d => d.name),
      overwrite: sel.overwrite,
      installPhp: sel.installPhp && (plan.missingPhp ?? []).length > 0,
      keepSecure: sel.keepSecure,
      mysql: kind === 'mysql' ? mysqlSrc : null,
    }
    setProgress({message: 'Starting import…', percent: 0})
    setDoneMessage('')
    setError(null)
    setPhase('running')
    try {
      await backend.RunImport(req)
      setResult({...counts})
      setPhase('done')
      toast('success', `Imported from ${envName(kind)}`)
    } catch (e) {
      const m = errMsg(e)
      setError(m)
      setPhase('error')
      toast('error', m)
    }
  }

  const update = (f: (s: Selection) => Selection) => setSel(s => (s ? f(s) : s))
  const toggleIn = (set: Set<string>, v: string) => {
    const n = new Set(set)
    if (n.has(v)) n.delete(v)
    else n.add(v)
    return n
  }

  const missingPhp = plan?.missingPhp ?? []
  const hasSecure = (plan?.sites ?? []).some(s => s.secure)
  const busy = phase === 'running' || phase === 'scanning'

  return (
    <>
      <ConflictBanner />
      <PageHeader title="Import" subtitle="Bring projects and databases from other local stacks. Project folders are served in place, never copied or changed.">
        <Button onClick={() => envs.reload()} disabled={busy}>
          Rescan
        </Button>
      </PageHeader>

      {envs.error && <div className="text-[13px] text-eol">{envs.error}</div>}

      <div className="grid grid-cols-4 gap-3">
        {list.map(e => (
          <EnvCard
            key={e.kind + e.path}
            env={e}
            selected={kind === e.kind}
            onImport={() => !busy && scan(e.kind)}
            onStop={() => stopEnv(e.kind, {root: e.path, startAfter: !!overview?.services.some(s => !s.running)})}
          />
        ))}
        {!envs.loading && missingKinds.length > 0 && (
          <div className="card flex min-w-0 flex-col gap-2.5 p-4">
            <div className="flex min-w-0 items-center gap-2">
              <span className="min-w-0 grow truncate font-semibold">{KIND_TITLES[missingKinds[0]]}</span>
              <span className="tag bg-[#EFEDE8] text-muted">Not found</span>
            </div>
            <div className="text-xs text-muted">
              {missingKinds.length > 1 ? `Also detects ${missingKinds.slice(1).map(k => KIND_TITLES[k]).join(', ')}` : 'Checked on every rescan'}
            </div>
          </div>
        )}
        <div className={`card flex min-w-0 flex-col gap-2.5 p-4 ${kind === 'mysql' ? 'border-brand! shadow-[0_0_0_3px_#FBE6DC]' : ''}`}>
          <div className="font-semibold">Other MySQL server</div>
          <div className="text-xs text-muted">Copy databases from any MySQL or MariaDB by host and port.</div>
          <div className="flex gap-1.5">
            <Button size="sm" disabled={busy} onClick={() => setMysqlOpen(true)}>
              Connect…
            </Button>
          </div>
        </div>
      </div>

      {phase === 'idle' && !envs.loading && list.length === 0 && (
        <div className="text-[13px] text-muted">No other local stacks were found. You can still copy databases from any MySQL server.</div>
      )}

      {phase === 'scanning' && (
        <section className="card flex flex-col gap-3 p-5">
          <div className="text-[13px] text-muted">Reading {envName(kind)}…</div>
          <ProgressBar percent={-1} />
        </section>
      )}

      {phase === 'error' && (
        <section className="card flex items-start gap-3 border-[#F0C6B2]! p-5">
          <AlertCircle size={18} className="mt-0.5 shrink-0 text-eol" />
          <div className="flex min-w-0 grow flex-col gap-1">
            <div className="font-semibold">{plan ? `Import from ${envName(kind)} failed` : `Could not read ${envName(kind)}`}</div>
            <div className="selectable text-[13px] break-words text-body">{error}</div>
          </div>
          <Button onClick={reset}>Close</Button>
          {plan ? (
            <Button variant="primary" onClick={() => setPhase('preview')}>
              Back to preview
            </Button>
          ) : (
            kind && (
              <Button variant="primary" onClick={() => (kind === 'mysql' ? setMysqlOpen(true) : scan(kind))}>
                Try again
              </Button>
            )
          )}
        </section>
      )}

      {phase === 'running' && (
        <section className="card flex flex-col gap-3 p-5" aria-live="polite">
          <div className="flex items-center gap-3">
            <h2 className="m-0 grow text-[15px] font-semibold">Importing from {envName(kind)}…</h2>
            {progress.percent >= 0 && <span className="font-mono text-xs text-muted">{Math.round(progress.percent)}%</span>}
          </div>
          <ProgressBar percent={progress.percent} color="#C2461A" />
          <div className="truncate text-[13px] text-muted">{progress.message}</div>
        </section>
      )}

      {phase === 'done' && result && (
        <section className="card flex items-center gap-3 p-5">
          <CheckCircle2 size={20} className="shrink-0 text-ok" />
          <div className="flex min-w-0 grow flex-col gap-0.5">
            <div className="font-semibold">Import complete</div>
            <div className="text-[13px] text-muted">
              {[result.sites ? plural(result.sites, 'site') : '', result.dbs ? plural(result.dbs, 'database') : ''].filter(Boolean).join(' and ')} imported from {envName(kind)}.{' '}
              {shortName(kind)} itself was not changed.
            </div>
            {doneMessage.includes(': ') && (
              <ul className="m-0 mt-1 flex list-none flex-col gap-0.5 p-0">
                {doneMessage
                  .slice(doneMessage.indexOf(': ') + 2)
                  .split('; ')
                  .map((n, i) => (
                    <li key={i} className="flex items-start gap-2 text-xs text-muted">
                      <Info size={13} className="mt-0.5 shrink-0" />
                      <span className="selectable min-w-0 break-words">{n}</span>
                    </li>
                  ))}
              </ul>
            )}
          </div>
          <Button onClick={reset}>Done</Button>
          {result.sites > 0 ? (
            <Button variant="primary" onClick={() => go('sites')}>
              Open Sites
            </Button>
          ) : (
            <Button variant="primary" onClick={() => go('mysql')}>
              Open MySQL
            </Button>
          )}
        </section>
      )}

      {phase === 'preview' && plan && sel && (
        <section className="card flex min-h-0 flex-col overflow-hidden">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-line-soft px-4 py-3.5">
            <h2 className="m-0 min-w-0 grow truncate text-[15px] font-semibold">Import from {envName(kind)}: preview</h2>
            {(plan.parkedDirs ?? []).map(d => (
              <label key={d} className="flex items-center gap-1.5 text-[13px] whitespace-nowrap" title={d}>
                <input type="checkbox" className="cb" checked={sel.park.has(d)} onChange={() => update(s => ({...s, park: toggleIn(s.park, d)}))} />
                Park {shortPath(d)} (serves every folder)
              </label>
            ))}
            {missingPhp.length > 0 && (
              <label className="flex items-center gap-1.5 text-[13px] whitespace-nowrap">
                <input type="checkbox" className="cb" checked={sel.installPhp} onChange={() => update(s => ({...s, installPhp: !s.installPhp}))} />
                Install missing PHP {missingPhp.join(', ')}
              </label>
            )}
            {hasSecure && (
              <label className="flex items-center gap-1.5 text-[13px] whitespace-nowrap">
                <input type="checkbox" className="cb" checked={sel.keepSecure} onChange={() => update(s => ({...s, keepSecure: !s.keepSecure}))} />
                Keep HTTPS
              </label>
            )}
          </div>

          {(plan.sites ?? []).length > 0 && (
            <>
              <div className={`${ROW} label-caps h-[34px] border-b border-line-soft bg-subtle text-[11px]!`}>
                <span />
                <span>Site</span>
                <span>Folder (served in place)</span>
                <span>PHP</span>
                <span>HTTPS</span>
                <span>Source</span>
              </div>
              {(plan.sites ?? []).map(s => (
                <SiteRow
                  key={s.path}
                  s={s}
                  sel={sel}
                  parkedBy={parkedBy.get(s.path)}
                  missing={!!s.php && missingPhp.includes(s.php)}
                  onToggle={() => update(x => ({...x, sites: toggleIn(x.sites, s.path)}))}
                />
              ))}
            </>
          )}

          {(plan.databases ?? []).length > 0 && (
            <div className="flex flex-col border-b border-line-soft">
              <div className="flex items-center gap-3 bg-subtle px-4 py-2">
                <span className="label-caps grow text-[11px]!">Databases</span>
                {(plan.databases ?? []).some(d => d.exists) && (
                  <label className="flex items-center gap-2 text-[13px] whitespace-nowrap">
                    Overwrite existing
                    <Toggle on={sel.overwrite} label="Overwrite existing AMPLS databases" onChange={v => update(s => ({...s, overwrite: v}))} />
                  </label>
                )}
              </div>
              {(plan.databases ?? []).map(d => {
                const blocked = d.exists && !sel.overwrite
                return (
                  <label key={d.name} className={`flex h-10 items-center gap-3 border-t border-line-soft px-4 text-[13px] ${blocked ? 'text-muted' : ''}`}>
                    <input
                      type="checkbox"
                      className="cb"
                      disabled={blocked}
                      checked={!blocked && sel.dbs.has(d.name)}
                      onChange={() => update(s => ({...s, dbs: toggleIn(s.dbs, d.name)}))}
                    />
                    <span className="min-w-0 truncate font-mono">{d.name}</span>
                    {d.exists && <Badge tone="warn">exists</Badge>}
                    <span className="grow" />
                    {blocked && <span className="truncate text-xs text-muted">Turn on Overwrite to replace</span>}
                    {d.exists && sel.overwrite && sel.dbs.has(d.name) && <span className="truncate text-xs text-eol">Will replace the AMPLS database</span>}
                    <span className="w-20 shrink-0 text-right font-mono text-xs text-muted">{d.sizeBytes ? bytes(d.sizeBytes) : ''}</span>
                  </label>
                )
              })}
            </div>
          )}

          {(plan.sites ?? []).length === 0 && (plan.databases ?? []).length === 0 && (
            <div className="px-4 py-6 text-[13px] text-muted">Nothing to import from {envName(kind)}.</div>
          )}

          {(plan.notes ?? []).length > 0 && (
            <ul className="m-0 flex list-none flex-col gap-1 px-4 py-3">
              {(plan.notes ?? []).map((n, i) => (
                <li key={i} className="flex items-start gap-2 text-xs text-muted">
                  <Info size={13} className="mt-0.5 shrink-0" />
                  <span className="min-w-0 break-words">{n}</span>
                </li>
              ))}
            </ul>
          )}

          <div className="flex items-center gap-3 border-t border-line-soft bg-subtle px-4 py-3">
            <span className="min-w-0 grow text-[13px] text-muted">
              {shortName(kind)} itself is not changed.
              {kind === 'herd' && ' Stop Herd before using the same .test names in AMPLS.'}
              {kind === 'xampp' && ' Project folders stay where they are.'}
            </span>
            <Button onClick={reset}>Cancel</Button>
            <Button variant="primary" disabled={counts.sites + counts.dbs === 0} onClick={start}>
              {importLabel}
            </Button>
          </div>
        </section>
      )}

      {mysqlOpen && (
        <MySQLDialog
          initial={mysqlSrc ?? {host: '127.0.0.1', port: 3306, user: 'root', password: ''}}
          onClose={() => setMysqlOpen(false)}
          onConnected={(src, p) => {
            setMysqlOpen(false)
            setMysqlSrc(src)
            setKind('mysql')
            setPlan(p)
            setSel(initialSelection(p))
            setError(null)
            setResult(null)
            setPhase('preview')
          }}
        />
      )}
    </>
  )
}
