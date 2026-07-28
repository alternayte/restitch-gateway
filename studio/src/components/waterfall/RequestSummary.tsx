import type { RequestRecord } from "../../lib/api"
import { stepColors } from "../../lib/chart-theme"

export function RequestSummary({ req }: { req: RequestRecord }) {
  const stepTime = req.steps?.reduce((sum, s) => sum + s.duration_ms, 0) ?? 0
  const overhead = Math.max(req.duration_ms - stepTime, 0)

  return (
    <div className="flex items-center gap-6 mb-3 text-[12px]">
      <StepDonut steps={req.steps || []} totalDuration={req.duration_ms} />
      <div>
        <span className="text-ink-subtle">Total: </span>
        <span className="font-mono tabular-nums text-ink font-medium">{req.duration_ms.toFixed(1)}ms</span>
      </div>
      <div>
        <span className="text-ink-subtle">Gateway overhead: </span>
        <span className="font-mono tabular-nums text-ink-muted">{overhead.toFixed(1)}ms</span>
      </div>
      <div>
        <span className="text-ink-subtle">Steps: </span>
        <span className="text-ink">{req.steps?.length ?? 0}</span>
      </div>
    </div>
  )
}

function StepDonut({ steps, totalDuration }: { steps: { name: string; duration_ms: number }[]; totalDuration: number }) {
  if (steps.length === 0 || totalDuration <= 0) return null

  const size = 32
  const strokeWidth = 5
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius

  let offset = 0
  const segments = steps.map((s, i) => {
    const pct = s.duration_ms / totalDuration
    const dashLength = pct * circumference
    const seg = { offset, dashLength, color: stepColors[i % stepColors.length], name: s.name }
    offset += dashLength
    return seg
  })

  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="shrink-0 -rotate-90">
      <circle cx={size / 2} cy={size / 2} r={radius} fill="none" stroke="rgba(178,182,189,0.08)" strokeWidth={strokeWidth} />
      {segments.map((seg, i) => (
        <circle
          key={i}
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={seg.color}
          strokeWidth={strokeWidth}
          strokeDasharray={`${seg.dashLength} ${circumference - seg.dashLength}`}
          strokeDashoffset={-seg.offset}
          strokeLinecap="round"
        >
          <title>{`${seg.name}: ${((seg.dashLength / circumference) * 100).toFixed(0)}%`}</title>
        </circle>
      ))}
    </svg>
  )
}
