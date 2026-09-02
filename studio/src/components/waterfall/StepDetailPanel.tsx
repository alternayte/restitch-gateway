import type { StepRecord } from "../../lib/api"
import { X } from "lucide-react"

export function StepDetailPanel({ step, onClose }: { step: StepRecord; onClose: () => void }) {
  return (
    <div className="bg-surface-2 border border-hairline rounded-xl p-4 mt-2">
      <div className="flex items-center justify-between mb-3">
        <div className="text-[14px] font-semibold text-ink">{step.name}</div>
        <button onClick={onClose} aria-label={`Close ${step.name} details`} className="p-1 text-ink-subtle hover:text-ink">
          <X size={14} />
        </button>
      </div>
      <div className="grid grid-cols-2 gap-3 text-[12px]">
        <Detail label="Upstream" value={step.upstream || "—"} />
        <Detail label="URL" value={step.url || "—"} mono />
        <Detail label="Status" value={step.http_status > 0 ? String(step.http_status) : "—"} />
        <Detail label="Duration" value={`${step.duration_ms.toFixed(1)}ms`} />
        <Detail label="Start offset" value={`+${step.start_offset_ms.toFixed(1)}ms`} />
        <Detail label="Body size" value={step.body_size > 0 ? `${step.body_size} bytes` : "—"} />
        <Detail label="Cached" value={step.cached ? "Yes" : "No"} />
        <Detail label="Retries" value={String(step.retries)} />
        {step.error && <Detail label="Error" value={step.error} span2 error />}
      </div>
    </div>
  )
}

function Detail({ label, value, mono, span2, error }: {
  label: string; value: string; mono?: boolean; span2?: boolean; error?: boolean
}) {
  return (
    <div className={span2 ? "col-span-2" : ""}>
      <div className="text-ink-subtle mb-0.5">{label}</div>
      <div className={`${mono ? "font-mono text-[11px]" : ""} ${error ? "text-error" : "text-ink"} break-all`}>
        {value}
      </div>
    </div>
  )
}
