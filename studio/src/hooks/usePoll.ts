import { useState, useEffect, useCallback, useRef } from "react"

export function usePoll<T>(fn: () => Promise<T>, ms: number) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<Error | null>(null)
  const fnRef = useRef(fn)
  fnRef.current = fn

  const refresh = useCallback(async () => {
    try {
      const result = await fnRef.current()
      setData(result)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)))
    }
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, ms)
    const onVisibility = () => {
      if (document.visibilityState === "visible") refresh()
    }
    document.addEventListener("visibilitychange", onVisibility)
    return () => {
      clearInterval(id)
      document.removeEventListener("visibilitychange", onVisibility)
    }
  }, [ms, refresh])

  return { data, error, refresh }
}
