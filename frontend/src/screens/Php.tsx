import {useEffect, useState} from 'react'
import {Code2} from 'lucide-react'
import {backend, Extension, onProgress, PHPVersion, Progress} from '../lib/api'
import {bytes} from '../lib/format'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Button, Empty, PageHeader, ProgressBar} from '../components/ui'

const INI_KEYS = ['memory_limit', 'upload_max_filesize', 'post_max_size', 'max_execution_time'] as const

function SettingsPanel({version}: {version: string}) {
  const {run} = useApp()
  const s = useLoad(() => backend.GetPHPSettings(version), [version])
  const [ini, setIni] = useState<Record<string, string>>({})
  const [exts, setExts] = useState<Extension[]>([])
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    if (s.data && !dirty) {
      setIni({...s.data.ini})
      setExts(s.data.extensions.map(e => ({...e})))
    }
  }, [s.data, dirty])

  const save = async () => {
    const ok = await run(() => backend.SavePHPSettings({version, ini, extensions: exts}).then(() => true), `PHP ${version} settings saved — Apache restarted`)
    if (ok) setDirty(false)
  }

  return (
    <section className="card col-span-2 flex flex-col gap-3 self-start p-5">
      <div className="flex items-center gap-2">
        <h2 className="m-0 grow text-[15px] font-semibold">PHP {version} settings</h2>
        <Button
          size="md"
          onClick={async () => {
            const p = await backend.PHPIniPath(version)
            await run(() => backend.OpenPath(p))
          }}
        >
          Edit php.ini
        </Button>
      </div>
      {s.error && <div className="text-[13px] text-eol">{s.error}</div>}
      <div className="grid grid-cols-2 gap-2.5">
        {INI_KEYS.map(k => (
          <label key={k} className="flex flex-col gap-1 text-xs text-muted">
            {k}
            <input
              className="input mono h-[34px]!"
              value={ini[k] ?? ''}
              onChange={e => {
                setIni(v => ({...v, [k]: e.target.value}))
                setDirty(true)
              }}
              spellCheck={false}
            />
          </label>
        ))}
        <label className="flex flex-col gap-1 text-xs text-muted">
          display_errors
          <select
            className="input mono h-[34px]!"
            value={(ini.display_errors ?? 'On').toLowerCase() === 'off' || ini.display_errors === '0' ? 'Off' : 'On'}
            onChange={e => {
              setIni(v => ({...v, display_errors: e.target.value}))
              setDirty(true)
            }}
          >
            <option>On</option>
            <option>Off</option>
          </select>
        </label>
      </div>
      <div className="label-caps mt-1">Extensions</div>
      <div className="scroll-thin grid max-h-[300px] grid-cols-2 gap-1.5 overflow-auto">
        {exts.map((e, i) => (
          <label key={e.name} className="flex h-10 items-center gap-2.5 rounded-lg bg-subtle px-3 text-[13px]">
            <input
              type="checkbox"
              className="h-4 w-4 accent-[#3F48B8]"
              checked={e.enabled}
              onChange={ev => {
                const next = [...exts]
                next[i] = {...e, enabled: ev.target.checked}
                setExts(next)
                setDirty(true)
              }}
            />
            {e.name}
          </label>
        ))}
      </div>
      <div className="mt-1 flex items-center justify-end gap-2">
        {dirty && <span className="grow text-xs text-muted">Unsaved changes</span>}
        <Button
          disabled={!dirty}
          onClick={() => {
            setDirty(false)
          }}
        >
          Reset
        </Button>
        <Button variant="primary" disabled={!dirty} onClick={save}>
          Save
        </Button>
      </div>
    </section>
  )
}

