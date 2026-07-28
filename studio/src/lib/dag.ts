const STEP_REF_PATTERN = /steps\.([A-Za-z_][A-Za-z0-9_]*)/g

export interface DagStep {
  name: string
  depends_on?: string[]
  path?: string
  headers?: Record<string, string>
  body?: string
}

export function inferDeps(step: DagStep, allStepNames: string[]): string[] {
  const nameSet = new Set(allStepNames)
  const inferred = new Set<string>()

  const scan = (text: string | undefined) => {
    if (!text) return
    let match: RegExpExecArray | null
    const re = new RegExp(STEP_REF_PATTERN.source, "g")
    while ((match = re.exec(text)) !== null) {
      const ref = match[1]
      if (nameSet.has(ref) && ref !== step.name) {
        inferred.add(ref)
      }
    }
  }

  scan(step.path)
  scan(step.body)
  if (step.headers) {
    for (const v of Object.values(step.headers)) {
      scan(v)
    }
  }

  if (step.depends_on) {
    for (const d of step.depends_on) {
      inferred.add(d)
    }
  }

  return [...inferred]
}

export function computeWaves(steps: DagStep[]): string[][] {
  if (steps.length === 0) return []

  const names = steps.map((s) => s.name)
  const depsMap = new Map<string, string[]>()

  for (const step of steps) {
    depsMap.set(step.name, inferDeps(step, names))
  }

  const inDegree = new Map<string, number>()
  for (const name of names) inDegree.set(name, 0)

  for (const [, deps] of depsMap) {
    for (const dep of deps) {
      inDegree.set(dep, (inDegree.get(dep) ?? 0))
    }
  }
  for (const [name, deps] of depsMap) {
    let count = 0
    for (const dep of deps) {
      if (inDegree.has(dep)) count++
    }
    inDegree.set(name, count)
  }

  const waves: string[][] = []
  const remaining = new Set(names)

  while (remaining.size > 0) {
    const wave: string[] = []
    for (const name of remaining) {
      const deps = depsMap.get(name) ?? []
      const unresolved = deps.filter((d) => remaining.has(d))
      if (unresolved.length === 0) {
        wave.push(name)
      }
    }

    if (wave.length === 0) {
      wave.push(...remaining)
      remaining.clear()
    } else {
      for (const name of wave) {
        remaining.delete(name)
      }
    }

    waves.push(wave.sort())
  }

  return waves
}
