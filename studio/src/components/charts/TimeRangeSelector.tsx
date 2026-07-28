export type TimeRange = "1h" | "6h" | "24h"

export function TimeRangeSelector({ value, onChange }: { value: TimeRange; onChange: (v: TimeRange) => void }) {
  const options: TimeRange[] = ["1h", "6h", "24h"]
  return (
    <div className="inline-flex rounded-lg border border-hairline bg-surface-1 p-0.5">
      {options.map((opt) => (
        <button
          key={opt}
          onClick={() => onChange(opt)}
          className={`px-3 py-1 rounded-md text-[12px] font-semibold transition-colors ${
            value === opt
              ? "bg-surface-2 text-ink"
              : "text-ink-muted hover:text-ink"
          }`}
        >
          {opt}
        </button>
      ))}
    </div>
  )
}
