import type { StepRecord } from "../../lib/api"

function stepStatusColor(status: string) {
  if (status === "success") return "bg-success"
  if (status === "failed") return "bg-error"
  return "bg-ink-subtle/40"
}

export function TimelineWaterfall({
  steps,
  totalDuration,
  onStepClick,
}: {
  steps: StepRecord[]
  totalDuration: number
  onStepClick?: (step: StepRecord) => void
}) {
  if (steps.length === 0) return null

  const sorted = [...steps].sort((a, b) => a.start_offset_ms - b.start_offset_ms || a.wave - b.wave)
  const maxTime = totalDuration || Math.max(...steps.map((s) => s.start_offset_ms + s.duration_ms), 1)

  return (
    <div className="space-y-1">
      <div className="text-[11px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
        Timeline
      </div>
      <div className="relative">
        <div className="absolute left-28 top-0 bottom-0 w-px bg-hairline-soft" />
        {sorted.map((step) => {
          const leftPct = (step.start_offset_ms / maxTime) * 100
          const widthPct = Math.max((step.duration_ms / maxTime) * 100, 1)

          return (
            <div
              key={step.name}
              className={`flex items-center gap-3 h-7 ${onStepClick ? "cursor-pointer hover:bg-surface-2 rounded" : ""}`}
              onClick={() => onStepClick?.(step)}
            >
              <div className="w-28 text-[12px] font-medium text-ink truncate pl-1">{step.name}</div>
              <div className="flex-1 h-5 relative">
                <div
                  className={`absolute top-0.5 h-4 rounded ${stepStatusColor(step.status)} opacity-80`}
                  style={{ left: `${leftPct}%`, width: `${widthPct}%`, minWidth: 4 }}
                />
              </div>
              <div className="w-52 text-right text-[11px] text-ink-muted font-mono tabular-nums whitespace-nowrap pr-1">
                +{step.start_offset_ms.toFixed(0)}ms · {step.duration_ms.toFixed(1)}ms
                {step.http_status > 0 ? ` · ${step.http_status}` : ""}
                {step.cached ? " · cached" : ""}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
