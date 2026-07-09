import { describe, it, expect, vi } from "vitest"
import { render } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"

vi.mock("@/lib/api", () => ({
  api: {
    stats: vi.fn(),
    upstreams: vi.fn(),
    timeseries: vi.fn(),
  },
}))

vi.mock("@/hooks/usePoll", () => ({
  usePoll: () => ({ data: null, error: null, refresh: vi.fn() }),
}))

import Dashboard from "./Dashboard"

describe("Dashboard", () => {
  it("renders without crashing", () => {
    expect(() =>
      render(
        <MemoryRouter>
          <Dashboard />
        </MemoryRouter>
      )
    ).not.toThrow()
  })
})
