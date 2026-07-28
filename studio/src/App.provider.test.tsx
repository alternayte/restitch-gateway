import { describe, it, expect, vi } from "vitest"
import { render, waitFor } from "@testing-library/react"

// This file deliberately does NOT mock @/hooks/usePreferences.
//
// App.test.tsx mocks both PreferencesProvider and usePreferences so it can
// drive sidebar state directly. That is the right call for those assertions,
// but it makes the provider itself invisible: removing the
// <PreferencesProvider> wrapper from App leaves every one of those tests green,
// while the real app white-screens because usePreferences throws.
//
// Rendering App against the REAL hook is what closes that gap.
vi.mock("@/lib/api", () => ({
  api: {
    info: vi.fn(),
    reload: vi.fn(),
    getPreferences: vi.fn().mockResolvedValue({
      pinned_compositions: [],
      sidebar_collapsed: false,
      default_time_range: "1h",
      initialized: true,
    }),
    putPreferences: vi.fn().mockResolvedValue({
      pinned_compositions: [],
      sidebar_collapsed: false,
      default_time_range: "1h",
      initialized: true,
    }),
  },
}))

vi.mock("@/hooks/usePoll", () => ({
  usePoll: () => ({ data: null, error: null, refresh: vi.fn() }),
}))

import App from "./App"

describe("App preferences provider", () => {
  it("mounts PreferencesProvider so the real usePreferences hook resolves", async () => {
    // If App stops wrapping Shell in PreferencesProvider, the real hook throws
    // "usePreferences must be used inside a PreferencesProvider" and this
    // render fails — which in production is a blank page, not a subtle bug.
    const { container } = render(<App />)
    await waitFor(() => {
      expect(container.querySelector("nav")).not.toBeNull()
    })
  })
})
