import {ButtonHTMLAttributes, ReactNode, useEffect, useRef, useState} from 'react'
import {Loader2, X} from 'lucide-react'

type BtnVariant = 'default' | 'primary' | 'danger'

interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onClick'> {
  variant?: BtnVariant
  size?: 'sm' | 'md' | 'lg'
  /** may return a promise; the button shows a spinner until it settles */
  onClick?: () => unknown
  pending?: boolean
  icon?: ReactNode
}

export function Button({variant = 'default', size = 'lg', onClick, pending, icon, className = '', children, disabled, ...rest}: ButtonProps) {
  const [busy, setBusy] = useState(false)
  const mounted = useRef(true)
  useEffect(() => () => void (mounted.current = false), [])
  const isBusy = busy || !!pending
  const cls = [
    // never wrap or shrink (also enforced by .btn in index.css)
    'btn shrink-0 whitespace-nowrap',
    variant === 'primary' ? 'btn-pri' : variant === 'danger' ? 'btn-danger' : '',
    size === 'sm' ? 'btn-sm' : size === 'md' ? 'btn-md' : '',
    className,
  ].join(' ')
  return (
    <button
      type="button"
      {...rest}
      className={cls}
      disabled={disabled || isBusy}
      aria-busy={isBusy || undefined}
      onClick={async () => {
        if (!onClick) return
        const r = onClick()
        if (r && typeof (r as Promise<unknown>).then === 'function') {
          setBusy(true)
          try {
            await r
          } finally {
            if (mounted.current) setBusy(false)
          }
        }
      }}
    >
      {isBusy ? <Loader2 size={14} className="spin" aria-hidden /> : icon}
      {children}
    </button>
  )
}

interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onClick'> {
  label: string
  onClick?: () => unknown
}

export function IconButton({label, onClick, children, disabled, ...rest}: IconButtonProps) {
  const [busy, setBusy] = useState(false)
  return (
    <button
      type="button"
      className="ib"
      aria-label={label}
      title={label}
      disabled={disabled || busy}
      onClick={async () => {
        const r = onClick?.()
        if (r && typeof (r as Promise<unknown>).then === 'function') {
          setBusy(true)
          try {
            await r
          } finally {
            setBusy(false)
          }
        }
      }}
      {...rest}
    >
      {busy ? <Loader2 size={15} className="spin" /> : children}
    </button>
  )
}

export function PageHeader({title, subtitle, children}: {title: string; subtitle?: ReactNode; children?: ReactNode}) {
  return (
    <div className="flex items-end gap-3">
      <div className="min-w-0 grow">
        <h1 className="m-0 font-display text-[32px] leading-tight font-bold tracking-[-0.01em]">{title}</h1>
        {subtitle && <p className="mt-1 mb-0 text-sm text-muted">{subtitle}</p>}
      </div>
      {children}
    </div>
  )
}

export function Badge({tone, children}: {tone: 'ok' | 'off' | 'php' | 'mysql' | 'apache' | 'eol' | 'warn'; children: ReactNode}) {
  const tones: Record<string, string> = {
    ok: 'text-ok bg-ok-bg',
    off: 'text-muted bg-[#EEEBE5]',
    php: 'text-php bg-php-bg',
    mysql: 'text-mysql bg-mysql-bg',
    apache: 'text-apache bg-apache-bg',
    eol: 'text-eol bg-eol-bg',
    warn: 'text-[#8A5A00] bg-[#FDF1D6]',
  }
  return <span className={`badge ${tones[tone]}`}>{children}</span>
}

export function Toggle({on, onChange, label, disabled}: {on: boolean; onChange: (v: boolean) => unknown; label: string; disabled?: boolean}) {
  const [busy, setBusy] = useState(false)
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled || busy}
      onClick={async () => {
        setBusy(true)
        try {
          await onChange(!on)
        } finally {
          setBusy(false)
        }
      }}
      className="relative h-[22px] w-10 shrink-0 rounded-full border-0 transition-colors disabled:opacity-60"
      style={{background: on ? '#1F7A4D' : '#C9C6BF'}}
    >
      <span className="absolute top-[3px] h-4 w-4 rounded-full bg-white transition-all" style={{left: on ? 21 : 3}} />
    </button>
  )
}

