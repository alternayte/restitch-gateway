import type { Edge } from "@xyflow/react";
import type { CSSProperties } from "react";

/** Named is a step with a name, and its depends_on as strings. */
export type Named = { name: string; depends_on: string[] };

const inferredStyle: CSSProperties = { strokeDasharray: "5 4" };

/** Inferred maps a step name to the steps its templates reference, as Studio's compiler reports them. */
export type Inferred = Record<string, string[]>;

/** inferredFor lists the template references of step that name another drawn step and are not in its depends_on. */
export function inferredFor(step: Named, inferred: Inferred, names: Set<string>): string[] {
  return (inferred[step.name] ?? []).filter((d) => d !== step.name && names.has(d) && !step.depends_on.includes(d));
}

/**
 * stepEdges builds the edges of the graph. depends_on edges are solid and
 * removable; template references are dashed and read-only, because only an
 * edit of the template changes them.
 */
export function stepEdges(list: Named[], inferred: Inferred, selected: ReadonlySet<string>): Edge[] {
  const names = new Set(list.map((s) => s.name));
  return list.flatMap((s) => [
    ...s.depends_on
      .filter((d) => names.has(d))
      .map((d): Edge => {
        const id = `${d}->${s.name}`;
        return { id, source: d, target: s.name, selected: selected.has(id) };
      }),
    ...inferredFor(s, inferred, names).map(
      (d): Edge => ({
        id: `template:${d}->${s.name}`,
        source: d,
        target: s.name,
        style: inferredStyle,
        deletable: false,
        selectable: false,
        focusable: false,
      }),
    ),
  ]);
}
