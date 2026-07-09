import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts"
import { chartTheme } from "../../lib/chart-theme"
import type { TimeSeriesBucket } from "../../lib/api"

export function LatencyChart({ data }: { data: TimeSeriesBucket[] }) {
  const chartData = data.map((b) => ({
    time: new Date(b.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    p50: b.latency_p50,
    p95: b.latency_p95,
    p99: b.latency_p99,
  }))

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Latency (ms)
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
          <XAxis dataKey="time" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} width={50} unit="ms" />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
            labelStyle={{ color: chartTheme.colors.axisText }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Line type="monotone" dataKey="p50" stroke={chartTheme.colors.p50} strokeWidth={1.5} dot={false} name="p50" />
          <Line type="monotone" dataKey="p95" stroke={chartTheme.colors.p95} strokeWidth={2} dot={false} name="p95" />
          <Line type="monotone" dataKey="p99" stroke={chartTheme.colors.p99} strokeWidth={1.5} dot={false} strokeDasharray="4 2" name="p99" />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
