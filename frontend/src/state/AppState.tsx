import {createContext, ReactNode, useCallback, useContext, useEffect, useRef, useState} from 'react'
import {backend, errMsg, hasBackend, onProgress, onStatus, Overview, PortConflict} from '../lib/api'

export type Route = 'dashboard' | 'sites' | 'php' | 'mysql' | 'import' | 'logs' | 'settings'

export interface Toast {
  id: number
  kind: 'success' | 'error' | 'info'
  text: string
}

export interface ConfirmOptions {
  title: string
  body?: ReactNode
  confirmLabel?: string
  danger?: boolean
}

interface ConfirmState extends ConfirmOptions {
  resolve: (ok: boolean) => void
}

interface AppState {
  route: Route
  go: (r: Route) => void
  overview: Overview | null
  /** ports Apnoro needs that another program holds (polled with the overview) */
  conflicts: PortConflict[]
  /** increments on every status change; screens reload their data when it changes */
  tick: number
  refresh: () => Promise<void>
  toast: (kind: Toast['kind'], text: string) => void
  toasts: Toast[]
  dismissToast: (id: number) => void
  /** run an async action, toasting errors (and an optional success message) */
  run: <T>(fn: () => Promise<T>, success?: string) => Promise<T | undefined>
  confirm: (o: ConfirmOptions) => Promise<boolean>
  confirmState: ConfirmState | null
  newProjectOpen: boolean
  setNewProjectOpen: (open: boolean) => void
}

const Ctx = createContext<AppState | null>(null)

export function useApp(): AppState {
  const c = useContext(Ctx)
  if (!c) throw new Error('useApp outside provider')
  return c
}

export function AppProvider({children}: {children: ReactNode}) {
  const [route, setRoute] = useState<Route>(() => {
    const h = window.location.hash.replace('#/', '') as Route
    return ['dashboard', 'sites', 'php', 'mysql', 'import', 'logs', 'settings'].includes(h) ? h : 'dashboard'
  })
  const [overview, setOverview] = useState<Overview | null>(null)
  const [conflicts, setConflicts] = useState<PortConflict[]>([])
  const [tick, setTick] = useState(0)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [confirmState, setConfirmState] = useState<ConfirmState | null>(null)
  const [newProjectOpen, setNewProjectOpen] = useState(false)
  const nextId = useRef(1)

  const go = useCallback((r: Route) => {
    setRoute(r)
    window.location.hash = '/' + r
  }, [])

  const dismissToast = useCallback((id: number) => setToasts(t => t.filter(x => x.id !== id)), [])

  const toast = useCallback(
    (kind: Toast['kind'], text: string) => {
      const id = nextId.current++
      setToasts(t => [...t.slice(-3), {id, kind, text}])
      window.setTimeout(() => dismissToast(id), kind === 'error' ? 7000 : 3500)
    },
    [dismissToast],
  )

  const refresh = useCallback(async () => {
    if (!hasBackend()) return
    const [ov, pc] = await Promise.allSettled([backend.Overview(), backend.PortConflicts()])
    if (ov.status === 'fulfilled') setOverview(ov.value)
    else console.error(ov.reason)
    // a backend without coexistence support rejects; treat as no conflicts
    const next = pc.status === 'fulfilled' ? (pc.value ?? []) : []
    setConflicts(prev => (JSON.stringify(prev) === JSON.stringify(next) ? prev : next))
  }, [])

  const run = useCallback(
    async <T,>(fn: () => Promise<T>, success?: string): Promise<T | undefined> => {
      try {
        const r = await fn()
        if (success) toast('success', success)
        return r
      } catch (e) {
        toast('error', errMsg(e))
        return undefined
      }
    },
    [toast],
  )

  const confirm = useCallback(
    (o: ConfirmOptions) =>
      new Promise<boolean>(resolve => {
        setConfirmState({
          ...o,
          resolve: ok => {
            setConfirmState(null)
            resolve(ok)
          },
        })
      }),
    [],
  )

  // Poll overview every 3s while visible; refresh on status events.
  useEffect(() => {
    refresh()
    const id = window.setInterval(() => {
      if (document.visibilityState === 'visible') refresh()
    }, 3000)
    const off = onStatus(() => {
      refresh()
      setTick(t => t + 1)
    })
    // errors from actions the UI did not start (launch-time service start, tray menu)
    const offNotice = onProgress(p => {
      if (p.task === 'notice' && p.error) toast('error', p.error)
    })
    const onVis = () => document.visibilityState === 'visible' && refresh()
    document.addEventListener('visibilitychange', onVis)
    return () => {
      window.clearInterval(id)
      off()
      offNotice()
      document.removeEventListener('visibilitychange', onVis)
    }
  }, [refresh, toast])

  return (
    <Ctx.Provider
      value={{
        route,
        go,
        overview,
        conflicts,
        tick,
        refresh,
        toast,
        toasts,
        dismissToast,
        run,
        confirm,
        confirmState,
        newProjectOpen,
        setNewProjectOpen,
      }}
    >
      {children}
    </Ctx.Provider>
  )
}
