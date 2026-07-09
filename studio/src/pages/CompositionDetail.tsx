import { useState } from "react"
import { useParams, Link } from "react-router-dom"
import { usePoll } from "../hooks/usePoll"
import { api, type CompositionInfo } from "../lib/api"
import { ArrowLeft, Copy, Check } from "lucide-react"
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"
import { SparklineCard } from "../components/charts/SparklineCard"
import { RequestRateChart } from "../components/charts/RequestRateChart"
import { LatencyChart } from "../components/charts/LatencyChart"
import { LatencyHeatmap } from "../components/charts/LatencyHeatmap"
import { TimeRangeSelector, type TimeRange } from "../components/charts/TimeRangeSelector"
import { StepBreakdownChart } from "../components/charts/StepBreakdownChart"
import { StepComparisonChart } from "../components/charts/StepComparisonChart"
import { TimelineWaterfall } from "../components/waterfall/TimelineWaterfall"

export default function CompositionDetail() {
  const { name } = useParams<{ name: string }>()
  const { data: compositions } = usePoll(() => api.compositions(), 10000)
  const [tab, setTab] = useState<"metrics" | "graph" | "steps" | "route">("metrics")

  const comp = compositions?.find((c) => c.name === name)

  if (!compositions) {
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
      </div>
    )
  }

  if (!comp) {
    return (
      <div className="p-8">
        <Link to="/compositions" className="flex items-center gap-1.5 text-[13px] text-ink-muted hover:text-ink mb-6">
          <ArrowLeft size={14} /> Back to compositions
        </Link>
        <div className="bg-surface-1 border border-hairline rounded-xl p-12 text-center">
          <div className="text-[20px] font-semibold text-ink mb-2">Not found</div>
          <p className="text-[14px] text-ink-muted">Composition "{name}" does not exist.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="p-8 max-w-[1280px]">
      <Link to="/compositions" className="flex items-center gap-1.5 text-[13px] text-ink-muted hover:text-ink mb-6">
        <ArrowLeft size={14} /> Back to compositions
      </Link>

      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Composition
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          {comp.name}
        </h1>
        <div className="mt-2 font-mono text-[13px] text-ink-muted">
          <span className="text-rs-accent font-semibold">{comp.method}</span> {comp.path}
        </div>
      </div>

      {/* Tab bar */}
      <div className="flex gap-1 mb-6">
        {(["metrics", "graph", "steps", "route"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-3 py-1.5 rounded-lg text-[13px] font-medium transition-colors capitalize ${
              tab === t ? "bg-surface-2 text-ink" : "text-ink-muted hover:text-ink hover:bg-surface-1"
            }`}
          >
            {t === "graph" ? "DAG" : t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {tab === "metrics" && <MetricsTab comp={comp} />}
      {tab === "graph" && <DAGView comp={comp} />}
      {tab === "steps" && <StepsTable comp={comp} />}
      {tab === "route" && <RouteTab comp={comp} />}
    </div>
  )
}

function MetricsTab({ comp }: { comp: CompositionInfo }) {
  const [range, setRange] = useState<TimeRange>("1h")
  const { data: timeseries } = usePoll(() => api.timeseries(range, "1m", comp.name), 30000)
  const { data: stepMetrics } = usePoll(() => api.stepMetrics(comp.name, range), 30000)
  const { data: recentRequests } = usePoll(() => api.requests(20), 5000)

  const compRequests = (recentRequests || []).filter((r) => r.composition === comp.name)
  const ts = timeseries || []

  const totalReqs = ts.reduce((s, b) => s + b.requests, 0)
  const totalErrs = ts.reduce((s, b) => s + b.errors, 0)
  const errorRate = totalReqs > 0 ? ((totalErrs / totalReqs) * 100).toFixed(1) + "%" : "—"
  const avgLatency = ts.length > 0
    ? (ts.reduce((s, b) => s + b.latency_p50, 0) / ts.length).toFixed(1) + "ms"
    : "—"
  const p95 = ts.length > 0
    ? Math.max(...ts.map((b) => b.latency_p95)).toFixed(1) + "ms"
    : "—"

  const stepNames = [...new Set((stepMetrics || []).map((s) => s.name))]

  return (
    <div className="space-y-6">
      <div className="flex justify-end"><TimeRangeSelector value={range} onChange={setRange} /></div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <SparklineCard label="Requests" value={totalReqs} data={ts.map((b) => ({ value: b.requests }))} />
        <SparklineCard label="Error rate" value={errorRate} data={ts.map((b) => ({ value: b.errors }))} accent={totalErrs > 0} />
        <SparklineCard label="Avg latency" value={avgLatency} data={ts.map((b) => ({ value: b.latency_p50 }))} />
        <SparklineCard label="P95 latency" value={p95} data={ts.map((b) => ({ value: b.latency_p95 }))} />
      </div>

      {ts.length > 0 && (
        <>
          <RequestRateChart data={ts} />
          <LatencyChart data={ts} />
          <LatencyHeatmap data={ts} />
        </>
      )}

      {stepMetrics && stepMetrics.length > 0 && (
        <>
          <StepBreakdownChart steps={stepMetrics} />
          {ts.length > 0 && <StepComparisonChart data={ts} stepNames={stepNames} />}
        </>
      )}

      {compRequests.length > 0 && (
        <div>
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-3">Recent traces</div>
          {compRequests.slice(0, 10).map((req) => (
            <div key={req.id} className="mb-3 bg-surface-1 border border-hairline rounded-xl p-4">
              <div className="flex items-center gap-4 text-[12px] mb-2">
                <span className="font-mono text-ink-muted">{new Date(req.time).toLocaleTimeString()}</span>
                <span className={`px-2 py-0.5 rounded text-[11px] font-semibold ${req.status < 300 ? "bg-success/15 text-success" : "bg-error/15 text-error"}`}>{req.status}</span>
                <span className="font-mono text-ink-muted tabular-nums">{req.duration_ms.toFixed(1)}ms</span>
              </div>
              <TimelineWaterfall steps={req.steps || []} totalDuration={req.duration_ms} />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function DAGView({ comp }: { comp: CompositionInfo }) {
  const nodes: Node[] = comp.steps.map((step) => {
    const wave = comp.waves.findIndex((w) => w.includes(step.name))
    const inWave = comp.waves[wave]?.indexOf(step.name) ?? 0

    return {
      id: step.name,
      position: { x: wave * 260, y: inWave * 120 },
      data: { label: step.name, upstream: step.upstream, method: step.method, optional: step.optional },
      type: "default",
      style: {
        background: "#161618",
        border: "1px solid rgba(178,182,189,0.12)",
        borderRadius: "12px",
        padding: "0",
        color: "#fff",
        fontSize: "13px",
        width: 180,
      },
    }
  })

  const edges: Edge[] = comp.steps.flatMap((step) =>
    (step.depends_on || []).map((dep) => ({
      id: `${dep}-${step.name}`,
      source: dep,
      target: step.name,
      style: { stroke: "rgba(178,182,189,0.3)" },
    }))
  )

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl overflow-hidden" style={{ height: 450 }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        fitView
        proOptions={{ hideAttribution: true }}
        nodeTypes={{ default: StepNode }}
      >
        <Background color="rgba(178,182,189,0.05)" />
        <Controls
          style={{ background: "#222225", border: "1px solid rgba(178,182,189,0.12)", borderRadius: "8px" }}
        />
      </ReactFlow>
    </div>
  )
}

function StepNode({ data }: { data: { label: string; upstream: string; method: string; optional: boolean } }) {
  return (
    <div className="px-3 py-2.5">
      <div className="flex items-center gap-2">
        <span className="font-medium text-[13px]">{data.label}</span>
        {data.optional && (
          <span className="text-[9px] font-semibold tracking-[0.5px] uppercase px-1.5 py-0.5 rounded bg-warning/15 text-warning">
            opt
          </span>
        )}
      </div>
      <div className="flex items-center gap-2 mt-1 text-[11px] text-ink-muted">
        <span className="text-rs-accent font-semibold">{data.method}</span>
        <span>{data.upstream}</span>
      </div>
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
            <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Timeout</th>
            <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Depends on</th>
          </tr>
        </thead>
        <tbody>
          {comp.steps.map((s) => (
            <tr key={s.name} className="border-t border-hairline-soft">
              <td className="px-4 py-2.5 font-medium text-ink">{s.name}</td>
              <td className="px-4 py-2.5 text-ink-muted">{s.upstream}</td>
              <td className="px-4 py-2.5 font-mono text-[12px] text-rs-accent">{s.method}</td>
              <td className="px-4 py-2.5 text-center">
                {s.optional && <span className="text-warning text-[11px] font-semibold">optional</span>}
              </td>
              <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums font-mono text-[12px]">
                {s.timeout_ms > 0 ? `${s.timeout_ms}ms` : "—"}
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

function RouteTab({ comp }: { comp: CompositionInfo }) {
  const [copied, setCopied] = useState(false)
  const curlCmd = `curl http://localhost:8080${comp.path}`

  const handleCopy = () => {
    navigator.clipboard.writeText(curlCmd)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="bg-surface-1 rounded-xl border border-hairline p-6 space-y-5">
      <div className="grid grid-cols-3 gap-6">
        <div>
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-1">Method</div>
          <div className="font-mono text-[14px] text-rs-accent font-semibold">{comp.method}</div>
        </div>
        <div>
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-1">Path</div>
          <div className="font-mono text-[14px] text-ink">{comp.path}</div>
        </div>
        <div>
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-1">Access</div>
          <div className="text-[14px] text-ink">
            {comp.public ? (
              <span className="inline-block px-2 py-0.5 rounded text-[11px] font-semibold bg-success/15 text-success">public</span>
            ) : (
              <span className="text-ink-muted">authenticated</span>
            )}
          </div>
        </div>
      </div>

      <div>
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">cURL</div>
        <div className="flex items-center gap-2 bg-canvas border border-hairline rounded-lg px-4 py-3">
          <code className="flex-1 font-mono text-[13px] text-ink select-all">{curlCmd}</code>
          <button onClick={handleCopy} className="p-1 text-ink-subtle hover:text-ink transition-colors">
            {copied ? <Check size={14} className="text-success" /> : <Copy size={14} />}
          </button>
        </div>
      </div>
    </div>
  )
}
