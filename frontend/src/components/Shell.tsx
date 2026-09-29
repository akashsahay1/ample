import {CSSProperties} from 'react'
import {AlertCircle, CheckCircle2, Info, Minus, Square, X} from 'lucide-react'
import {WindowMinimise, WindowToggleMaximise} from '../../wailsjs/runtime/runtime'
import {backend, hasBackend} from '../lib/api'
import {Route, useApp} from '../state/AppState'
import {Button, Modal} from './ui'

export function Logo({size = 18}: {size?: number}) {
  return (
    <svg viewBox="0 0 256 256" width={size} height={size} aria-hidden="true">
      <rect width="256" height="256" rx="58" fill="#2A2F39" />
      <path d="M70 200 L128 56 L186 200" fill="none" stroke="#F5F3EF" strokeWidth="24" strokeLinecap="round" strokeLinejoin="round" />
      <rect x="113" y="129" width="30" height="12" rx="6" fill="#E8622C" />
      <rect x="104" y="151" width="48" height="12" rx="6" fill="#9AA3FF" />
      <rect x="96" y="173" width="64" height="12" rx="6" fill="#3FA9C9" />
    </svg>
  )
}

const drag = {'--wails-draggable': 'drag'} as CSSProperties
const noDrag = {'--wails-draggable': 'no-drag'} as CSSProperties

export function TitleBar() {
  const wc = (fn: () => void) => () => hasBackend() && fn()
  return (
    <div
      className="flex h-10 shrink-0 items-center gap-2.5 bg-shell pl-3.5"
      style={drag}
      onDoubleClick={wc(WindowToggleMaximise)}
    >
      <Logo />
      <span className="text-[13px] font-semibold tracking-[0.04em] text-canvas">AMPLS</span>
      <span className="grow" />
      <div className="flex" style={noDrag}>
        <button type="button" className="wc" aria-label="Minimize" onClick={wc(WindowMinimise)}>
          <Minus size={15} />
        </button>
        <button type="button" className="wc" aria-label="Maximize" onClick={wc(WindowToggleMaximise)}>
          <Square size={12} />
        </button>
        <button type="button" className="wc close" aria-label="Close to tray" title="Close to tray" onClick={wc(() => backend.HideWindow())}>
          <X size={16} />
        </button>
      </div>
    </div>
  )
}

const I = {
  dashboard: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <rect x="3" y="3" width="7" height="9" rx="1" />
      <rect x="14" y="3" width="7" height="5" rx="1" />
      <rect x="14" y="12" width="7" height="9" rx="1" />
      <rect x="3" y="16" width="7" height="5" rx="1" />
    </svg>
  ),
  sites: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />
    </svg>
  ),
  php: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <path d="M8 7l-5 5 5 5M16 7l5 5-5 5" />
    </svg>
  ),
  mysql: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <ellipse cx="12" cy="5" rx="8" ry="3" />
      <path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3" />
    </svg>
  ),
  logs: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <path d="M4 6h16M4 12h16M4 18h10" />
    </svg>
  ),
  settings: (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1" />
    </svg>
  ),
}

const NAV: [Route, string][] = [
  ['dashboard', 'Dashboard'],
  ['sites', 'Sites'],
  ['php', 'PHP Versions'],
  ['mysql', 'MySQL'],
  ['logs', 'Logs'],
  ['settings', 'Settings'],
]

export function Sidebar() {
  const {route, go, overview} = useApp()
  const services = overview?.services ?? []
  const stopped = services.filter(s => !s.running).length
  let dot = '#3FB57A'
  let label = 'All services running'
  if (!overview) {
    dot = '#9A9EA8'
    label = hasBackend() ? 'Checking services…' : 'Backend not connected'
  } else if (stopped === services.length && services.length > 0) {
    dot = '#9A9EA8'
    label = 'All services stopped'
  } else if (stopped > 0) {
    dot = '#F2B544'
    label = `${stopped} stopped`
  }
  return (
    <nav className="flex w-56 shrink-0 flex-col gap-1 bg-shell px-3 py-4" aria-label="Main">
      {NAV.map(([r, name]) => (
        <button key={r} type="button" className={`nav ${route === r ? 'on' : ''}`} aria-current={route === r ? 'page' : undefined} onClick={() => go(r)}>
          {I[r]}
          {name}
        </button>
      ))}
      <span className="grow" />
      <button
        type="button"
        onClick={() => go('dashboard')}
        className="flex flex-col gap-1.5 rounded-[10px] border-0 bg-shell-hover p-3 text-left"
        aria-label={`Service status: ${label}`}
      >
        <div className="flex items-center gap-2 text-[13px] font-semibold text-canvas">
          <span className="h-2 w-2 rounded-full" style={{background: dot}} />
          {label}
        </div>
        <div className="truncate text-xs text-shell-muted">
          AMPLS {overview?.appVersion ?? '1.0.0'}
          {overview?.home ? ` · ${overview.home}` : ''}
        </div>
      </button>
    </nav>
  )
}

export function Toasts() {
  const {toasts, dismissToast} = useApp()
  return (
    <div className="pointer-events-none fixed right-5 bottom-5 z-50 flex w-[380px] flex-col gap-2" aria-live="polite">
      {toasts.map(t => (
        <div
          key={t.id}
          className="toast-in pointer-events-auto flex items-start gap-2.5 rounded-xl bg-shell px-4 py-3 text-[13px] text-[#E6E7EA] shadow-xl"
          role={t.kind === 'error' ? 'alert' : 'status'}
        >
          <span className="mt-px shrink-0">
            {t.kind === 'success' ? (
              <CheckCircle2 size={16} color="#3FB57A" />
            ) : t.kind === 'error' ? (
              <AlertCircle size={16} color="#F08A6B" />
            ) : (
              <Info size={16} color="#9AA3FF" />
            )}
          </span>
          <span className="selectable grow break-words">{t.text}</span>
          <button type="button" aria-label="Dismiss" className="shrink-0 border-0 bg-transparent p-0 text-shell-muted hover:text-white" onClick={() => dismissToast(t.id)}>
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  )
}

export function ConfirmDialog() {
  const {confirmState: c} = useApp()
  if (!c) return null
  return (
    <Modal
      title={c.title}
      onClose={() => c.resolve(false)}
      width={440}
      footer={
        <>
          <Button onClick={() => c.resolve(false)}>Cancel</Button>
          <Button variant={c.danger ? 'danger' : 'primary'} onClick={() => c.resolve(true)} autoFocus>
            {c.confirmLabel ?? 'Confirm'}
          </Button>
        </>
      }
    >
      <div className="text-[13px] leading-relaxed text-body">{c.body}</div>
    </Modal>
  )
}
