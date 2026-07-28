import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts"
import { chartTheme } from "../../lib/chart-theme"
import type { TimeSeriesBucket } from "../../lib/api"

export function RequestRateChart({ data }: { data: TimeSeriesBucket[] }) {
  const chartData = data.map((b) => ({
    time: new Date(b.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    success: b.requests - b.errors - b.partials,
    partials: b.partials,
    errors: b.errors,
  }))

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Request rate
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <AreaChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
          <XAxis dataKey="time" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} width={40} />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
            labelStyle={{ color: chartTheme.colors.axisText }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Area type="monotone" dataKey="success" stackId="1" stroke={chartTheme.colors.success} fill={chartTheme.colors.success} fillOpacity={0.3} strokeWidth={1.5} />
          <Area type="monotone" dataKey="partials" stackId="1" stroke={chartTheme.colors.warning} fill={chartTheme.colors.warning} fillOpacity={0.3} strokeWidth={1.5} />
          <Area type="monotone" dataKey="errors" stackId="1" stroke={chartTheme.colors.error} fill={chartTheme.colors.error} fillOpacity={0.3} strokeWidth={1.5} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
