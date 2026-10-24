/**
 * The step graph of a composition. depends_on holds every step a step waits
 * for; callers add the template references that POST /api/v1/step-dependencies
 * reports. The frontend never parses templates itself.
 */
export interface DagStep {
  name: string;
  depends_on?: string[];
}

/** dependsOnMap maps each step name to the step names it depends on that exist. */
function dependsOnMap(steps: DagStep[]): Map<string, string[]> {
  const names = new Set(steps.map((s) => s.name));
  return new Map(steps.map((s) => [s.name, (s.depends_on ?? []).filter((d) => names.has(d))]));
}

/** dependsOnTransitively reports whether step reaches target through depends_on. */
export function dependsOnTransitively(steps: DagStep[], step: string, target: string): boolean {
  const deps = dependsOnMap(steps);
  const seen = new Set<string>();
  const stack = [step];
  while (stack.length > 0) {
    const cur = stack.pop()!;
    if (cur === target) return true;
    if (seen.has(cur)) continue;
    seen.add(cur);
    stack.push(...(deps.get(cur) ?? []));
  }
  return false;
}

/**
 * computeWaves groups steps into waves: a step runs in the first wave after
 * all its dependencies. Steps caught in a cycle land together in the last wave.
 */
export function computeWaves(steps: DagStep[]): string[][] {
  const deps = dependsOnMap(steps);
  const remaining = new Set(steps.map((s) => s.name));
  const waves: string[][] = [];
  while (remaining.size > 0) {
    const wave = [...remaining].filter((name) => (deps.get(name) ?? []).every((d) => !remaining.has(d)));
    const next = wave.length > 0 ? wave : [...remaining];
    for (const name of next) remaining.delete(name);
    waves.push(next);
  }
  return waves;
}
