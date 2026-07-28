import { describe, it, expect, vi } from "vitest"
import { render, screen, fireEvent } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"

vi.mock("@/lib/api", () => ({
  api: {
    // Return real-shaped data so Dashboard renders past its loading skeleton;
    // the TimeRangeSelector only exists in the loaded branch.
    stats: vi.fn(() => ({ total_requests: 0, total_errors: 0, per_composition: {} })),
    upstreams: vi.fn(() => []),
    timeseries: vi.fn(() => []),
  },
}))

vi.mock("@/hooks/usePoll", () => ({
  // Invokes the fetcher so tests can assert on the arguments it was called
  // with (e.g. that the current range prop reached api.timeseries).
  usePoll: (fn: () => unknown) => ({ data: fn(), error: null, refresh: vi.fn() }),
}))

// Declared outside the factory so a test can assert on it. A vi.fn() created
// inside the factory is unreachable from the test (see Compositions.test.tsx).
let mockDefaultTimeRange: "1h" | "6h" | "24h" = "1h"
const mockSetDefaultTimeRange = vi.fn()

vi.mock("@/hooks/usePreferences", () => ({
  usePreferences: () => ({
    prefs: { pinnedCompositions: [], sidebarCollapsed: false, defaultTimeRange: mockDefaultTimeRange },
    togglePin: vi.fn(),
    setSidebarCollapsed: vi.fn(),
    setDefaultTimeRange: (v: string) => mockSetDefaultTimeRange(v),
  }),
}))

import { api } from "@/lib/api"
import Dashboard from "./Dashboard"

describe("Dashboard", () => {
  it("persists the range when a new one is selected", () => {
    // The seed direction (prefs -> api.timeseries) is covered separately. This
    // is the write direction: without it, replacing the selector's onChange
    // with a no-op — so a chosen range never persists — passes every test.
    mockSetDefaultTimeRange.mockClear()
    render(
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    )
    fireEvent.click(screen.getByRole("button", { name: "24h" }))
    expect(mockSetDefaultTimeRange).toHaveBeenCalledWith("24h")
  })

  it("renders without crashing", () => {
    expect(() =>
      render(
        <MemoryRouter>
          <Dashboard />
        </MemoryRouter>
      )
    ).not.toThrow()
  })

  it("uses the preference-backed default time range for the timeseries call", () => {
    mockDefaultTimeRange = "6h"
    render(
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    )
    expect(api.timeseries).toHaveBeenCalledWith("6h", "1m")
  })
})
