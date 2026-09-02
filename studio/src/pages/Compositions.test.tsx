import "@testing-library/jest-dom"
import { describe, it, expect, vi } from "vitest"
import { render, screen, fireEvent } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"

const comps = [
  { name: "alpha", path: "/a", method: "GET", public: true, steps: [], waves: [] },
  { name: "beta", path: "/b", method: "GET", public: false, steps: [], waves: [] },
  { name: "gamma", path: "/g", method: "GET", public: false, steps: [], waves: [] },
]

vi.mock("@/lib/api", () => ({ api: { compositions: vi.fn() } }))
vi.mock("@/hooks/usePoll", () => ({
  usePoll: () => ({ data: comps, error: null, refresh: vi.fn() }),
}))
// Declared outside the factory so a test can assert on it. A vi.fn() created
// inside the factory is unreachable, which is how the pin button being inert
// went unnoticed: every test passed with the togglePin call deleted entirely.
const mockTogglePin = vi.fn()

vi.mock("@/hooks/usePreferences", () => ({
  usePreferences: () => ({
    prefs: { pinnedCompositions: ["gamma"], sidebarCollapsed: false, defaultTimeRange: "1h" },
    togglePin: (name: string) => mockTogglePin(name),
    setSidebarCollapsed: vi.fn(),
    setDefaultTimeRange: vi.fn(),
  }),
}))

const mockNavigate = vi.fn()
vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual<typeof import("react-router-dom")>("react-router-dom")
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

import Compositions from "./Compositions"

describe("Compositions", () => {
  it("calls togglePin with the composition name when the pin is clicked", () => {
    mockTogglePin.mockClear()
    render(<MemoryRouter><Compositions /></MemoryRouter>)
    fireEvent.click(screen.getByRole("button", { name: /pin alpha/i }))
    expect(mockTogglePin).toHaveBeenCalledWith("alpha")
  })

  it("sorts pinned compositions to the top", () => {
    render(<MemoryRouter><Compositions /></MemoryRouter>)
    const rows = screen.getAllByTestId("composition-row")
    expect(rows[0]).toHaveTextContent("gamma")
  })

  it("renders a pin control per row", () => {
    render(<MemoryRouter><Compositions /></MemoryRouter>)
    expect(screen.getAllByRole("button", { name: /pin/i })).toHaveLength(3)
  })

  it("does not navigate when the pin is clicked", () => {
    render(<MemoryRouter><Compositions /></MemoryRouter>)
    const pinButtons = screen.getAllByRole("button", { name: /pin/i })
    fireEvent.click(pinButtons[0]!)
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})
