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

interface StepNodeData extends Record<string, unknown> {
  label: string
  upstream: string
  method: string
  optional: boolean
  timeoutMs: number
  wave: number
  overlayBorder: string
  durationLabel: string
}

const methodColors: Record<string, string> = {
  GET: "bg-blue-500/15 text-blue-400",
  POST: "bg-green-500/15 text-green-400",
  PUT: "bg-amber-500/15 text-amber-400",
  DELETE: "bg-red-500/15 text-red-400",
}

function EnhancedStepNode({ data }: { data: StepNodeData }) {
  return (
    <div className={`px-3 py-2.5 min-w-[180px] transition-all ${data.overlayBorder || ""}`}>
      <div className="flex items-center gap-2">
        <span className="font-medium text-[13px]">{data.label}</span>
        {data.optional && (
          <span className="text-[9px] font-semibold tracking-[0.5px] uppercase px-1.5 py-0.5 rounded bg-warning/15 text-warning">opt</span>
        )}
      </div>
      <div className="flex items-center gap-2 mt-1.5">
        <span className={`text-[10px] font-bold px-1.5 py-0.5 rounded ${methodColors[data.method] || "bg-ink-subtle/15 text-ink-muted"}`}>
          {data.method}
        </span>
        <span className="text-[11px] text-ink-muted truncate">{data.upstream}</span>
      </div>
      <div className="flex items-center gap-2 mt-1 text-[10px] text-ink-subtle">
        <span>wave {data.wave + 1}</span>
        {data.timeoutMs > 0 && <span>· {data.timeoutMs}ms timeout</span>}
      </div>
      {data.durationLabel && (
        <div className="mt-1.5 text-[11px] font-mono tabular-nums text-ink-subtle">
          {data.durationLabel}
        </div>
      )}
    </div>
  )
}

const dagNodeTypes = { enhanced: EnhancedStepNode }

function DAGView({ comp }: { comp: CompositionInfo }) {
  const [showOverlay, setShowOverlay] = useState(false)
  const { data: requests } = usePoll(() => api.requests(20), 3000)

  const latestRequest = requests?.find((r) => r.composition === comp.name)

  const traceSteps = showOverlay && latestRequest ? latestRequest.steps : []

  const nodes: Node<StepNodeData>[] = comp.steps.map((step) => {
    const wave = comp.waves.findIndex((w) => w.includes(step.name))
    const inWave = comp.waves[wave]?.indexOf(step.name) ?? 0
    const traceStep = traceSteps.find((s) => s.name === step.name)

    let overlayBorder = ""
    let durationLabel = ""
    if (traceStep) {
      if (traceStep.status === "success") overlayBorder = "ring-2 ring-success/50"
      else if (traceStep.status === "failed") overlayBorder = "ring-2 ring-error/50"
      else overlayBorder = "ring-1 ring-ink-subtle/20 opacity-50"
      durationLabel = `${traceStep.duration_ms.toFixed(1)}ms`
    }

    return {
      id: step.name,
      position: { x: wave * 280, y: inWave * 130 },
      data: { label: step.name, upstream: step.upstream, method: step.method, optional: step.optional, timeoutMs: step.timeout_ms, wave, overlayBorder, durationLabel },
      type: "enhanced",
      style: {
        background: "#161618",
        border: "1px solid rgba(178,182,189,0.12)",
        borderRadius: "12px",
        padding: "0",
        color: "#fff",
        fontSize: "13px",
      },
    }
  })

  const edges: Edge[] = [
    ...comp.steps.flatMap((step) =>
      (step.depends_on || []).map((dep) => ({
        id: `explicit-${dep}-${step.name}`,
        source: dep,
        target: step.name,
        style: { stroke: showOverlay ? "rgba(74,222,128,0.5)" : "rgba(178,182,189,0.4)", strokeWidth: 2 },
        animated: showOverlay,
      }))
    ),
    ...comp.steps.flatMap((step) =>
      (step.inferred_deps || []).map((dep) => ({
        id: `inferred-${dep}-${step.name}`,
        source: dep,
        target: step.name,
        style: { stroke: showOverlay ? "rgba(74,222,128,0.3)" : "rgba(178,182,189,0.25)", strokeWidth: 1.5, strokeDasharray: "6 3" },
        animated: showOverlay,
        label: showOverlay ? "" : "inferred",
        labelStyle: { fontSize: 9, fill: "rgba(178,182,189,0.4)" },
      }))
    ),
  ]

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle">
          Execution DAG
        </div>
        {latestRequest && (
          <button
            onClick={() => setShowOverlay(!showOverlay)}
            className={`text-[12px] px-3 py-1 rounded-lg border transition-colors ${
              showOverlay ? "bg-surface-2 border-hairline text-ink" : "border-hairline-soft text-ink-muted hover:text-ink"
            }`}
          >
            {showOverlay ? "Hide trace" : "Show latest trace"}
          </button>
        )}
      </div>
      <div className="bg-surface-1 border border-hairline rounded-xl overflow-hidden" style={{ height: 450 }}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          fitView
          proOptions={{ hideAttribution: true }}
          nodeTypes={dagNodeTypes}
        >
          <Background color="rgba(178,182,189,0.05)" />
          <Controls style={{ background: "#222225", border: "1px solid rgba(178,182,189,0.12)", borderRadius: "8px" }} />
        </ReactFlow>
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
