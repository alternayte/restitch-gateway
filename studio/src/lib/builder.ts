import * as yaml from "js-yaml"

export interface StepState {
  name: string
  upstream: string
  method: string
  path: string
  optional: boolean
  depends_on: string
}

export interface BuilderState {
  compositionName: string
  path: string
  method: string
  upstreams: { name: string; url: string }[]
  steps: StepState[]
  responseBody: string
}

export function buildYaml(state: BuilderState): string {
  const upstreams: Record<string, { url: string }> = {}
  for (const u of state.upstreams) {
    if (u.name) upstreams[u.name] = { url: u.url }
  }

  const steps = state.steps.map((s) => {
    const step: Record<string, unknown> = {
      name: s.name,
      upstream: s.upstream,
      path: s.path,
      method: s.method,
    }
    if (s.optional) step.optional = true
    if (s.depends_on.trim()) step.depends_on = s.depends_on.split(",").map((d) => d.trim())
    return step
  })

  let responseBody: unknown
  try {
    responseBody = yaml.load(state.responseBody)
  } catch {
    responseBody = state.responseBody
  }

  const doc = {
    upstreams,
    compositions: {
      [state.compositionName]: {
        path: state.path,
        method: state.method,
        steps,
        response: { status: 200, body: responseBody },
      },
    },
  }

  return yaml.dump(doc, { lineWidth: -1 })
}