function VersionRow({
  v,
  selected,
  onSelect,
  progress,
  onInstall,
}: {
  v: PHPVersion
  selected: boolean
  onSelect: () => void
  progress?: Progress
  onInstall: () => Promise<void>
}) {
  const {run, confirm} = useApp()
  const installing = !!progress && !progress.done
  if (!v.installed) {
    return (
      <div className="flex items-center gap-3.5 rounded-xl border border-line bg-subtle px-4 py-3.5">
        <span className="w-14 font-display text-[22px] font-bold text-muted">{v.version}</span>
        {installing ? (
          <div className="flex grow flex-col gap-1.5">
            <div className="font-mono text-xs text-muted">{progress.message || 'Downloading…'}</div>
            <ProgressBar percent={progress.percent} />
          </div>
        ) : (
          <div className="grow">
            <div className="font-mono text-xs text-muted">
              {v.full} · ≈ {bytes(v.downloadSize)} download
            </div>
            {v.eol && <div className="text-xs text-eol">End of life — for legacy projects only</div>}
          </div>
        )}
        <Button size="md" variant="primary" pending={installing} onClick={onInstall}>
          {installing ? 'Installing' : 'Install'}
        </Button>
      </div>
    )
  }
  const sitesLabel = `${v.siteCount} site${v.siteCount === 1 ? '' : 's'}`
  return (
    <div
      role="button"
      tabIndex={0}
      aria-pressed={selected}
      onClick={onSelect}
      onKeyDown={e => (e.key === 'Enter' || e.key === ' ') && e.target === e.currentTarget && (e.preventDefault(), onSelect())}
      className={`flex cursor-pointer items-center gap-3.5 rounded-xl border bg-white px-4 py-3.5 ${selected ? 'border-[#9AA3FF] shadow-[0_0_0_3px_#E8EAFF]' : 'border-line hover:border-field'}`}
    >
      <span className="w-14 font-display text-[22px] font-bold">{v.version}</span>
      <div className="grow">
        <div className="font-mono text-xs">{v.full} · NTS x64</div>
        <div className={`text-xs ${v.eol ? 'text-eol' : 'text-muted'}`}>{v.eol ? `End of life · ${sitesLabel}` : sitesLabel}</div>
      </div>
      {v.default ? (
        <span className="badge bg-php text-white">Default</span>
      ) : (
        <>
          <Button size="md" onClick={() => run(() => backend.SetDefaultPHP(v.version), `PHP ${v.version} is now the default`)}>
            Make default
          </Button>
          <Button
            size="md"
            onClick={async () => {
              const ok = await confirm({
                title: `Remove PHP ${v.version}?`,
                body:
                  v.siteCount > 0 ? (
                    <>
                      {sitesLabel} currently use PHP {v.version}. Switch them to another version first.
                    </>
                  ) : (
                    <>The PHP {v.version} binaries and its php.ini will be deleted. You can reinstall it any time.</>
                  ),
                confirmLabel: 'Remove',
                danger: true,
              })
              if (ok) await run(() => backend.RemovePHP(v.version), `PHP ${v.version} removed`)
            }}
          >
            Remove
          </Button>
        </>
      )}
    </div>
  )
}

export default function Php() {
  const {run, toast} = useApp()
  const list = useLoad(() => backend.ListPHP())
  const [selected, setSelected] = useState('')
  const [progress, setProgress] = useState<Record<string, Progress>>({})

  useEffect(
    () =>
      onProgress(p => {
        if (!p.task.startsWith('php:install:')) return
        const v = p.task.slice('php:install:'.length)
        setProgress(m => ({...m, [v]: p}))
      }),
    [],
  )

  const versions = list.data ?? []
  const installed = versions.filter(v => v.installed)
  const available = versions.filter(v => !v.installed)
  const current = installed.find(v => v.version === selected) ?? installed.find(v => v.default) ?? installed[0]

  const install = async (v: string) => {
    setProgress(m => ({...m, [v]: {task: `php:install:${v}`, message: 'Starting download…', percent: -1, done: false, error: ''}}))
    const ok = await run(() => backend.InstallPHP(v).then(() => true))
    setProgress(m => {
      const n = {...m}
      delete n[v]
      return n
    })
    if (ok) {
      toast('success', `PHP ${v} installed`)
      setSelected(v)
    }
    await list.reload()
  }

  return (
    <>
      <PageHeader title="PHP Versions" subtitle="Install side by side. Official Windows builds from windows.php.net.">
        <Button onClick={() => list.reload()}>Check for updates</Button>
      </PageHeader>
      {list.error && <div className="text-[13px] text-eol">{list.error}</div>}
      <div className="grid min-h-0 grid-cols-5 gap-5">
        <div className="col-span-3 flex flex-col gap-2.5">
          <div className="label-caps">Installed</div>
          {installed.length === 0 && !list.loading && (
            <div className="card">
              <Empty icon={<Code2 size={28} />} title="No PHP installed">
                Install a version below — sites need at least one PHP version to run.
              </Empty>
            </div>
          )}
          {installed.map(v => (
            <VersionRow key={v.version} v={v} selected={current?.version === v.version} onSelect={() => setSelected(v.version)} onInstall={async () => {}} />
          ))}
          {available.length > 0 && <div className="label-caps mt-2">Available</div>}
          {available.map(v => (
            <VersionRow key={v.version} v={v} selected={false} onSelect={() => {}} progress={progress[v.version]} onInstall={() => install(v.version)} />
          ))}
        </div>
        {current ? (
          <SettingsPanel key={current.version} version={current.version} />
        ) : (
          <section className="card col-span-2 self-start p-5 text-[13px] text-muted">Install a PHP version to edit its settings.</section>
        )}
      </div>
    </>
  )
}
