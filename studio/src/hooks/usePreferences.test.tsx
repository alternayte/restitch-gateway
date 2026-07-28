import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { render, screen, waitFor, act } from "@testing-library/react"

const getPreferences = vi.fn()
const putPreferences = vi.fn()

vi.mock("@/lib/api", () => ({
  api: {
    getPreferences: (...a: unknown[]) => getPreferences(...a),
    putPreferences: (...a: unknown[]) => putPreferences(...a),
  },
}))

import { PreferencesProvider, usePreferences, PREFS_STORAGE_KEY } from "./usePreferences"

function Probe() {
  const { prefs, togglePin } = usePreferences()
  return (
    <div>
      <span data-testid="range">{prefs.defaultTimeRange}</span>
      <span data-testid="collapsed">{String(prefs.sidebarCollapsed)}</span>
      <span data-testid="pins">{prefs.pinnedCompositions.join(",")}</span>
      <button onClick={() => togglePin("new-comp")}>pin</button>
    </div>
  )
}

beforeEach(() => {
  localStorage.clear()
  getPreferences.mockReset()
  putPreferences.mockReset()
  putPreferences.mockResolvedValue({
    pinned_compositions: [], sidebar_collapsed: false,
    default_time_range: "1h", initialized: true,
  })
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
})

describe("usePreferences", () => {
  it("paints from localStorage before the server responds", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["cached"], sidebarCollapsed: true, defaultTimeRange: "24h",
    }))
    getPreferences.mockReturnValue(new Promise(() => {})) // never resolves

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    expect(screen.getByTestId("range").textContent).toBe("24h")
    expect(screen.getByTestId("collapsed").textContent).toBe("true")
    expect(screen.getByTestId("pins").textContent).toBe("cached")
  })

  it("server wins when the record is initialized", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["stale"], sidebarCollapsed: false, defaultTimeRange: "1h",
    }))
    getPreferences.mockResolvedValue({
      pinned_compositions: ["from-server"], sidebar_collapsed: true,
      default_time_range: "6h", initialized: true,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => {
      expect(screen.getByTestId("pins").textContent).toBe("from-server")
    })
    expect(screen.getByTestId("range").textContent).toBe("6h")
    expect(putPreferences).not.toHaveBeenCalled()
  })

  it("adopts localStorage and seeds the server when uninitialized", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["keep-me"], sidebarCollapsed: true, defaultTimeRange: "24h",
    }))
    getPreferences.mockResolvedValue({
      pinned_compositions: [], sidebar_collapsed: false,
      default_time_range: "1h", initialized: false,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => {
      expect(putPreferences).toHaveBeenCalledWith({
        pinned_compositions: ["keep-me"],
        sidebar_collapsed: true,
        default_time_range: "24h",
      })
    })
    // The cookie-cleared case must not wipe the user's real preferences.
    expect(screen.getByTestId("pins").textContent).toBe("keep-me")
  })

  it("keeps local state when the fetch fails", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["offline"], sidebarCollapsed: false, defaultTimeRange: "6h",
    }))
    getPreferences.mockRejectedValue(new Error("network down"))

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => expect(getPreferences).toHaveBeenCalled())
    expect(screen.getByTestId("pins").textContent).toBe("offline")
    expect(screen.getByTestId("range").textContent).toBe("6h")
  })

  it("debounces rapid changes into a single PUT", async () => {
    getPreferences.mockResolvedValue({
      pinned_compositions: [], sidebar_collapsed: false,
      default_time_range: "1h", initialized: true,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)
    await waitFor(() => expect(getPreferences).toHaveBeenCalled())

    const button = screen.getByText("pin")
    act(() => { button.click(); button.click(); button.click() })
    act(() => { vi.advanceTimersByTime(600) })

    await waitFor(() => expect(putPreferences).toHaveBeenCalledTimes(1))
  })

  it("mirrors state back to localStorage", async () => {
    getPreferences.mockResolvedValue({
      pinned_compositions: ["srv"], sidebar_collapsed: true,
      default_time_range: "6h", initialized: true,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => {
      const raw = localStorage.getItem(PREFS_STORAGE_KEY)
      expect(raw).not.toBeNull()
      expect(JSON.parse(raw as string).pinnedCompositions).toEqual(["srv"])
    })
  })
})
