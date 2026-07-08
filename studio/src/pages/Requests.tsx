import { usePoll } from "../hooks/usePoll"
import { api } from "../lib/api"

export default function Requests() {
  const { data: requests } = usePoll(() => api.requests(100), 3000)

  if (!requests) return <div className="p-8 text-zinc-500">Loading...</div>

  return (
    <div className="p-8">
      <h1 className="text-2xl font-semibold mb-6">Requests</h1>
      {requests.length === 0 ? (
        <div className="text-center py-12 text-zinc-500">No requests recorded yet</div>
      ) : (
        <div className="border border-zinc-800 rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-zinc-900 text-zinc-400">
              <tr>
                <th className="px-4 py-2 text-left">Time</th>
                <th className="px-4 py-2 text-left">Composition</th>
                <th className="px-4 py-2 text-left">Path</th>
                <th className="px-4 py-2 text-right">Status</th>
                <th className="px-4 py-2 text-right">Duration</th>
                <th className="px-4 py-2 text-center">Partial</th>
              </tr>
            </thead>
            <tbody>
              {requests.map((req, i) => (
                <tr key={i} className="border-t border-zinc-800 hover:bg-zinc-900/50">
                  <td className="px-4 py-2 text-zinc-400">{new Date(req.time).toLocaleTimeString()}</td>
                  <td className="px-4 py-2">{req.composition}</td>
                  <td className="px-4 py-2 font-mono text-xs">{req.method} {req.path}</td>
                  <td className="px-4 py-2 text-right">
                    <span className={`px-2 py-0.5 rounded text-xs ${
                      req.status < 300 ? "bg-green-900/50 text-green-400" :
                      req.status < 500 ? "bg-amber-900/50 text-amber-400" :
                      "bg-red-900/50 text-red-400"
                    }`}>{req.status}</span>
                  </td>
                  <td className="px-4 py-2 text-right">{req.duration_ms.toFixed(1)}ms</td>
                  <td className="px-4 py-2 text-center">{req.partial ? "yes" : ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
