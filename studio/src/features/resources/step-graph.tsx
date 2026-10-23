import {
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { X } from "lucide-react";
import { useState, type CSSProperties } from "react";
import { Button } from "@/components/ui/button";
import { computeWaves, dependsOnTransitively } from "@/lib/dag";

/** Step is one entry of a composition's steps list; fields other than name and depends_on pass through. */
export type Step = Record<string, unknown> & { name?: unknown; depends_on?: unknown };

type Named = { name: string; depends_on: string[] };

/** named lists the steps that have a name, with their depends_on as strings. */
function named(steps: Step[]): Named[] {
  return steps.flatMap((s) =>
    typeof s.name === "string" && s.name !== ""
      ? [{ name: s.name, depends_on: Array.isArray(s.depends_on) ? s.depends_on.filter((d): d is string => typeof d === "string") : [] }]
      : [],
  );
}

/** withDependsOn returns steps with the depends_on of step replaced; an empty list removes the key. */
function withDependsOn(steps: Step[], step: string, next: string[]): Step[] {
  return steps.map((s) => {
    if (s.name !== step) return s;
    const copy: Step = { ...s };
    if (next.length > 0) copy.depends_on = next;
    else delete copy.depends_on;
    return copy;
  });
}

const selectClass = "h-8 min-w-0 flex-1 rounded-control border bg-panel px-2 text-sm sm:flex-none sm:w-40";

const NODE_W = 160;
const COL = 220;
const ROW = 72;

type StepNode = Node<{ label: string }, "step">;

function StepNodeView({ data }: NodeProps<StepNode>) {
  return (
    <div
      className="flex h-10 items-center justify-center rounded-control border bg-panel px-3 font-mono text-xs text-foreground shadow-panel"
      style={{ width: NODE_W }}
    >
      <Handle type="target" position={Position.Left} />
      <span className="truncate">{data.label}</span>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

const nodeTypes = { step: StepNodeView };

const fitOptions = { padding: 0.2, maxZoom: 1 };

const edgeOptions = { markerEnd: { type: MarkerType.ArrowClosed, color: "var(--muted-foreground)" } };

// React Flow reads its colours from these variables; the tokens follow the light and dark themes.
const flowStyle = {
  "--xy-edge-stroke": "var(--muted-foreground)",
  "--xy-edge-stroke-selected": "var(--accent)",
  "--xy-connectionline-stroke": "var(--accent)",
  "--xy-handle-background-color": "var(--accent)",
  "--xy-handle-border-color": "var(--panel)",
  "--xy-background-pattern-dots-color": "var(--input)",
  "--xy-controls-button-background-color": "var(--panel)",
  "--xy-controls-button-background-color-hover": "var(--muted)",
  "--xy-controls-button-color": "var(--foreground)",
  "--xy-controls-button-border-color": "var(--border)",
} as CSSProperties;

/**
 * StepGraph draws the steps of a composition and edits their depends_on.
 * An edge runs from a dependency to the step that depends on it. Drawing an
 * edge adds depends_on, deleting one removes it, and an edge that would make
 * a cycle is refused. Template references to steps.X also order steps at
 * runtime; the graph does not show them.
 */
export function StepGraph({ steps, onChange }: { steps: Step[]; onChange: (next: Step[]) => void }) {
  const list = named(steps);
  const [error, setError] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  // The edges come from depends_on, so React Flow's edge selection is kept here.
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const onEdgesChange = (changes: EdgeChange[]) => {
    setSelected((cur) => {
      const next = new Set(cur);
      for (const c of changes) {
        if (c.type === "select" && c.selected) next.add(c.id);
        else if (c.type === "select" || c.type === "remove") next.delete(c.id);
      }
      return next;
    });
  };

  const connect = (step: string, dep: string) => {
    const target = list.find((s) => s.name === step);
    if (!target || !list.some((s) => s.name === dep)) return;
    if (step === dep) {
      setError(`${step} cannot depend on itself.`);
      return;
    }
    if (target.depends_on.includes(dep)) {
      setError("");
      return;
    }
    if (dependsOnTransitively(list, dep, step)) {
      setError(`${step} cannot depend on ${dep}: ${dep} already depends on ${step}.`);
      return;
    }
    setError("");
    onChange(withDependsOn(steps, step, [...target.depends_on, dep]));
  };

  const disconnect = (pairs: { step: string; dep: string }[]) => {
    let next = steps;
    for (const { step, dep } of pairs) {
      const cur = named(next).find((s) => s.name === step);
      if (cur) next = withDependsOn(next, step, cur.depends_on.filter((d) => d !== dep));
    }
    setError("");
    onChange(next);
  };

  const names = new Set(list.map((s) => s.name));
  const edges: Edge[] = list.flatMap((s) =>
    s.depends_on
      .filter((d) => names.has(d))
      .map((d) => {
        const id = `${d}->${s.name}`;
        return { id, source: d, target: s.name, selected: selected.has(id) };
      }),
  );
  const nodes: StepNode[] = computeWaves(list).flatMap((wave, x) =>
    wave.map((name, y) => ({
      id: name,
      type: "step",
      position: { x: x * COL, y: y * ROW },
      data: { label: name },
      draggable: false,
    })),
  );

  if (list.length === 0) {
    return <p className="text-sm text-muted-foreground">Name a step to see it in the graph.</p>;
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="h-64 overflow-hidden rounded-panel border bg-background" style={flowStyle}>
        {/* A new layout mounts a new canvas, so fitView frames every step again. */}
        <ReactFlow
          key={nodes.map((n) => `${n.id}@${n.position.x},${n.position.y}`).join(" ")}
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={fitOptions}
          defaultEdgeOptions={edgeOptions}
          proOptions={{ hideAttribution: true }}
          onEdgesChange={onEdgesChange}
          onConnect={(c: Connection) => connect(c.target, c.source)}
          onEdgesDelete={(deleted) => disconnect(deleted.map((e) => ({ step: e.target, dep: e.source })))}
          nodesDraggable={false}
          panOnScroll={false}
          zoomOnScroll={false}
          preventScrolling={false}
        >
          <Background gap={16} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
      <p className="text-xs text-muted-foreground">
        Drag from the right handle of a step to the left handle of a step that depends on it. Select an edge and press
        Backspace to remove it. Steps referenced in templates as steps.name are not drawn.
      </p>
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      {edges.length > 0 && (
        <ul className="flex flex-col gap-1" aria-label="Edges">
          {edges.map((e) => (
            <li key={e.id} className="flex items-center gap-2 font-mono text-xs">
              <span>
                {e.target} depends on {e.source}
              </span>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                aria-label={`Remove edge ${e.source} to ${e.target}`}
                onClick={() => disconnect([{ step: e.target, dep: e.source }])}
              >
                <X className="h-3.5 w-3.5" aria-hidden />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <label htmlFor="edge-step" className="sr-only">
          Step
        </label>
        <select id="edge-step" className={selectClass} value={from} onChange={(e) => setFrom(e.target.value)}>
          <option value="">Step</option>
          {list.map((s) => (
            <option key={s.name} value={s.name}>
              {s.name}
            </option>
          ))}
        </select>
        <span className="text-muted-foreground">depends on</span>
        <label htmlFor="edge-dep" className="sr-only">
          Depends on
        </label>
        <select id="edge-dep" className={selectClass} value={to} onChange={(e) => setTo(e.target.value)}>
          <option value="">Step</option>
          {list.map((s) => (
            <option key={s.name} value={s.name}>
              {s.name}
            </option>
          ))}
        </select>
        <Button type="button" size="sm" variant="secondary" disabled={!from || !to} onClick={() => connect(from, to)}>
          Add edge
        </Button>
      </div>
    </div>
  );
}
