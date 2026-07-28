import { describe, it, expect, vi } from "vitest"
import { render } from "@testing-library/react"

vi.mock("@/lib/api", () => ({
  api: {
    info: vi.fn(),
    reload: vi.fn(),
  },
}))

vi.mock("@/hooks/usePoll", () => ({
  usePoll: () => ({ data: null, error: null, refresh: vi.fn() }),
}))

const mockUsePreferences = vi.fn()

vi.mock("@/hooks/usePreferences", () => ({
  PreferencesProvider: ({ children }: { children: React.ReactNode }) => children,
  usePreferences: () => mockUsePreferences(),
}))

import App from "./App"

describe("App sidebar collapse", () => {
  it("renders the expanded width class when sidebarCollapsed is false", () => {
    mockUsePreferences.mockReturnValue({
      prefs: { pinnedCompositions: [], sidebarCollapsed: false, defaultTimeRange: "1h" },
      setSidebarCollapsed: vi.fn(),
      togglePin: vi.fn(),
      setDefaultTimeRange: vi.fn(),
    })

    const { container } = render(<App />)
    const nav = container.querySelector("nav")
    expect(nav).not.toBeNull()
    expect(nav!.className).toContain("w-52")
    expect(nav!.className).not.toContain("w-14")
  })

  it("renders the collapsed width class when sidebarCollapsed is true", () => {
    mockUsePreferences.mockReturnValue({
      prefs: { pinnedCompositions: [], sidebarCollapsed: true, defaultTimeRange: "1h" },
      setSidebarCollapsed: vi.fn(),
      togglePin: vi.fn(),
      setDefaultTimeRange: vi.fn(),
    })

    const { container } = render(<App />)
    const nav = container.querySelector("nav")
    expect(nav).not.toBeNull()
    expect(nav!.className).toContain("w-14")
    expect(nav!.className).not.toContain("w-52")
  })
})
