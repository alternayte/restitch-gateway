import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend, Cell } from "recharts"
import { chartTheme, stepColors } from "../../lib/chart-theme"
import type { StepAggregate } from "../../lib/api"

export function StepBreakdownChart({ steps }: { steps: StepAggregate[] }) {
  const sorted = [...steps].sort((a, b) => b.avg_ms - a.avg_ms)
  const data = sorted.map((s, i) => ({
    name: s.name,
    avg: s.avg_ms,
    p95: s.p95_ms,
    color: stepColors[i % stepColors.length],
  }))

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Step latency breakdown
      </div>
      <ResponsiveContainer width="100%" height={Math.max(steps.length * 40 + 20, 120)}>
        <BarChart data={data} layout="vertical" margin={{ top: 0, right: 20, bottom: 0, left: 80 }}>
          <XAxis type="number" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} unit="ms" />
          <YAxis type="category" dataKey="name" tick={{ fill: chartTheme.colors.axisText, fontSize: 12 }} axisLine={false} tickLine={false} width={80} />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
            formatter={(value: any) => [`${Number(value).toFixed(1)}ms`]}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Bar dataKey="avg" name="Avg" radius={[0, 4, 4, 0]}>
            {data.map((d, i) => <Cell key={i} fill={d.color} fillOpacity={0.7} />)}
          </Bar>
          <Bar dataKey="p95" name="P95" radius={[0, 4, 4, 0]}>
            {data.map((d, i) => <Cell key={i} fill={d.color} fillOpacity={0.3} />)}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
