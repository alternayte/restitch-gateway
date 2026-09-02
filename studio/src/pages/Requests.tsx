import { useState } from "react"
import { usePoll } from "../hooks/usePoll"
import { api, type RequestRecord, type StepRecord } from "../lib/api"
import { ChevronDown, ChevronRight } from "lucide-react"
import PollError from "../components/PollError"
import { RequestFilters } from "../components/filters/RequestFilters"
import { TimelineWaterfall } from "../components/waterfall/TimelineWaterfall"
import { StepDetailPanel } from "../components/waterfall/StepDetailPanel"
import { RequestSummary } from "../components/waterfall/RequestSummary"

function statusColor(code: number) {
  if (code < 300) return "bg-success/15 text-success"
  if (code < 500) return "bg-warning/15 text-warning"
  return "bg-error/15 text-error"
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60_000) return `${Math.floor(diff / 1000)}s ago`
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`
  return new Date(iso).toLocaleTimeString()
}

export default function Requests() {
  const [limit, setLimit] = useState(100)
  const [composition, setComposition] = useState("")
  const [statusFilter, setStatusFilter] = useState("")
  const [durationFilter, setDurationFilter] = useState("")
  const [partialOnly, setPartialOnly] = useState(false)

  const { data: compositions } = usePoll(() => api.compositions(), 10000)
  const { data: requests, error, refresh } = usePoll(() => api.requests(limit), 3000)
  const [expanded, setExpanded] = useState<Set<number>>(new Set())
  const [selectedStep, setSelectedStep] = useState<StepRecord | null>(null)

  const compNames = (compositions || []).map((c) => c.name)

  const toggle = (i: number) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })
    setSelectedStep(null)
  }

  // Client-side filtering (backend also filters if params sent)
  const filtered = (requests || []).filter((r) => {
    if (composition && r.composition !== composition) return false
    if (statusFilter === "2xx" && (r.status < 200 || r.status >= 300)) return false
    if (statusFilter === "4xx" && (r.status < 400 || r.status >= 500)) return false
    if (statusFilter === "5xx" && r.status < 500) return false
    if (durationFilter && r.duration_ms < Number(durationFilter)) return false
    if (partialOnly && !r.partial) return false
    return true
  })

  if (!requests) {
    if (error) {
      return <PollError message={error.message} onRetry={refresh} />
    }
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
        <div className="mt-8 space-y-2">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="h-10 bg-surface-1 rounded-lg animate-pulse" />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8 flex items-end justify-between">
        <div>
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
            Explorer
          </div>
          <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
            Requests
          </h1>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-[12px] text-ink-muted">Limit</span>
          <select
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
            className="bg-surface-1 border border-hairline rounded-lg px-2.5 py-1 text-[12px] text-ink outline-none"
          >
            <option value={50}>50</option>
            <option value={100}>100</option>
            <option value={500}>500</option>
          </select>
        </div>
      </div>

      <RequestFilters
        compositions={compNames}
        composition={composition}
        onCompositionChange={setComposition}
        statusFilter={statusFilter}
        onStatusChange={setStatusFilter}
        durationFilter={durationFilter}
        onDurationChange={setDurationFilter}
        partialOnly={partialOnly}
        onPartialChange={setPartialOnly}
      />

      {filtered.length === 0 ? (
        <div className="bg-surface-1 border border-hairline rounded-xl p-12 text-center">
          <div className="text-[20px] font-semibold text-ink mb-2">
            No requests recorded
          </div>
          <p className="text-[14px] text-ink-muted leading-[1.5]">
            {requests.length === 0
              ? "Send traffic to the gateway to see requests appear here in real time."
              : "No requests match the current filters."}
          </p>
        </div>
      ) : (
        <div className="bg-surface-1 rounded-xl border border-hairline overflow-hidden">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="border-b border-hairline-soft">
                <th className="w-8" />
                <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Time</th>
                <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Composition</th>
                <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Route</th>
                <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Status</th>
                <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Duration</th>
                <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Partial</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((req, i) => (
                <RequestRow
                  key={req.id || i}
                  req={req}
                  isExpanded={expanded.has(i)}
                  onToggle={() => toggle(i)}
                  selectedStep={expanded.has(i) ? selectedStep : null}
                  onStepClick={setSelectedStep}
                  onCloseStep={() => setSelectedStep(null)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function RequestRow({
  req,
  isExpanded,
  onToggle,
  selectedStep,
  onStepClick,
  onCloseStep,
}: {
  req: RequestRecord
  isExpanded: boolean
  onToggle: () => void
  selectedStep: StepRecord | null
  onStepClick: (step: StepRecord) => void
  onCloseStep: () => void
}) {
  return (
    <>
      <tr
        onClick={onToggle}
        className={`border-t border-hairline-soft cursor-pointer transition-colors ${
          isExpanded ? "bg-surface-2" : "hover:bg-surface-2"
        }`}
      >
        <td className="pl-3 py-2.5">
          {req.steps?.length > 0 && (
            isExpanded
              ? <ChevronDown size={14} className="text-ink-subtle" />
              : <ChevronRight size={14} className="text-ink-subtle" />
          )}
        </td>
        <td className="px-4 py-2.5 text-ink-subtle tabular-nums">
          {relativeTime(req.time)}
        </td>
        <td className="px-4 py-2.5 font-medium text-ink">{req.composition}</td>
        <td className="px-4 py-2.5 font-mono text-[12px] text-ink-muted">
          <span className="text-ink-subtle">{req.method}</span>{" "}
          {req.path}
        </td>
        <td className="px-4 py-2.5 text-right">
          <span className={`inline-block px-2 py-0.5 rounded text-[11px] font-semibold tabular-nums ${statusColor(req.status)}`}>
            {req.status}
          </span>
        </td>
        <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums font-mono text-[12px]">
          {req.duration_ms.toFixed(1)}ms
        </td>
        <td className="px-4 py-2.5 text-right">
          {req.partial && (
            <span className="inline-block px-2 py-0.5 rounded text-[11px] font-semibold bg-warning/15 text-warning">
              partial
            </span>
          )}
        </td>
      </tr>
      {isExpanded && req.steps?.length > 0 && (
        <tr>
          <td colSpan={7} className="bg-canvas px-6 py-4 border-t border-hairline-soft">
            <RequestSummary req={req} />
            <TimelineWaterfall steps={req.steps} totalDuration={req.duration_ms} onStepClick={onStepClick} />
            {selectedStep && <StepDetailPanel step={selectedStep} onClose={onCloseStep} />}
          </td>
        </tr>
      )}
    </>
  )
}
