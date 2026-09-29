import {useCallback, useEffect, useRef, useState} from 'react'
import {errMsg, hasBackend} from '../lib/api'
import {useApp} from './AppState'

/** Loads data via fn, reloading whenever the global status tick changes. */
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const {tick} = useApp()
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const fnRef = useRef(fn)
  fnRef.current = fn

  const reload = useCallback(async () => {
    if (!hasBackend()) {
      setLoading(false)
      setError('Backend not connected (run inside AMPLS / wails dev).')
      return
    }
    try {
      const d = await fnRef.current()
      setData(d)
      setError(null)
    } catch (e) {
      setError(errMsg(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick, reload, ...deps])

  return {data, error, loading, reload, setData}
}
