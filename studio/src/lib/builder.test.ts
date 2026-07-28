import { describe, it, expect } from "vitest"
import * as yaml from "js-yaml"
import { buildYaml, type BuilderState } from "./builder"

describe("buildYaml", () => {
  it("generates valid YAML with expected keys", () => {
    const state: BuilderState = {
      compositionName: "test-comp",
      path: "/api/test",
      method: "GET",
      upstreams: [{ name: "my-api", url: "http://localhost:8081" }],
      steps: [
        { name: "fetch", upstream: "my-api", method: "GET", path: "/data", optional: false, depends_on: "" },
        { name: "enrich", upstream: "my-api", method: "POST", path: "/enrich", optional: true, depends_on: "fetch" },
      ],
      responseBody: 'result: "{{ steps.fetch.body }}"',
    }

    const output = buildYaml(state)
    const parsed = yaml.load(output) as Record<string, unknown>

    expect(parsed).toHaveProperty("upstreams")
    expect(parsed).toHaveProperty("compositions")

    const upstreams = parsed.upstreams as Record<string, { url: string }>
    expect(upstreams["my-api"]).toEqual({ url: "http://localhost:8081" })

    const compositions = parsed.compositions as Record<string, unknown>
    expect(compositions).toHaveProperty("test-comp")

    const comp = compositions["test-comp"] as Record<string, unknown>
    expect(comp.path).toBe("/api/test")
    expect(comp.method).toBe("GET")

    const steps = comp.steps as Array<Record<string, unknown>>
    expect(steps).toHaveLength(2)
    expect(steps[0].name).toBe("fetch")
    expect(steps[1].optional).toBe(true)
    expect(steps[1].depends_on).toEqual(["fetch"])
  })

  it("skips empty upstream names", () => {
    const state: BuilderState = {
      compositionName: "test",
      path: "/test",
      method: "GET",
      upstreams: [{ name: "", url: "http://localhost" }, { name: "valid", url: "http://localhost" }],
      steps: [],
      responseBody: "{}",
    }

    const output = buildYaml(state)
    const parsed = yaml.load(output) as Record<string, unknown>
    const upstreams = parsed.upstreams as Record<string, unknown>
    expect(Object.keys(upstreams)).toEqual(["valid"])
  })
})
