import { useState } from "react"
import { usePoll } from "../hooks/usePoll"
import { api, type CompositionInfo } from "../lib/api"
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"

export default function Compositions() {
  const { data: compositions } = usePoll(() => api.compositions(), 10000)
  const [selected, setSelected] = useState<string | null>(null)

  if (!compositions) {
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
      </div>
    )
  }

  const detail = selected ? compositions.find((c) => c.name === selected) : null

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Routes
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          Compositions
        </h1>
      </div>

      <div className="bg-surface-1 rounded-xl border border-hairline overflow-hidden mb-8">
        <table className="w-full text-[13px]">
          <thead>
            <tr className="border-b border-hairline-soft">
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Name</th>
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Route</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Steps</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Waves</th>
              <th className="px-4 py-2.5 text-center font-medium text-ink-muted">Public</th>
            </tr>
          </thead>
          <tbody>
            {compositions.map((c) => (
              <tr
                key={c.name}
                onClick={() => setSelected(c.name === selected ? null : c.name)}
                className={`border-t border-hairline-soft cursor-pointer transition-colors ${
                  c.name === selected ? "bg-surface-2" : "hover:bg-surface-2"
                }`}
              >
                <td className="px-4 py-2.5 font-medium text-ink">{c.name}</td>
                <td className="px-4 py-2.5 font-mono text-[12px] text-ink-muted">
                  <span className="text-accent font-semibold">{c.method}</span> {c.path}
                </td>
                <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums">{c.steps.length}</td>
                <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums">{c.waves.length}</td>
                <td className="px-4 py-2.5 text-center">
                  {c.public && (
                    <span className="inline-block px-2 py-0.5 rounded text-[11px] font-semibold bg-success/15 text-success">
                      public
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {detail && <CompositionDetail comp={detail} />}
    </div>
  )
}

function CompositionDetail({ comp }: { comp: CompositionInfo }) {
  const [tab, setTab] = useState<"graph" | "steps">("graph")

  return (
    <div>
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-3">
        {comp.name}
      </div>

      <div className="flex gap-1 mb-4">
        {(["graph", "steps"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-3 py-1.5 rounded-lg text-[13px] font-medium transition-colors ${
              tab === t ? "bg-surface-2 text-ink" : "text-ink-muted hover:text-ink hover:bg-surface-1"
            }`}
          >
            {t === "graph" ? "DAG" : "Steps"}
          </button>
        ))}
      </div>

      {tab === "graph" && <DAGView comp={comp} />}
      {tab === "steps" && <StepsTable comp={comp} />}
    </div>
  )
}

function DAGView({ comp }: { comp: CompositionInfo }) {
  const nodes: Node[] = comp.steps.map((step) => {
    const wave = comp.waves.findIndex((w) => w.includes(step.name))
    const inWave = comp.waves[wave]?.indexOf(step.name) ?? 0

    return {
      id: step.name,
      position: { x: wave * 260, y: inWave * 110 },
      data: { label: step.name },
      style: {
        background: "#161618",
        border: "1px solid rgba(178,182,189,0.12)",
        borderRadius: "12px",
        padding: "12px 16px",
        color: "#fff",
        fontSize: "13px",
        fontWeight: 500,
      },
    }
  })

  const edges: Edge[] = comp.steps.flatMap((step) =>
    step.depends_on.map((dep) => ({
      id: `${dep}-${step.name}`,
      source: dep,
      target: step.name,
      style: { stroke: "rgba(178,182,189,0.3)" },
    }))
  )

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl overflow-hidden" style={{ height: 400 }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        fitView
        proOptions={{ hideAttribution: true }}
      >
        <Background color="rgba(178,182,189,0.05)" />
        <Controls
          style={{ background: "#222225", border: "1px solid rgba(178,182,189,0.12)", borderRadius: "8px" }}
        />
      </ReactFlow>
    </div>
  )
}

function StepsTable({ comp }: { comp: CompositionInfo }) {
  return (
    <div className="bg-surface-1 rounded-xl border border-hairline overflow-hidden">
      <table className="w-full text-[13px]">
        <thead>
          <tr className="border-b border-hairline-soft">
            <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Name</th>
            <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Upstream</th>
            <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Method</th>
            <th className="px-4 py-2.5 text-center font-medium text-ink-muted">Optional</th>
            <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Depends on</th>
          </tr>
        </thead>
        <tbody>
          {comp.steps.map((s) => (
            <tr key={s.name} className="border-t border-hairline-soft">
              <td className="px-4 py-2.5 font-medium text-ink">{s.name}</td>
              <td className="px-4 py-2.5 text-ink-muted">{s.upstream}</td>
              <td className="px-4 py-2.5 font-mono text-[12px] text-accent">{s.method}</td>
              <td className="px-4 py-2.5 text-center">
                {s.optional && <span className="text-warning text-[11px] font-semibold">optional</span>}
              </td>
              <td className="px-4 py-2.5 text-ink-subtle text-[12px]">
                {s.depends_on?.join(", ") || "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
