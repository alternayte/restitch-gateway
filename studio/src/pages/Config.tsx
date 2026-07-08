import { useState } from "react"
import { api } from "../lib/api"
import { CheckCircle2, XCircle, Download } from "lucide-react"

export default function Config() {
  const [yaml, setYaml] = useState("")
  const [result, setResult] = useState<{ valid: boolean; errors: string[] } | null>(null)

  const validate = async () => {
    try {
      const res = await api.validate(yaml)
      setResult(res)
    } catch (e) {
      setResult({ valid: false, errors: [String(e)] })
    }
  }

  const download = () => {
    const blob = new Blob([yaml], { type: "text/yaml" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = "restitch.yaml"
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Configuration
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          Config
        </h1>
      </div>

      <div className="bg-surface-1 border border-hairline rounded-xl overflow-hidden">
        <textarea
          className="w-full h-80 bg-transparent p-5 font-mono text-[13px] leading-[1.6] text-ink resize-y border-none outline-none placeholder:text-ink-subtle"
          value={yaml}
          onChange={(e) => { setYaml(e.target.value); setResult(null) }}
          placeholder="Paste your restitch.yaml here..."
          spellCheck={false}
        />
      </div>

      <div className="flex items-center gap-3 mt-4">
        <button
          onClick={validate}
          disabled={!yaml.trim()}
          className="inline-flex items-center gap-2 px-[18px] py-[10px] bg-inverse-canvas text-inverse-ink text-[14px] font-semibold rounded-lg hover:bg-ink-muted transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          Validate
        </button>
        <button
          onClick={download}
          disabled={!yaml.trim()}
          className="inline-flex items-center gap-2 px-[18px] py-[10px] bg-surface-2 text-ink text-[14px] font-semibold rounded-lg hover:bg-surface-3 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Download size={14} />
          Download
        </button>
      </div>

      {result && (
        <div className={`mt-6 rounded-xl border p-5 ${
          result.valid
            ? "bg-success/8 border-success/20"
            : "bg-error/8 border-error/20"
        }`}>
          <div className="flex items-center gap-2 mb-1">
            {result.valid ? (
              <CheckCircle2 size={16} className="text-success" />
            ) : (
              <XCircle size={16} className="text-error" />
            )}
            <span className={`text-[14px] font-semibold ${result.valid ? "text-success" : "text-error"}`}>
              {result.valid ? "Valid configuration" : "Validation failed"}
            </span>
          </div>
          {!result.valid && (
            <div className="mt-3 space-y-1.5">
              {result.errors.map((e, i) => (
                <div key={i} className="text-[13px] text-error/80 font-mono leading-[1.5] pl-6">
                  {e}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
