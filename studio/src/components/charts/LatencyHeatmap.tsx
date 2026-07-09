import { useMemo } from "react"
import type { TimeSeriesBucket } from "../../lib/api"

const LATENCY_LABELS = ["0-10ms", "10-50ms", "50-100ms", "100-250ms", "250-500ms", "500ms-1s", "1-5s", "5s+"]

export function LatencyHeatmap({ data }: { data: TimeSeriesBucket[] }) {
  const { grid, maxCount, times } = useMemo(() => {
    let max = 0
    const g: number[][] = []
    const t: string[] = []
    for (const bucket of data) {
      const buckets = bucket.latency_buckets || []
      const row = LATENCY_LABELS.map((_, i) => buckets[i] || 0)
      for (const v of row) {
        if (v > max) max = v
      }
      g.push(row)
      t.push(new Date(bucket.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }))
    }
    return { grid: g, maxCount: max, times: t }
  }, [data])

  if (data.length === 0) return null

  const cellW = Math.max(Math.min(Math.floor(800 / data.length), 40), 8)
  const cellH = 24
  const labelW = 70
  const svgW = labelW + data.length * cellW
  const svgH = LATENCY_LABELS.length * cellH + 30

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Latency distribution
      </div>
      <div className="overflow-x-auto">
        <svg width={svgW} height={svgH} className="text-ink-muted">
          {LATENCY_LABELS.map((label, row) => (
            <text key={label} x={labelW - 6} y={row * cellH + cellH / 2 + 4} textAnchor="end" fontSize={10} fill="currentColor">
              {label}
            </text>
          ))}
          {grid.map((cols, col) =>
            cols.map((count, row) => {
              const intensity = maxCount > 0 ? count / maxCount : 0
              return (
                <rect
                  key={`${col}-${row}`}
                  x={labelW + col * cellW}
                  y={row * cellH}
                  width={cellW - 1}
                  height={cellH - 1}
                  rx={3}
                  fill={`oklch(0.75 ${0.18 * intensity} 65 / ${0.1 + 0.8 * intensity})`}
                >
                  <title>{`${times[col]}: ${count} requests (${LATENCY_LABELS[row]})`}</title>
                </rect>
              )
            })
          )}
          {times.filter((_, i) => i % Math.max(1, Math.floor(times.length / 8)) === 0).map((t, i, arr) => (
            <text
              key={t}
              x={labelW + (i * (data.length / arr.length)) * cellW + cellW / 2}
              y={LATENCY_LABELS.length * cellH + 16}
              textAnchor="middle"
              fontSize={10}
              fill="currentColor"
            >
              {t}
            </text>
          ))}
        </svg>
      </div>
    </div>
  )
}
