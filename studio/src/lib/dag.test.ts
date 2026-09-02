import { describe, it, expect } from "vitest"
import { computeWaves, inferDeps } from "./dag"

describe("inferDeps", () => {
  it("extracts step references from path", () => {
    const deps = inferDeps(
      { name: "bonus", path: "/users/{{ steps.loyalty.body.id }}" },
      ["loyalty", "bonus", "user"]
    )
    expect(deps).toEqual(["loyalty"])
  })

  it("merges explicit depends_on with inferred", () => {
    const deps = inferDeps(
      { name: "c", depends_on: ["a"], path: "/{{ steps.b.body }}" },
      ["a", "b", "c"]
    )
    expect(deps.sort()).toEqual(["a", "b"])
  })

  it("does not include self-references", () => {
    const deps = inferDeps(
      { name: "x", path: "/{{ steps.x.body }}" },
      ["x"]
    )
    expect(deps).toEqual([])
  })
})

describe("computeWaves", () => {
  it("puts independent steps in wave 1", () => {
    const waves = computeWaves([
      { name: "a" },
      { name: "b" },
    ])
    expect(waves).toEqual([["a", "b"]])
  })

  it("computes M3 example: user+loyalty in wave 1, bonus in wave 2", () => {
    const waves = computeWaves([
      { name: "user", path: "/users/1" },
      { name: "loyalty", path: "/loyalty" },
      { name: "bonus", path: "/users/{{ steps.loyalty.body.id }}" },
    ])
    expect(waves).toHaveLength(2)
    expect(waves[0]).toContain("user")
    expect(waves[0]).toContain("loyalty")
    expect(waves[1]).toEqual(["bonus"])
  })

  it("handles linear chain", () => {
    const waves = computeWaves([
      { name: "a" },
      { name: "b", depends_on: ["a"] },
      { name: "c", depends_on: ["b"] },
    ])
    expect(waves).toEqual([["a"], ["b"], ["c"]])
  })

  it("returns empty for empty input", () => {
    expect(computeWaves([])).toEqual([])
  })

  it("handles diamond dependency", () => {
    const waves = computeWaves([
      { name: "root" },
      { name: "left", depends_on: ["root"] },
      { name: "right", depends_on: ["root"] },
      { name: "join", depends_on: ["left", "right"] },
    ])
    expect(waves).toHaveLength(3)
    expect(waves[0]).toEqual(["root"])
    expect(waves[1]!.sort()).toEqual(["left", "right"])
    expect(waves[2]).toEqual(["join"])
  })
})
