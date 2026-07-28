import { useNavigate } from "react-router-dom"
import { usePoll } from "../hooks/usePoll"
import { usePreferences } from "../hooks/usePreferences"
import { api } from "../lib/api"
import { SparklineCard } from "../components/charts/SparklineCard"
import { RequestRateChart } from "../components/charts/RequestRateChart"
import { LatencyChart } from "../components/charts/LatencyChart"
import { LatencyHeatmap } from "../components/charts/LatencyHeatmap"
import { TimeRangeSelector } from "../components/charts/TimeRangeSelector"

export default function Dashboard() {
  const { prefs, setDefaultTimeRange } = usePreferences()
  const range = prefs.defaultTimeRange
  const setRange = setDefaultTimeRange
  const navigate = useNavigate()
  const { data: stats } = usePoll(() => api.stats(), 5000)
  const { data: upstreams } = usePoll(() => api.upstreams(), 10000)
  const { data: timeseries } = usePoll(() => api.timeseries(range, "1m"), 30000)

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

  // Build sparkline data from timeseries
  const requestSparkline = (timeseries || []).map((b) => ({ value: b.requests }))
  const errorSparkline = (timeseries || []).map((b) => ({ value: b.errors }))
  const partialSparkline = (timeseries || []).map((b) => ({ value: b.partials }))

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8 flex items-end justify-between">
        <div>
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
            Overview
          </div>
          <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
            Dashboard
          </h1>
        </div>
        <TimeRangeSelector value={range} onChange={setRange} />
      </div>

      {/* Sparkline stat cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <SparklineCard label="Total requests" value={stats.total_requests} data={requestSparkline} />
        <SparklineCard label="Error rate" value={errorRate} data={errorSparkline} accent={stats.total_errors > 0} />
        <SparklineCard label="Partial responses" value={stats.partial_responses} data={partialSparkline} />
        <SparklineCard label="Compositions" value={compositions.length} data={[]} />
      </div>

      {/* Charts */}
      {timeseries && timeseries.length > 0 && (
        <div className="space-y-6 mb-10">
          <RequestRateChart data={timeseries} />
          <LatencyChart data={timeseries} />
          <LatencyHeatmap data={timeseries} />
        </div>
      )}

      {/* Upstream health strip */}
      {upstreams && upstreams.length > 0 && (
        <div className="mb-10">
          <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-3">
            Upstream health
          </div>
          <div className="flex flex-wrap gap-2">
            {upstreams.map((u) => {
              const healthy = u.health.status === "healthy"
              return (
                <div
                  key={u.name}
                  className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border text-[12px] font-medium ${
                    healthy
                      ? "bg-success/8 border-success/20 text-success"
                      : "bg-error/8 border-error/20 text-error"
                  }`}
                  title={
                    healthy
                      ? `${u.name}: ${u.health.latency_ms.toFixed(0)}ms`
                      : `${u.name}: ${u.health.error || "unhealthy"}`
                  }
                >
                  <span className={`w-1.5 h-1.5 rounded-full ${healthy ? "bg-success" : "bg-error"}`} />
                  {u.name}
                </div>
              )
            })}
          </div>
        </div>
      )}

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
                  <tr
                    key={name}
                    onClick={() => navigate(`/compositions/${name}`)}
                    className="border-t border-hairline-soft hover:bg-surface-2 transition-colors cursor-pointer"
                  >
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