export function Modal({
  title,
  onClose,
  children,
  footer,
  width = 520,
  dismissable = true,
}: {
  title: ReactNode
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  width?: number
  dismissable?: boolean
}) {
  const ref = useRef<HTMLDivElement>(null)
  // Callers pass inline onClose functions; keep them in refs so re-renders (every
  // keystroke in a form) never re-run the focus effect and steal focus.
  const onCloseRef = useRef(onClose)
  const dismissableRef = useRef(dismissable)
  onCloseRef.current = onClose
  dismissableRef.current = dismissable

  // Initial focus, once per open: an [autofocus] field, else the first text field,
  // else the primary button. Never the header's close button.
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null
    const root = ref.current
    const pick = (sel: string) => root?.querySelector<HTMLElement>(sel)
    const el =
      pick('[autofocus], [data-autofocus]') ??
      pick('input:not([type=checkbox]):not([type=radio]):not([readonly]):not([disabled]), select, textarea') ??
      pick('button.btn-pri')
    if (el && !root?.contains(document.activeElement)) el.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && dismissableRef.current) onCloseRef.current()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      prev?.focus?.()
    }
  }, [])
  return (
    <div
      className="fade-in fixed inset-0 z-40 flex items-center justify-center bg-[rgba(22,24,29,0.45)] p-6"
      onMouseDown={e => {
        if (e.target === e.currentTarget && dismissable) onClose()
      }}
    >
      <div ref={ref} role="dialog" aria-modal="true" className="card flex max-h-full flex-col shadow-2xl" style={{width}}>
        <div className="flex items-center gap-3 border-b border-line-soft px-5 py-4">
          <h2 className="m-0 grow font-display text-lg font-bold">{title}</h2>
          {dismissable && (
            <button type="button" className="ib border-0" aria-label="Close dialog" onClick={onClose}>
              <X size={16} />
            </button>
          )}
        </div>
        <div className="scroll-thin min-h-0 overflow-auto px-5 py-4">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t border-line-soft px-5 py-3">{footer}</div>}
      </div>
    </div>
  )
}

export function Field({label, children, hint}: {label: string; children: ReactNode; hint?: ReactNode}) {
  return (
    <label className="flex flex-col gap-1 text-xs text-muted">
      <span className="font-medium">{label}</span>
      {children}
      {hint && <span className="text-[12px] text-muted">{hint}</span>}
    </label>
  )
}

export function Empty({icon, title, children}: {icon?: ReactNode; title: string; children?: ReactNode}) {
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-12 text-center">
      {icon && <div className="mb-1 text-muted">{icon}</div>}
      <div className="text-[15px] font-semibold">{title}</div>
      <div className="max-w-[460px] text-[13px] text-muted">{children}</div>
    </div>
  )
}

export function ProgressBar({percent, color = '#3F48B8'}: {percent: number; color?: string}) {
  const indeterminate = percent < 0
  return (
    <div className="h-1.5 overflow-hidden rounded-full bg-line" role="progressbar" aria-valuenow={indeterminate ? undefined : Math.round(percent)} aria-valuemin={0} aria-valuemax={100}>
      <div
        className="h-1.5 rounded-full transition-[width] duration-200"
        style={{width: indeterminate ? '40%' : `${Math.max(2, Math.min(100, percent))}%`, background: color}}
      />
    </div>
  )
}

export function ServiceIcon({kind}: {kind: 'apache' | 'mysql' | 'php'}) {
  const map = {
    apache: ['A', 'bg-apache-bg text-apache'],
    mysql: ['M', 'bg-mysql-bg text-mysql'],
    php: ['P', 'bg-php-bg text-php'],
  } as const
  const [l, c] = map[kind]
  return <div className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-[9px] text-[15px] font-bold ${c}`}>{l}</div>
}
