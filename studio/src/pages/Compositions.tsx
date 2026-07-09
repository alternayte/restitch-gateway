import { useNavigate } from "react-router-dom"
import { usePoll } from "../hooks/usePoll"
import { api } from "../lib/api"

export default function Compositions() {
  const { data: compositions } = usePoll(() => api.compositions(), 10000)
  const navigate = useNavigate()

  if (!compositions) {
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
      </div>
    )
  }

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Routes
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          Compositions
        </h1>
      </div>

      <div className="bg-surface-1 rounded-xl border border-hairline overflow-hidden">
        <table className="w-full text-[13px]">
          <thead>
            <tr className="border-b border-hairline-soft">
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Name</th>
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Route</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Steps</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Waves</th>
              <th className="px-4 py-2.5 text-center font-medium text-ink-muted">Public</th>
            </tr>
          </thead>
          <tbody>
            {compositions.map((c) => (
              <tr
                key={c.name}
                onClick={() => navigate(`/compositions/${c.name}`)}
                className="border-t border-hairline-soft cursor-pointer hover:bg-surface-2 transition-colors"
              >
                <td className="px-4 py-2.5 font-medium text-ink">{c.name}</td>
                <td className="px-4 py-2.5 font-mono text-[12px] text-ink-muted">
                  <span className="text-rs-accent font-semibold">{c.method}</span> {c.path}
                </td>
                <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums">{c.steps.length}</td>
                <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums">{c.waves.length}</td>
                <td className="px-4 py-2.5 text-center">
                  {c.public && (
                    <span className="inline-block px-2 py-0.5 rounded text-[11px] font-semibold bg-success/15 text-success">
                      public
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
