import {
  Background,
  BaseEdge,
  Controls,
  getBezierPath,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Connection,
  type EdgeChange,
  type EdgeProps,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { X } from "lucide-react";
import { useState, type CSSProperties } from "react";
import { Button } from "@/components/ui/button";
import { inferredFor, stepEdges, type Inferred, type Named } from "@/features/resources/step-edges";
import { computeWaves, dependsOnTransitively } from "@/lib/dag";

/** Step is one entry of a composition's steps list; fields other than name and depends_on pass through. */
export type Step = Record<string, unknown> & { name?: unknown; depends_on?: unknown };


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

/**
 * WaveEdgeView draws an edge between adjacent waves as a bezier. An edge that
 * skips a wave would run straight under the steps in between, so it bows
 * below them.
 */
function WaveEdgeView({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, style, markerEnd }: EdgeProps) {
  const [path] =
    targetX - sourceX > COL
      ? [`M ${sourceX},${sourceY} C ${sourceX + 80},${sourceY + ROW} ${targetX - 80},${targetY + ROW} ${targetX},${targetY}`]
      : getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition });
  return <BaseEdge path={path} style={style} markerEnd={markerEnd} />;
}

const edgeTypes = { wave: WaveEdgeView };

const fitOptions = { padding: 0.2, maxZoom: 1 };

const edgeOptions = { type: "wave", markerEnd: { type: MarkerType.ArrowClosed, color: "var(--muted-foreground)" } };

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
 * a cycle is refused. inferred holds the template references Studio's
 * compiler found; they are drawn dashed, count in the cycle check and cannot
 * be removed here. problem is why the gateway refuses the steps, if it does.
 */
export function StepGraph({
  steps,
  inferred = {},
  problem = "",
  onChange,
}: {
  steps: Step[];
  inferred?: Inferred;
  problem?: string;
  onChange: (next: Step[]) => void;
}) {
  const list = named(steps);
  const names = new Set(list.map((s) => s.name));
  // What each step waits for at runtime: depends_on plus template references.
  const runtime = list.map((s) => ({ name: s.name, depends_on: [...s.depends_on, ...inferredFor(s, inferred, names)] }));
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
    if (dependsOnTransitively(runtime, dep, step)) {
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

  const edges = stepEdges(list, inferred, selected);
  const nodes: StepNode[] = computeWaves(runtime).flatMap((wave, x) =>
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
          edgeTypes={edgeTypes}
          fitView
          fitViewOptions={fitOptions}
          defaultEdgeOptions={edgeOptions}
          proOptions={{ hideAttribution: true }}
          onEdgesChange={onEdgesChange}
          onConnect={(c: Connection) => connect(c.target, c.source)}
          onEdgesDelete={(deleted) =>
            disconnect(deleted.filter((e) => e.deletable !== false).map((e) => ({ step: e.target, dep: e.source })))
          }
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
        Backspace to remove it. Dashed edges come from template references such as {"{{ steps.name.body }}"}; edit the
        template to change them.
      </p>
      {problem && (
        <p aria-live="polite" className="text-xs text-destructive">
          The gateway refuses these steps: {problem}
        </p>
      )}
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      {edges.length > 0 && (
        <ul className="flex flex-col gap-1" aria-label="Edges">
          {edges.map((e) =>
            e.deletable === false ? (
              <li key={e.id} className="flex h-8 items-center gap-2 font-mono text-xs text-muted-foreground">
                <span aria-hidden className="w-4 border-t border-dashed border-current" />
                <span>
                  {e.target} uses {e.source} in a template
                </span>
              </li>
            ) : (
              <li key={e.id} className="flex items-center gap-2 font-mono text-xs">
                <span aria-hidden className="w-4 border-t border-current" />
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
            ),
          )}
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
