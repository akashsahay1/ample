import {useEffect, useRef, useState} from 'react'
import {Copy, FolderOpen, RefreshCw} from 'lucide-react'
import {backend, errMsg} from '../lib/api'
import {copyText} from '../lib/format'
import {useApp} from '../state/AppState'
import {useLoad} from '../state/useLoad'
import {Button, PageHeader, Toggle} from '../components/ui'

function label(n: string) {
  if (n === 'apache-error') return 'Apache errors'
  if (n === 'apache-access') return 'Apache access'
  if (n === 'mysql') return 'MySQL'
  if (n.startsWith('php-')) return `PHP ${n.slice(4)}`
  return n
}

export default function Logs() {
  const {overview, run, toast} = useApp()
  const names = useLoad(() => backend.LogNames())
  const [active, setActive] = useState('')
  const [text, setText] = useState('')
  const [err, setErr] = useState('')
  const [auto, setAuto] = useState(true)
  const pre = useRef<HTMLPreElement>(null)
  const stick = useRef(true)

  const list = names.data ?? []
  const current = active && list.includes(active) ? active : list[0] ?? ''

  const load = async () => {
    if (!current) return
    try {
      const t = await backend.ReadLog(current, 500)
      setText(t)
      setErr('')
    } catch (e) {
      setErr(errMsg(e))
      setText('')
    }
  }

  useEffect(() => {
    stick.current = true
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current])

  useEffect(() => {
    if (!auto) return
    const id = window.setInterval(() => document.visibilityState === 'visible' && load(), 2000)
    return () => window.clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [auto, current])

  useEffect(() => {
    const el = pre.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [text])

  return (
    <>
      <PageHeader title="Logs" subtitle="The last 500 lines of each log. Newest at the bottom.">
        <Button icon={<Copy size={14} />} disabled={!text} onClick={() => (copyText(text), toast('success', 'Log copied'))}>
          Copy
        </Button>
        <Button icon={<FolderOpen size={14} />} disabled={!overview} onClick={() => overview && run(() => backend.OpenFolder(overview.home + '\\logs'))}>
          Open logs folder
        </Button>
      </PageHeader>
      <div className="flex items-center gap-3">
        <div role="tablist" aria-label="Log files" className="flex flex-wrap gap-1 rounded-[10px] border border-line bg-white p-1">
          {list.map(n => (
            <button
              key={n}
              role="tab"
              type="button"
              aria-selected={n === current}
              onClick={() => setActive(n)}
              className={`h-8 rounded-[7px] border-0 px-3 text-[13px] font-medium ${n === current ? 'bg-shell text-white' : 'bg-transparent text-body hover:bg-subtle'}`}
            >
              {label(n)}
            </button>
          ))}
        </div>
        <span className="grow" />
        <label className="flex items-center gap-2 text-[13px] text-muted">
          Auto-refresh
          <Toggle on={auto} onChange={setAuto} label="Auto-refresh" />
        </label>
        <Button size="md" icon={<RefreshCw size={14} />} onClick={load}>
          Refresh
        </Button>
      </div>
      <pre
        ref={pre}
        onScroll={e => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        }}
        className="scroll-dark selectable m-0 min-h-[300px] grow overflow-auto rounded-[14px] bg-shell p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap text-[#E6E7EA]"
        tabIndex={0}
        aria-label={`${label(current)} log`}
      >
        {err ? <span className="text-[#F08A6B]">{err}</span> : text || <span className="text-shell-muted">This log is empty.</span>}
      </pre>
    </>
  )
}
