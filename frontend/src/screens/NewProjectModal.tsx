import {ReactNode, useEffect, useRef, useState} from 'react'
import {AlertCircle, CheckCircle2, FileCode2, Newspaper, Sparkles} from 'lucide-react'
import {backend, errMsg, onProgress, Progress, ProjectKind, Site} from '../lib/api'
import {joinPath, shortPath, slug} from '../lib/format'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Button, Field, Modal, ProgressBar} from '../components/ui'

const KINDS: {kind: ProjectKind; name: string; desc: string; icon: ReactNode; tint: string}[] = [
  {kind: 'laravel', name: 'Laravel', desc: 'Latest Laravel via Composer, .env wired to MySQL.', icon: <Sparkles size={18} />, tint: 'bg-apache-bg text-apache'},
  {kind: 'wordpress', name: 'WordPress', desc: 'Latest WordPress with wp-config.php ready to go.', icon: <Newspaper size={18} />, tint: 'bg-mysql-bg text-mysql'},
  {kind: 'blank', name: 'Blank PHP', desc: 'An empty folder with index.php — bring your own code.', icon: <FileCode2 size={18} />, tint: 'bg-php-bg text-php'},
]

type Phase = 'form' | 'running' | 'done' | 'error'

export default function NewProjectModal({onClose}: {onClose: () => void}) {
  const {overview, run} = useApp()
  const settings = useLoad(() => backend.GetSettings())
  const [kind, setKind] = useState<ProjectKind>('laravel')
  const [name, setName] = useState('')
  const [dir, setDir] = useState('')
  const [php, setPhp] = useState('')
  const [createDb, setCreateDb] = useState(true)
  const [phase, setPhase] = useState<Phase>('form')
  const [progress, setProgress] = useState<Progress | null>(null)
  const [log, setLog] = useState<string[]>([])
  const [error, setError] = useState('')
  const [site, setSite] = useState<Site | null>(null)
  const offRef = useRef<() => void>(() => {})

  const tld = settings.data?.tld ?? 'test'
  const parked = settings.data?.parked ?? []
  const s = slug(name)
  const phpVersions = overview?.phpVersions ?? []

  useEffect(() => {
    if (!dir && parked[0]) setDir(parked[0])
  }, [parked, dir])
  useEffect(() => () => offRef.current(), [])

  const create = async () => {
    if (!s) return
    setPhase('running')
    setLog([])
    setProgress({task: '', message: 'Starting…', percent: 0, done: false, error: ''})
    const task = `project:${s}`
    offRef.current = onProgress(p => {
      if (p.task !== task) return
      setProgress(p)
      if (p.message) setLog(l => (l[l.length - 1] === p.message ? l : [...l, p.message]))
    })
    try {
      const created = await backend.NewProject({name: s, kind, directory: dir, php, createDb})
      setSite(created)
      setPhase('done')
    } catch (e) {
      setError(errMsg(e))
      setPhase('error')
    } finally {
      offRef.current()
    }
  }

  const running = phase === 'running'

  if (phase !== 'form') {
    return (
      <Modal title={phase === 'done' ? 'Project ready' : phase === 'error' ? 'Project failed' : `Creating ${s}.${tld}`} onClose={onClose} dismissable={!running} width={560}>
        <div className="flex flex-col gap-4 py-1">
          {phase === 'running' && (
            <>
              <div className="flex items-center justify-between text-[13px]">
                <span className="text-body">{progress?.message || 'Working…'}</span>
                <span className="font-mono text-xs text-muted">{progress && progress.percent >= 0 ? `${Math.round(progress.percent)}%` : ''}</span>
              </div>
              <ProgressBar percent={progress?.percent ?? -1} color="#C2461A" />
            </>
          )}
          {phase === 'done' && site && (
            <div className="flex items-start gap-3">
              <CheckCircle2 size={22} color="#1C6E45" className="mt-0.5 shrink-0" />
              <div className="text-[13px] leading-relaxed">
                <div className="text-[15px] font-semibold">{site.domain} is live</div>
                <div className="text-muted">
                  Served from <span className="font-mono">{shortPath(site.path)}</span> with PHP {site.php}.
                </div>
              </div>
            </div>
          )}
          {phase === 'error' && (
            <div className="flex items-start gap-3">
              <AlertCircle size={22} color="#B42318" className="mt-0.5 shrink-0" />
              <div className="selectable text-[13px] leading-relaxed break-words text-body">{error}</div>
            </div>
          )}
          {log.length > 0 && (
            <pre className="scroll-dark m-0 max-h-40 overflow-auto rounded-[10px] bg-shell p-3.5 font-mono text-xs leading-relaxed text-[#E6E7EA]">
              {log.map(l => `› ${l}`).join('\n')}
            </pre>
          )}
          <div className="flex justify-end gap-2">
            {phase === 'done' && site && (
              <>
                <Button onClick={() => run(() => backend.OpenFolder(site.path))}>Open folder</Button>
                <Button onClick={() => run(() => backend.OpenTerminal(site.path))}>Open terminal</Button>
                <Button variant="primary" onClick={() => (backend.OpenURL(site.url), onClose())}>
                  Open site
                </Button>
              </>
            )}
            {phase === 'error' && (
              <>
                <Button onClick={onClose}>Close</Button>
                <Button variant="primary" onClick={() => setPhase('form')}>
                  Back
                </Button>
              </>
            )}
          </div>
        </div>
      </Modal>
    )
  }

  return (
    <Modal
      title="New project"
      onClose={onClose}
      width={620}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" disabled={!s || !dir} onClick={create}>
            Create project
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={e => {
          e.preventDefault()
          if (s && dir) create()
        }}
      >
        <div role="radiogroup" aria-label="Project type" className="grid grid-cols-3 gap-2.5">
          {KINDS.map(k => {
            const on = kind === k.kind
            return (
              <button
                key={k.kind}
                type="button"
                role="radio"
                aria-checked={on}
                onClick={() => {
                  setKind(k.kind)
                  setCreateDb(k.kind !== 'blank')
                }}
                className={`flex flex-col items-start gap-2 rounded-xl border bg-white p-3.5 text-left transition-shadow ${on ? 'border-[#E8622C] shadow-[0_0_0_3px_#FBE6DC]' : 'border-line hover:border-field'}`}
              >
                <span className={`flex h-8 w-8 items-center justify-center rounded-lg ${k.tint}`}>{k.icon}</span>
                <span className="text-sm font-semibold">{k.name}</span>
                <span className="text-xs leading-snug text-muted">{k.desc}</span>
              </button>
            )
          })}
        </div>

        <Field
          label="Project name"
          hint={
            s ? (
              <>
                Will be served at <span className="font-mono text-link">{s}.{tld}</span>
                {dir && (
                  <>
                    {' '}
                    from <span className="font-mono">{shortPath(joinPath(dir, s))}</span>
                  </>
                )}
              </>
            ) : (
              'Lowercase letters, digits and dashes.'
            )
          }
        >
          <input className="input text-sm" value={name} onChange={e => setName(e.target.value)} placeholder="my-app" autoFocus spellCheck={false} />
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Location">
            <div className="flex gap-2">
              <select className="input mono min-w-0 grow" value={dir} onChange={e => setDir(e.target.value)}>
                {!dir && <option value="">Choose a folder…</option>}
                {parked.map(p => (
                  <option key={p} value={p}>
                    {shortPath(p)}
                  </option>
                ))}
                {dir && !parked.includes(dir) && <option value={dir}>{shortPath(dir)}</option>}
              </select>
              <Button
                onClick={async () => {
                  const d = await run(() => backend.SelectDirectory('Create the project in…'))
                  if (d) setDir(d)
                }}
              >
                Browse…
              </Button>
            </div>
          </Field>
          <Field label="PHP version">
            <select className="input mono" value={php} onChange={e => setPhp(e.target.value)}>
              <option value="">Default ({overview?.defaultPhp || '—'})</option>
              {phpVersions.map(v => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <label className="flex items-center gap-2.5 text-[13px]">
          <input type="checkbox" className="h-4 w-4 accent-[#C2461A]" checked={createDb} onChange={e => setCreateDb(e.target.checked)} />
          Create a MySQL database
          {s && <span className="font-mono text-xs text-muted">{s.replace(/-/g, '_')}</span>}
        </label>
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}
