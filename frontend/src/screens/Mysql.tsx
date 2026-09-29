import {useState} from 'react'
import {Database as DbIcon, Download, Trash2, Upload} from 'lucide-react'
import {backend, Database, MySQLInfo} from '../lib/api'
import {bytes, copyText} from '../lib/format'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Badge, Button, Empty, Field, IconButton, Modal, PageHeader} from '../components/ui'

const DB_ROW = 'grid grid-cols-[minmax(0,1fr)_90px_90px_130px] items-center gap-3 px-5'

function envBlock(i: MySQLInfo, withPassword: boolean) {
  return [
    'DB_CONNECTION=mysql',
    `DB_HOST=${i.host}`,
    `DB_PORT=${i.port}`,
    'DB_DATABASE=',
    `DB_USERNAME=${i.user}`,
    `DB_PASSWORD=${withPassword ? i.password : i.password ? '••••••••' : ''}`,
  ].join('\n')
}

function PasswordDialog({onClose}: {onClose: () => void}) {
  const {run} = useApp()
  const [pw, setPw] = useState('')
  const [pw2, setPw2] = useState('')
  const mismatch = pw2 !== '' && pw !== pw2
  const submit = async () => {
    const ok = await run(() => backend.SetMySQLPassword(pw).then(() => true), pw ? 'Root password changed' : 'Root password cleared')
    if (ok) onClose()
  }
  return (
    <Modal
      title="Change root password"
      onClose={onClose}
      width={440}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" disabled={mismatch || pw !== pw2} onClick={submit}>
            Change password
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={e => {
          e.preventDefault()
          if (pw === pw2) submit()
        }}
      >
        <Field label="New password" hint="Leave empty for no password (local development only).">
          <input type="password" className="input mono" value={pw} onChange={e => setPw(e.target.value)} autoFocus />
        </Field>
        <Field label="Confirm password" hint={mismatch ? <span className="text-danger">Passwords don't match</span> : undefined}>
          <input type="password" className="input mono" value={pw2} onChange={e => setPw2(e.target.value)} />
        </Field>
        <p className="m-0 text-xs text-muted">Remember to update DB_PASSWORD in your projects' .env / wp-config.php files.</p>
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}

function NewDbDialog({onClose}: {onClose: () => void}) {
  const {run} = useApp()
  const [name, setName] = useState('')
  const valid = /^[A-Za-z0-9_$-]{1,64}$/.test(name)
  const submit = async () => {
    if (!valid) return
    const ok = await run(() => backend.CreateDatabase(name).then(() => true), `Database ${name} created`)
    if (ok) onClose()
  }
  return (
    <Modal
      title="New database"
      onClose={onClose}
      width={420}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" disabled={!valid} onClick={submit}>
            Create
          </Button>
        </>
      }
    >
      <form
        onSubmit={e => {
          e.preventDefault()
          submit()
        }}
      >
        <Field label="Database name" hint="Letters, digits and underscores. Charset utf8mb4.">
          <input className="input mono" value={name} onChange={e => setName(e.target.value)} placeholder="my_app" autoFocus spellCheck={false} />
        </Field>
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}

function ImportDialog({dbs, onClose}: {dbs: Database[]; onClose: () => void}) {
  const {run} = useApp()
  const [db, setDb] = useState(dbs[0]?.name ?? '')
  const [file, setFile] = useState('')
  const submit = async () => {
    const ok = await run(() => backend.ImportSQL(db, file).then(() => true), `Imported into ${db}`)
    if (ok) onClose()
  }
  return (
    <Modal
      title="Import .sql"
      onClose={onClose}
      width={480}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" disabled={!db || !file} onClick={submit}>
            Import
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <Field label="Into database">
          <select className="input mono" value={db} onChange={e => setDb(e.target.value)}>
            {dbs.map(d => (
              <option key={d.name}>{d.name}</option>
            ))}
          </select>
        </Field>
        <Field label="SQL file">
          <div className="flex gap-2">
            <input className="input mono grow" readOnly value={file} placeholder="No file selected" />
            <Button
              onClick={async () => {
                const f = await backend.SelectFile('Choose a SQL dump', '*.sql')
                if (f) setFile(f)
              }}
            >
              Browse…
            </Button>
          </div>
        </Field>
      </div>
    </Modal>
  )
}

export default function Mysql() {
  const {run, confirm, overview} = useApp()
  const info = useLoad(() => backend.MySQLInfo())
  const dbs = useLoad(() => backend.ListDatabases())
  const [showPw, setShowPw] = useState(false)
  const [dialog, setDialog] = useState<'pw' | 'db' | 'import' | null>(null)
  const i = info.data
  const running = overview?.services.find(s => s.name === 'mysql')?.running ?? i?.running ?? false
  const list = dbs.data ?? []

  const kv = 'flex items-center h-11 border-b border-line-soft text-[13px]'
  const k = 'w-[110px] shrink-0 text-muted'

  return (
    <>
      <PageHeader
        title="MySQL"
        subtitle={
          <>
            MySQL {i?.version ? i.version.split('.').slice(0, 2).join('.') : '8.4'} LTS · data in <span className="font-mono">{i?.dataDir ?? '…'}</span>
          </>
        }
      >
        {running ? (
          <Button onClick={() => run(() => backend.RestartService('mysql'), 'MySQL restarted')}>Restart</Button>
        ) : (
          <Button onClick={() => run(() => backend.StartService('mysql'), 'MySQL started')}>Start</Button>
        )}
        <Button variant="primary" onClick={() => backend.OpenURL('http://localhost/phpmyadmin')}>
          Open phpMyAdmin
        </Button>
      </PageHeader>

      <div className="grid min-h-0 grid-cols-5 gap-5">
        <section className="card col-span-2 flex flex-col gap-1 self-start p-5">
          <div className="mb-2 flex items-center">
            <h2 className="m-0 grow text-[15px] font-semibold">Connection</h2>
            {running ? <Badge tone="ok">Running</Badge> : <Badge tone="off">Stopped</Badge>}
          </div>
          {i && (
            <>
              <div className={kv}>
                <span className={k}>Host</span>
                <span className="selectable font-mono">{i.host}</span>
              </div>
              <div className={kv}>
                <span className={k}>Port</span>
                <span className="selectable font-mono">{i.port}</span>
              </div>
              <div className={kv}>
                <span className={k}>Username</span>
                <span className="selectable font-mono">{i.user}</span>
              </div>
              <div className={kv}>
                <span className={k}>Password</span>
                <span className="selectable grow font-mono">{i.password ? (showPw ? i.password : '••••••••') : <span className="text-muted">(empty)</span>}</span>
                {i.password && (
                  <Button size="sm" onClick={() => setShowPw(v => !v)} aria-pressed={showPw}>
                    {showPw ? 'Hide' : 'Show'}
                  </Button>
                )}
              </div>
              <div className={`${kv} border-b-0`}>
                <span className={k}>Socket</span>
                <span className="font-mono text-muted">n/a (TCP)</span>
              </div>
              <div className="mt-3 flex gap-2">
                <Button onClick={() => run(() => copyText(envBlock(i, true)), '.env block copied')}>Copy .env block</Button>
                <Button disabled={!running} onClick={() => setDialog('pw')}>
                  Change password
                </Button>
              </div>
              <pre className="selectable mt-3 mb-0 rounded-[10px] bg-shell p-3.5 font-mono text-xs leading-relaxed text-[#E6E7EA]">{envBlock(i, showPw)}</pre>
            </>
          )}
          {info.error && <div className="text-[13px] text-eol">{info.error}</div>}
        </section>

        <section className="card col-span-3 flex flex-col self-start overflow-hidden">
          <div className="flex items-center gap-2.5 border-b border-line-soft px-5 py-4">
            <h2 className="m-0 grow text-[15px] font-semibold">Databases</h2>
            <Button size="md" icon={<Upload size={14} />} disabled={!running || list.length === 0} onClick={() => setDialog('import')}>
              Import .sql
            </Button>
            <Button size="md" variant="primary" disabled={!running} onClick={() => setDialog('db')}>
              New database
            </Button>
          </div>
          <div className={`${DB_ROW} label-caps h-10 bg-subtle`}>
            <span>Name</span>
            <span>Tables</span>
            <span>Size</span>
            <span className="text-right">Actions</span>
          </div>
          {!running ? (
            <Empty icon={<DbIcon size={28} />} title="MySQL is stopped">
              Start MySQL to see and manage your databases.
              <div className="mt-4">
                <Button variant="primary" onClick={() => run(() => backend.StartService('mysql'), 'MySQL started')}>
                  Start MySQL
                </Button>
              </div>
            </Empty>
          ) : list.length === 0 && !dbs.loading ? (
            <Empty icon={<DbIcon size={28} />} title="No databases yet">
              Create one here, or tick “Create a MySQL database” when making a new project.
            </Empty>
          ) : (
            list.map((d, idx) => (
              <div key={d.name} className={`${DB_ROW} h-[52px] text-[13px] ${idx === list.length - 1 ? '' : 'border-b border-line-soft'}`}>
                <span className="selectable truncate font-mono font-medium">{d.name}</span>
                <span>{d.tables}</span>
                <span>{bytes(d.sizeBytes)}</span>
                <span className="flex items-center justify-end gap-1.5">
                  <Button
                    size="sm"
                    icon={<Download size={13} />}
                    onClick={async () => {
                      const f = await backend.SaveFile(`Export ${d.name}`, `${d.name}.sql`)
                      if (f) await run(() => backend.ExportDatabase(d.name, f), `Exported ${d.name}`)
                    }}
                  >
                    Export
                  </Button>
                  <IconButton
                    label={`Drop ${d.name}`}
                    onClick={async () => {
                      const ok = await confirm({
                        title: `Drop database ${d.name}?`,
                        body: (
                          <>
                            This permanently deletes <b>{d.name}</b> and its {d.tables} tables. Export it first if you might need the data.
                          </>
                        ),
                        confirmLabel: 'Drop database',
                        danger: true,
                      })
                      if (ok) await run(() => backend.DropDatabase(d.name), `Dropped ${d.name}`)
                    }}
                  >
                    <Trash2 size={15} />
                  </IconButton>
                </span>
              </div>
            ))
          )}
          {dbs.error && running && <div className="px-5 py-3 text-[13px] text-eol">{dbs.error}</div>}
        </section>
      </div>
      {dialog === 'pw' && <PasswordDialog onClose={() => setDialog(null)} />}
      {dialog === 'db' && <NewDbDialog onClose={() => setDialog(null)} />}
      {dialog === 'import' && <ImportDialog dbs={list} onClose={() => setDialog(null)} />}
    </>
  )
}
