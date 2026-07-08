import { usePoll } from "../hooks/usePoll"
import { api } from "../lib/api"

export default function Dashboard() {
  const { data: stats } = usePoll(() => api.stats(), 5000)

  if (!stats) {
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
        <div className="grid grid-cols-4 gap-4 mt-8">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-24 bg-surface-1 rounded-xl border border-hairline animate-pulse" />
          ))}
        </div>
      </div>
    )
  }

  const errorRate = stats.total_requests > 0
    ? ((stats.total_errors / stats.total_requests) * 100).toFixed(1) + "%"
    : "—"

  const compositions = Object.entries(stats.per_composition)

  return (
    <div className="p-8 max-w-[1280px]">
      {/* Eyebrow + headline */}
      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Overview
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          Dashboard
        </h1>
      </div>

      {/* Stat cards — surface-1 lift with hairline */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-10">
        <StatCard label="Total requests" value={stats.total_requests} />
        <StatCard label="Error rate" value={errorRate} accent={stats.total_errors > 0} />
        <StatCard label="Partial responses" value={stats.partial_responses} />
        <StatCard label="Compositions" value={compositions.length} />
      </div>

      {/* Composition table */}
      {compositions.length > 0 && (
        <div>
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-3">
            Per composition
          </div>
          <div className="bg-surface-1 rounded-xl border border-hairline overflow-hidden">
            <table className="w-full text-[13px]">
              <thead>
                <tr className="border-b border-hairline-soft">
                  <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Name</th>
                  <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Requests</th>
                  <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Errors</th>
                  <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Avg</th>
                  <th className="px-4 py-2.5 text-right font-medium text-ink-muted">P95</th>
                </tr>
              </thead>
              <tbody>
                {compositions.map(([name, s]) => (
                  <tr key={name} className="border-t border-hairline-soft hover:bg-surface-2 transition-colors">
                    <td className="px-4 py-2.5 font-medium text-ink">{name}</td>
                    <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums">{s.count}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums">
                      <span className={s.errors > 0 ? "text-error" : "text-ink-muted"}>
                        {s.errors}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums font-mono text-[12px]">
                      {s.avg_ms.toFixed(1)}ms
                    </td>
                    <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums font-mono text-[12px]">
                      {s.p95_ms.toFixed(1)}ms
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Empty state */}
      {stats.total_requests === 0 && (
        <div className="bg-surface-1 border border-hairline rounded-xl p-12 text-center">
          <div className="text-[20px] font-semibold text-ink mb-2">
            No traffic yet
          </div>
          <p className="text-[14px] text-ink-muted leading-[1.5] max-w-md mx-auto">
            Point requests at the gateway to see live metrics here.
            The dashboard updates every 5 seconds.
          </p>
        </div>
      )}
    </div>
  )
}

function StatCard({ label, value, accent }: { label: string; value: string | number; accent?: boolean }) {
  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
        {label}
      </div>
      <div className={`text-[28px] font-semibold leading-[1.17] tracking-[-0.6px] tabular-nums ${
        accent ? "text-warning" : "text-ink"
      }`}>
        {value}
      </div>
    </div>
  )
}
