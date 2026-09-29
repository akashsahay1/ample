import {ReactNode, useEffect, useState} from 'react'
import {FolderOpen, ShieldCheck, X} from 'lucide-react'
import {backend, Settings} from '../lib/api'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Badge, Button, Field, PageHeader} from '../components/ui'
import {Logo} from '../components/Shell'

function Section({title, desc, children}: {title: string; desc?: ReactNode; children: ReactNode}) {
  return (
    <section className="card grid grid-cols-[240px_minmax(0,1fr)] gap-6 p-5">
      <div>
        <h2 className="m-0 text-[15px] font-semibold">{title}</h2>
        {desc && <p className="mt-1 mb-0 text-[13px] leading-snug text-muted">{desc}</p>}
      </div>
      <div className="flex flex-col gap-3">{children}</div>
    </section>
  )
}

function Check({label, hint, checked, onChange}: {label: string; hint?: string; checked: boolean; onChange: (v: boolean) => void}) {
  return (
    <label className="flex items-start gap-2.5 text-[13px]">
      <input type="checkbox" className="mt-0.5 h-4 w-4 accent-[#C2461A]" checked={checked} onChange={e => onChange(e.target.checked)} />
      <span>
        {label}
        {hint && <span className="block text-xs text-muted">{hint}</span>}
      </span>
    </label>
  )
}

export default function SettingsScreen() {
  const {run, overview} = useApp()
  const loaded = useLoad(() => backend.GetSettings())
  const [s, setS] = useState<Settings | null>(null)
  const [dirty, setDirty] = useState(false)
  const [version, setVersion] = useState('1.0.0')

  useEffect(() => {
    if (loaded.data && !dirty) setS(loaded.data)
  }, [loaded.data, dirty])
  useEffect(() => {
    backend.AppVersion().then(setVersion).catch(() => {})
  }, [])

  const upd = (patch: Partial<Settings>) => {
    setS(v => (v ? ({...v, ...patch} as Settings) : v))
    setDirty(true)
  }
  const save = async () => {
    if (!s) return
    const ok = await run(() => backend.SaveSettings(s).then(() => true), 'Settings saved')
    if (ok) setDirty(false)
  }
  const port = (key: 'httpPort' | 'httpsPort' | 'mysqlPort', label: string) => (
    <Field label={label}>
      <input
        type="number"
        min={1}
        max={65535}
        className="input mono"
        value={s?.[key] ?? ''}
        onChange={e => upd({[key]: parseInt(e.target.value || '0', 10)} as Partial<Settings>)}
      />
    </Field>
  )

  return (
    <>
      <PageHeader title="Settings" subtitle="Ports, parked directories and how AMPLS starts.">
        {dirty && <span className="text-[13px] text-muted">Unsaved changes</span>}
        <Button disabled={!dirty} onClick={() => setDirty(false)}>
          Discard
        </Button>
        <Button variant="primary" disabled={!dirty || !s} onClick={save}>
          Save changes
        </Button>
      </PageHeader>
      {loaded.error && <div className="text-[13px] text-eol">{loaded.error}</div>}
      {s && (
        <div className="flex flex-col gap-4 pb-4">
          <Section title="General" desc="Sites are served at name.tld. The data directory holds Apache, PHP, MySQL and your databases.">
            <div className="grid grid-cols-[140px_minmax(0,1fr)] gap-3">
              <Field label="Domain TLD">
                <input className="input mono" readOnly value={`.${s.tld}`} />
              </Field>
              <Field label="Data directory">
                <div className="flex gap-2">
                  <input className="input mono grow" readOnly value={s.home} />
                  <Button icon={<FolderOpen size={14} />} onClick={() => run(() => backend.OpenFolder(s.home))}>
                    Open
                  </Button>
                </div>
              </Field>
            </div>
          </Section>

          <Section title="Ports" desc="Changing ports restarts the affected services. Ports 80/443 give you clean URLs.">
            <div className="grid grid-cols-3 gap-3">
              {port('httpPort', 'HTTP')}
              {port('httpsPort', 'HTTPS')}
              {port('mysqlPort', 'MySQL')}
            </div>
          </Section>

          <Section title="Parked directories" desc="Every subfolder of a parked directory becomes a site automatically.">
            <div className="flex flex-col gap-1.5">
              {(s.parked ?? []).map(p => (
                <div key={p} className="flex h-10 items-center gap-2 rounded-lg bg-subtle px-3">
                  <span className="selectable grow truncate font-mono text-xs">{p}</span>
                  <button
                    type="button"
                    className="ib h-7! w-7! border-0 bg-transparent"
                    aria-label={`Remove ${p}`}
                    onClick={() => upd({parked: s.parked.filter(x => x !== p)})}
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
              {(s.parked ?? []).length === 0 && <div className="text-[13px] text-muted">No parked directories.</div>}
            </div>
            <div>
              <Button
                size="md"
                onClick={async () => {
                  const d = await backend.SelectDirectory('Park a directory')
                  if (d && !s.parked.includes(d)) upd({parked: [...(s.parked ?? []), d]})
                }}
              >
                + Park directory
              </Button>
            </div>
          </Section>

          <Section title="Startup" desc="Services run in the background; closing the window keeps AMPLS in the system tray.">
            <Check label="Start services when AMPLS launches" checked={s.startServicesOnLaunch} onChange={v => upd({startServicesOnLaunch: v})} />
            <Check
              label="Stop services when quitting AMPLS"
              hint="Otherwise Apache and MySQL keep running after you quit."
              checked={s.stopServicesOnQuit}
              onChange={v => upd({stopServicesOnQuit: v})}
            />
            <Check label="Launch AMPLS at login" hint="Starts minimised to the tray." checked={s.launchAtLogin} onChange={v => upd({launchAtLogin: v})} />
          </Section>

          <Section title="HTTPS" desc="AMPLS signs site certificates with a local certificate authority. Trust it once so browsers accept https://*.test.">
            <div className="flex items-center gap-3">
              {overview?.caTrusted ? <Badge tone="ok">Trusted</Badge> : <Badge tone="warn">Not trusted</Badge>}
              <Button icon={<ShieldCheck size={14} />} disabled={overview?.caTrusted} onClick={() => run(() => backend.TrustCA(), 'Local HTTPS certificate trusted')}>
                Trust HTTPS certificate
              </Button>
            </div>
          </Section>

          <Section title="About">
            <div className="flex items-center gap-3">
              <Logo size={40} />
              <div>
                <div className="font-display text-lg font-bold">AMPLS {version}</div>
                <div className="text-[13px] text-muted">Apache, MySQL, PHP — Latest Software</div>
              </div>
            </div>
            <p className="m-0 text-xs leading-relaxed text-muted">
              AMPLS bundles and downloads third-party software under their own licenses: Apache HTTP Server (Apache License 2.0, Apache Lounge builds), PHP
              (PHP License v3.01, windows.php.net builds), MySQL Community Server (GPLv2 with FOSS exception), Composer (MIT) and phpMyAdmin (GPLv2).
              Fonts: Bricolage Grotesque, IBM Plex Sans and JetBrains Mono (SIL Open Font License).
            </p>
          </Section>
        </div>
      )}
    </>
  )
}
