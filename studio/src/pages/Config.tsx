import { useState, useCallback } from "react"
import CodeMirror from "@uiw/react-codemirror"
import { yaml as yamlLang } from "@codemirror/lang-yaml"
import * as yamlLib from "js-yaml"
import { api } from "../lib/api"
import { CheckCircle2, XCircle, Download, Upload } from "lucide-react"

const SKELETON = `# Restitch configuration
# Edit this YAML or use "Load current" to fetch runtime state.
#
# server:
#   port: 8080
#   log_format: json
#   log_level: info
#
# upstreams:
#   my-api:
#     url: "http://localhost:8081"
#     timeout: 10s
#
# compositions:
#   example:
#     path: "/api/example"
#     method: GET
#     steps:
#       - name: data
#         upstream: my-api
#         path: "/data"
#     response:
#       body:
#         result: "{{ steps.data.body }}"
`

export default function Config() {
  const [yamlContent, setYamlContent] = useState(SKELETON)
  const [result, setResult] = useState<{ valid: boolean; errors: string[] } | null>(null)

  const validate = async () => {
    try {
      const res = await api.validate(yamlContent)
      setResult(res)
    } catch (e) {
      setResult({ valid: false, errors: [String(e)] })
    }
  }

  const download = () => {
    const blob = new Blob([yamlContent], { type: "text/yaml" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = "restitch.yaml"
    a.click()
    URL.revokeObjectURL(url)
  }

  const loadCurrent = useCallback(async () => {
    try {
      const [compositions, upstreams] = await Promise.all([
        api.compositions(),
        api.upstreams(),
      ])

      const upstreamsMap: Record<string, { url: string; timeout_ms?: number }> = {}
      for (const u of upstreams) {
        const entry: { url: string; timeout_ms?: number } = { url: u.url }
        if (u.timeout_ms > 0) entry.timeout_ms = u.timeout_ms
        upstreamsMap[u.name] = entry
      }

      const compositionsMap: Record<string, unknown> = {}
      for (const c of compositions) {
        compositionsMap[c.name] = {
          path: c.path,
          method: c.method,
          ...(c.public ? { public: true } : {}),
          steps: c.steps.map((s) => ({
            name: s.name,
            upstream: s.upstream,
            path: `/${s.name}`,
            method: s.method,
            ...(s.optional ? { optional: true } : {}),
            ...(s.depends_on?.length ? { depends_on: s.depends_on } : {}),
          })),
          response: { status: 200, body: {} },
        }
      }

      const doc = { upstreams: upstreamsMap, compositions: compositionsMap }
      const generated = `# Regenerated from runtime state — secrets and comments not included\n${yamlLib.dump(doc, { lineWidth: -1 })}`
      setYamlContent(generated)
      setResult(null)
    } catch (e) {
      setResult({ valid: false, errors: [`Failed to load: ${e}`] })
    }
  }, [])

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
        <CodeMirror
          value={yamlContent}
          onChange={(val) => { setYamlContent(val); setResult(null) }}
          extensions={[yamlLang()]}
          height="380px"
          theme="dark"
          basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: true }}
          style={{ fontSize: "13px" }}
        />
      </div>

      <div className="flex items-center gap-3 mt-4">
        <button
          onClick={validate}
          disabled={!yamlContent.trim()}
          className="inline-flex items-center gap-2 px-[18px] py-[10px] bg-inverse-canvas text-inverse-ink text-[14px] font-semibold rounded-lg hover:bg-ink-muted transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          Validate
        </button>
        <button
          onClick={loadCurrent}
          className="inline-flex items-center gap-2 px-[18px] py-[10px] bg-surface-2 text-ink text-[14px] font-semibold rounded-lg hover:bg-surface-3 transition-colors"
        >
          <Upload size={14} />
          Load current
        </button>
        <button
          onClick={download}
          disabled={!yamlContent.trim()}
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
