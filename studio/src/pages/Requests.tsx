import { usePoll } from "../hooks/usePoll"
import { api } from "../lib/api"

function statusColor(code: number) {
  if (code < 300) return "bg-success/15 text-success"
  if (code < 500) return "bg-warning/15 text-warning"
  return "bg-error/15 text-error"
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60_000) return `${Math.floor(diff / 1000)}s ago`
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`
  return new Date(iso).toLocaleTimeString()
}

export default function Requests() {
  const { data: requests } = usePoll(() => api.requests(100), 3000)

  if (!requests) {
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
        <div className="mt-8 space-y-2">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="h-10 bg-surface-1 rounded-lg animate-pulse" />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Explorer
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          Requests
        </h1>
      </div>

      {requests.length === 0 ? (
        <div className="bg-surface-1 border border-hairline rounded-xl p-12 text-center">
          <div className="text-[20px] font-semibold text-ink mb-2">
            No requests recorded
          </div>
          <p className="text-[14px] text-ink-muted leading-[1.5]">
            Send traffic to the gateway to see requests appear here in real time.
          </p>
        </div>
      ) : (
        <div className="bg-surface-1 rounded-xl border border-hairline overflow-hidden">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="border-b border-hairline-soft">
                <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Time</th>
                <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Composition</th>
                <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Route</th>
                <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Status</th>
                <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Duration</th>
                <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Partial</th>
              </tr>
            </thead>
            <tbody>
              {requests.map((req, i) => (
                <tr key={i} className="border-t border-hairline-soft hover:bg-surface-2 transition-colors">
                  <td className="px-4 py-2.5 text-ink-subtle tabular-nums">
                    {relativeTime(req.time)}
                  </td>
                  <td className="px-4 py-2.5 font-medium text-ink">{req.composition}</td>
                  <td className="px-4 py-2.5 font-mono text-[12px] text-ink-muted">
                    <span className="text-ink-subtle">{req.method}</span>{" "}
                    {req.path}
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <span className={`inline-block px-2 py-0.5 rounded text-[11px] font-semibold tabular-nums ${statusColor(req.status)}`}>
                      {req.status}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums font-mono text-[12px]">
                    {req.duration_ms.toFixed(1)}ms
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    {req.partial && (
                      <span className="inline-block px-2 py-0.5 rounded text-[11px] font-semibold bg-warning/15 text-warning">
                        partial
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
