import { AreaChart, Area, ResponsiveContainer } from "recharts"
import { chartTheme } from "../../lib/chart-theme"

interface SparklineCardProps {
  label: string
  value: string | number
  data: { value: number }[]
  color?: string
  accent?: boolean
}

export function SparklineCard({ label, value, data, color, accent }: SparklineCardProps) {
  const fillColor = color || (accent ? chartTheme.colors.warning : chartTheme.colors.accent)

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5 relative overflow-hidden">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
        {label}
      </div>
      <div className={`text-[28px] font-semibold leading-[1.17] tracking-[-0.6px] tabular-nums ${
        accent ? "text-warning" : "text-ink"
      }`}>
        {value}
      </div>
      {data.length > 1 && (
        <div className="absolute bottom-0 left-0 right-0 h-12 opacity-30">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
              <Area
                type="monotone"
                dataKey="value"
                stroke={fillColor}
                fill={fillColor}
                strokeWidth={1.5}
                fillOpacity={0.3}
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}
