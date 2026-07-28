interface RequestFiltersProps {
  compositions: string[]
  composition: string
  onCompositionChange: (v: string) => void
  statusFilter: string
  onStatusChange: (v: string) => void
  durationFilter: string
  onDurationChange: (v: string) => void
  partialOnly: boolean
  onPartialChange: (v: boolean) => void
}

export function RequestFilters({
  compositions, composition, onCompositionChange,
  statusFilter, onStatusChange,
  durationFilter, onDurationChange,
  partialOnly, onPartialChange,
}: RequestFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-3 mb-6">
      <FilterSelect label="Composition" value={composition} onChange={onCompositionChange}
        options={[{ value: "", label: "All" }, ...compositions.map((c) => ({ value: c, label: c }))]} />
      <FilterSelect label="Status" value={statusFilter} onChange={onStatusChange}
        options={[
          { value: "", label: "All" },
          { value: "2xx", label: "2xx" },
          { value: "4xx", label: "4xx" },
          { value: "5xx", label: "5xx" },
        ]} />
      <FilterSelect label="Duration" value={durationFilter} onChange={onDurationChange}
        options={[
          { value: "", label: "All" },
          { value: "100", label: ">100ms" },
          { value: "500", label: ">500ms" },
          { value: "1000", label: ">1s" },
        ]} />
      <button
        onClick={() => onPartialChange(!partialOnly)}
        className={`px-3 py-1 rounded-lg text-[12px] font-medium border transition-colors ${
          partialOnly ? "bg-warning/15 border-warning/30 text-warning" : "border-hairline text-ink-muted hover:text-ink"
        }`}
      >
        Partial only
      </button>
    </div>
  )
}

function FilterSelect({ label, value, onChange, options }: {
  label: string; value: string; onChange: (v: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-[11px] font-semibold tracking-[0.5px] uppercase text-ink-subtle">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="bg-surface-1 border border-hairline rounded-lg px-2.5 py-1 text-[12px] text-ink outline-none"
      >
        {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    </div>
  )
}
