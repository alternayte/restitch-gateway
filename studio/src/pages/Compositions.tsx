import { useNavigate } from "react-router-dom"
import { Pin, PinOff } from "lucide-react"
import { usePoll } from "../hooks/usePoll"
import { usePreferences } from "../hooks/usePreferences"
import { api } from "../lib/api"

export default function Compositions() {
  const { data: compositions } = usePoll(() => api.compositions(), 10000)
  const { prefs, togglePin } = usePreferences()
  const navigate = useNavigate()

  if (!compositions) {
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
      </div>
    )
  }

  const isPinned = (name: string) => prefs.pinnedCompositions.includes(name)

  // Pinned first, then original order preserved within each group.
  const ordered = [...compositions].sort((a, b) => {
    const pa = isPinned(a.name) ? 0 : 1
    const pb = isPinned(b.name) ? 0 : 1
    return pa - pb
  })

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
              <th className="w-10 px-2 py-2.5" />
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Name</th>
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Route</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Steps</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Waves</th>
              <th className="px-4 py-2.5 text-center font-medium text-ink-muted">Public</th>
            </tr>
          </thead>
          <tbody>
            {ordered.map((c) => (
              <tr
                key={c.name}
                data-testid="composition-row"
                onClick={() => navigate(`/compositions/${c.name}`)}
                className="border-t border-hairline-soft cursor-pointer hover:bg-surface-2 transition-colors"
              >
                <td className="px-2 py-2.5 text-center">
                  <button
                    aria-label={isPinned(c.name) ? `Unpin ${c.name}` : `Pin ${c.name}`}
                    onClick={(e) => {
                      e.stopPropagation()
                      togglePin(c.name)
                    }}
                    className={`p-1 rounded transition-colors ${
                      isPinned(c.name)
                        ? "text-rs-accent"
                        : "text-ink-subtle hover:text-ink hover:bg-surface-2"
                    }`}
                  >
                    {isPinned(c.name)
                      ? <Pin size={14} strokeWidth={1.8} />
                      : <PinOff size={14} strokeWidth={1.8} />}
                  </button>
                </td>
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
