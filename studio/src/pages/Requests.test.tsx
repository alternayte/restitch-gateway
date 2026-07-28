import { describe, it, expect, vi } from "vitest"
import { render } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"

vi.mock("@/lib/api", () => ({
  api: {
    compositions: vi.fn(),
    requests: vi.fn(),
  },
}))

vi.mock("@/hooks/usePoll", () => ({
  usePoll: () => ({ data: null, error: null, refresh: vi.fn() }),
}))

import Requests from "./Requests"

describe("Requests", () => {
  it("renders without crashing", () => {
    expect(() =>
      render(
        <MemoryRouter>
          <Requests />
        </MemoryRouter>
      )
    ).not.toThrow()
  })
})
