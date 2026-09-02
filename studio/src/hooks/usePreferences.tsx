import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react"
import { api, type PreferencesPayload, type PreferencesResponse } from "@/lib/api"
import { type TimeRange, isTimeRange } from "@/components/charts/TimeRangeSelector"

export const PREFS_STORAGE_KEY = "restitch.prefs"

const PUT_DEBOUNCE_MS = 500

export interface Preferences {
  pinnedCompositions: string[]
  sidebarCollapsed: boolean
  defaultTimeRange: TimeRange
}

const DEFAULT_PREFS: Preferences = {
  pinnedCompositions: [],
  sidebarCollapsed: false,
  defaultTimeRange: "1h",
}

function toPayload(p: Preferences): PreferencesPayload {
  return {
    pinned_compositions: p.pinnedCompositions,
    sidebar_collapsed: p.sidebarCollapsed,
    default_time_range: p.defaultTimeRange,
  }
}

function fromResponse(r: PreferencesResponse): Preferences {
  return {
    pinnedCompositions: r.pinned_compositions ?? [],
    sidebarCollapsed: r.sidebar_collapsed ?? false,
    // Validate instead of casting: a future server version may send a range
    // this build does not know (finding L22).
    defaultTimeRange: isTimeRange(r.default_time_range) ? r.default_time_range : "1h",
  }
}

// readLocal paints the first frame. A malformed or absent entry falls back to
// defaults rather than throwing during render.
function readLocal(): Preferences {
  try {
    const raw = localStorage.getItem(PREFS_STORAGE_KEY)
    if (!raw) return DEFAULT_PREFS
    return { ...DEFAULT_PREFS, ...JSON.parse(raw) }
  } catch {
    return DEFAULT_PREFS
  }
}

function writeLocal(p: Preferences) {
  try {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify(p))
  } catch {
    // Private-browsing quota errors are not worth failing a render over.
  }
}

interface PreferencesContextValue {
  prefs: Preferences
  setSidebarCollapsed: (v: boolean) => void
  togglePin: (name: string) => void
  setDefaultTimeRange: (v: TimeRange) => void
}

const PreferencesContext = createContext<PreferencesContextValue | null>(null)

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefs] = useState<Preferences>(readLocal)
  const hydrated = useRef(false)
  // Serialised payload the server is already known to hold. The mirror effect
  // compares against it so reconciling does not immediately PUT back the value
  // it just received. A bare `hydrated` flag cannot do this: .finally() sets it
  // before React re-renders from the reconcile's setPrefs, so the mirror effect
  // would see hydrated=true and push a redundant write.
  const lastSynced = useRef<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    let cancelled = false
    api
      .getPreferences()
      .then((res) => {
        if (cancelled) return
        if (res.initialized) {
          // Server has a record: it wins.
          const merged = fromResponse(res)
          lastSynced.current = JSON.stringify(toPayload(merged))
          setPrefs(merged)
          writeLocal(merged)
        } else {
          // Never written (e.g. the user cleared cookies but not
          // localStorage). Adopt local state and seed the new session, rather
          // than letting empty server defaults wipe real preferences.
          const local = readLocal()
          const payload = toPayload(local)
          lastSynced.current = JSON.stringify(payload)
          setPrefs(local)
          writeLocal(local)
          void api.putPreferences(payload).catch(() => {})
        }
      })
      .catch(() => {
        // Offline or server error: keep whatever localStorage gave us.
      })
      .finally(() => {
        if (!cancelled) hydrated.current = true
      })
    return () => {
      cancelled = true
    }
  }, [])

  // Mirror every change locally and push it to the server, debounced.
  useEffect(() => {
    writeLocal(prefs)
    if (!hydrated.current) return

    const payload = toPayload(prefs)
    const serialised = JSON.stringify(payload)
    if (serialised === lastSynced.current) return

    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      lastSynced.current = serialised
      void api.putPreferences(payload).catch(() => {
        // Failed push: drop the marker so the next change retries this state.
        lastSynced.current = null
      })
    }, PUT_DEBOUNCE_MS)
    return () => {
      if (timer.current) clearTimeout(timer.current)
    }
  }, [prefs])

  const value: PreferencesContextValue = {
    prefs,
    setSidebarCollapsed: (v) => setPrefs((p) => ({ ...p, sidebarCollapsed: v })),
    setDefaultTimeRange: (v) => setPrefs((p) => ({ ...p, defaultTimeRange: v })),
    togglePin: (name) =>
      setPrefs((p) => ({
        ...p,
        pinnedCompositions: p.pinnedCompositions.includes(name)
          ? p.pinnedCompositions.filter((n) => n !== name)
          : [...p.pinnedCompositions, name],
      })),
  }

  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>
}

export function usePreferences(): PreferencesContextValue {
  const ctx = useContext(PreferencesContext)
  if (!ctx) throw new Error("usePreferences must be used inside a PreferencesProvider")
  return ctx
}
