import { describe, it, expect, vi } from "vitest"
import { render } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router-dom"

vi.mock("@/lib/api", () => ({
  api: {
    compositions: vi.fn(),
    requests: vi.fn(),
    timeseries: vi.fn(),
    stepMetrics: vi.fn(),
  },
}))

vi.mock("@/hooks/usePoll", () => ({
  usePoll: () => ({ data: null, error: null, refresh: vi.fn() }),
}))

import CompositionDetail from "./CompositionDetail"

describe("CompositionDetail", () => {
  it("renders without crashing", () => {
    expect(() =>
      render(
        <MemoryRouter initialEntries={["/compositions/test-comp"]}>
          <Routes>
            <Route path="/compositions/:name" element={<CompositionDetail />} />
          </Routes>
        </MemoryRouter>
      )
    ).not.toThrow()
  })
})
