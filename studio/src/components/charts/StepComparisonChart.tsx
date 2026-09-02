import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts"
import { chartTheme, stepColors } from "../../lib/chart-theme"
import type { TimeSeriesBucket } from "../../lib/api"

export function StepComparisonChart({ data, stepNames }: { data: TimeSeriesBucket[]; stepNames: string[] }) {
  const chartData = data.map((b) => {
    const point: Record<string, number | string> = {
      time: new Date(b.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    }
    for (const name of stepNames) {
      point[name] = b.step_metrics?.[name]?.avg_ms ?? 0
    }
    return point
  })

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Step latency over time
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
          <XAxis dataKey="time" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} width={50} unit="ms" />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {stepNames.map((name, i) => (
            <Line key={name} type="monotone" dataKey={name} stroke={stepColors[i % stepColors.length]} strokeWidth={1.5} dot={false} />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
