import { usePoll } from "../hooks/usePoll"
import { api } from "../lib/api"

export default function Dashboard() {
  const { data: stats } = usePoll(() => api.stats(), 5000)

  if (!stats) return <div className="p-8 text-zinc-500">Loading...</div>

  return (
    <div className="p-8">
      <h1 className="text-2xl font-semibold mb-6">Dashboard</h1>
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
        <StatCard label="Total Requests" value={stats.total_requests} />
        <StatCard
          label="Error Rate"
          value={stats.total_requests > 0
            ? `${((stats.total_errors / stats.total_requests) * 100).toFixed(1)}%`
            : "—"}
        />
        <StatCard label="Partial Responses" value={stats.partial_responses} />
        <StatCard label="Compositions" value={Object.keys(stats.per_composition).length} />
      </div>

      {Object.keys(stats.per_composition).length > 0 && (
        <div className="border border-zinc-800 rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-zinc-900 text-zinc-400">
              <tr>
                <th className="px-4 py-2 text-left">Composition</th>
                <th className="px-4 py-2 text-right">Requests</th>
                <th className="px-4 py-2 text-right">Errors</th>
                <th className="px-4 py-2 text-right">Avg (ms)</th>
                <th className="px-4 py-2 text-right">P95 (ms)</th>
              </tr>
            </thead>
            <tbody>
              {Object.entries(stats.per_composition).map(([name, s]) => (
                <tr key={name} className="border-t border-zinc-800 hover:bg-zinc-900/50">
                  <td className="px-4 py-2">{name}</td>
                  <td className="px-4 py-2 text-right">{s.count}</td>
                  <td className="px-4 py-2 text-right">{s.errors}</td>
                  <td className="px-4 py-2 text-right">{s.avg_ms}</td>
                  <td className="px-4 py-2 text-right">{s.p95_ms}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {stats.total_requests === 0 && (
        <div className="text-center py-12 text-zinc-500">
          No requests yet — point traffic at the gateway
        </div>
      )}
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
      <div className="text-zinc-400 text-xs uppercase tracking-wider mb-1">{label}</div>
      <div className="text-2xl font-semibold">{value}</div>
    </div>
  )
}
