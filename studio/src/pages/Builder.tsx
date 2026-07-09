import { useState } from "react"
import { api } from "../lib/api"
import { buildYaml, type BuilderState } from "../lib/builder"
import { computeWaves } from "../lib/dag"
import { Plus, Trash2, CheckCircle2, XCircle, Copy, Download } from "lucide-react"

const initial: BuilderState = {
  compositionName: "my-composition",
  path: "/api/example",
  method: "GET",
  upstreams: [{ name: "api", url: "http://localhost:8081" }],
  steps: [{ name: "main", upstream: "api", method: "GET", path: "/data", optional: false, depends_on: "" }],
  responseBody: 'result: "{{ steps.main.body }}"',
}

export default function Builder() {
  const [state, setState] = useState<BuilderState>(initial)
  const [result, setResult] = useState<{ valid: boolean; errors: string[] } | null>(null)

  const generated = buildYaml(state)

  const waves = computeWaves(
    state.steps.map((s) => ({
      name: s.name,
      depends_on: s.depends_on.trim() ? s.depends_on.split(",").map((d) => d.trim()) : undefined,
      path: s.path,
    }))
  )

  const update = <K extends keyof BuilderState>(key: K, val: BuilderState[K]) =>
    setState((s) => ({ ...s, [key]: val }))

  const validate = async () => {
    try {
      setResult(await api.validate(generated))
    } catch (e) {
      setResult({ valid: false, errors: [String(e)] })
    }
  }

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Visual
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          Builder
        </h1>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Form */}
        <div className="space-y-6">
          {/* Meta */}
          <Section title="Composition">
            <div className="grid grid-cols-3 gap-3">
              <Input label="Name" value={state.compositionName} onChange={(v) => update("compositionName", v)} />
              <Input label="Path" value={state.path} onChange={(v) => update("path", v)} />
              <Select label="Method" value={state.method} onChange={(v) => update("method", v)} options={["GET", "POST", "PUT", "DELETE"]} />
            </div>
          </Section>

          {/* Upstreams */}
          <Section title="Upstreams">
            {state.upstreams.map((u, i) => (
              <div key={i} className="grid grid-cols-[1fr_2fr_auto] gap-3 mb-2">
                <Input label="" value={u.name} placeholder="name" onChange={(v) => {
                  const ups = [...state.upstreams]; ups[i] = { ...u, name: v }; update("upstreams", ups)
                }} />
                <Input label="" value={u.url} placeholder="url" onChange={(v) => {
                  const ups = [...state.upstreams]; ups[i] = { ...u, url: v }; update("upstreams", ups)
                }} />
                <button onClick={() => update("upstreams", state.upstreams.filter((_, j) => j !== i))} className="text-ink-subtle hover:text-error mt-1">
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
            <button onClick={() => update("upstreams", [...state.upstreams, { name: "", url: "" }])} className="flex items-center gap-1.5 text-[13px] text-rs-accent hover:text-accent-bright mt-1">
              <Plus size={14} /> Add upstream
            </button>
          </Section>

          {/* Steps */}
          <Section title="Steps">
            {state.steps.map((s, i) => (
              <div key={i} className="bg-surface-2 rounded-xl border border-hairline p-4 mb-3">
                <div className="grid grid-cols-[1fr_1fr_auto] gap-3 mb-2">
                  <Input label="Name" value={s.name} onChange={(v) => {
                    const steps = [...state.steps]; steps[i] = { ...s, name: v }; update("steps", steps)
                  }} />
                  <Select label="Upstream" value={s.upstream} onChange={(v) => {
                    const steps = [...state.steps]; steps[i] = { ...s, upstream: v }; update("steps", steps)
                  }} options={state.upstreams.map((u) => u.name).filter(Boolean)} />
                  <button onClick={() => update("steps", state.steps.filter((_, j) => j !== i))} className="text-ink-subtle hover:text-error mt-5">
                    <Trash2 size={14} />
                  </button>
                </div>
                <div className="grid grid-cols-[auto_1fr_1fr] gap-3">
                  <Select label="Method" value={s.method} onChange={(v) => {
                    const steps = [...state.steps]; steps[i] = { ...s, method: v }; update("steps", steps)
                  }} options={["GET", "POST", "PUT", "DELETE"]} />
                  <Input label="Path" value={s.path} onChange={(v) => {
                    const steps = [...state.steps]; steps[i] = { ...s, path: v }; update("steps", steps)
                  }} />
                  <Input label="Depends on" value={s.depends_on} placeholder="step1, step2" onChange={(v) => {
                    const steps = [...state.steps]; steps[i] = { ...s, depends_on: v }; update("steps", steps)
                  }} />
                </div>
              </div>
            ))}
            <button onClick={() => update("steps", [...state.steps, { name: "", upstream: state.upstreams[0]?.name ?? "", method: "GET", path: "/", optional: false, depends_on: "" }])} className="flex items-center gap-1.5 text-[13px] text-rs-accent hover:text-accent-bright">
              <Plus size={14} /> Add step
            </button>
          </Section>

          {/* Response */}
          <Section title="Response body">
            <textarea
              className="w-full h-24 bg-canvas border border-hairline rounded-lg p-3 font-mono text-[12px] text-ink resize-y outline-none"
              value={state.responseBody}
              onChange={(e) => update("responseBody", e.target.value)}
            />
          </Section>
        </div>

        {/* Preview */}
        <div className="space-y-6">
          {/* DAG preview */}
          {waves.length > 0 && (
            <div>
              <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-3">
                Execution waves
              </div>
              <div className="bg-surface-1 border border-hairline rounded-xl p-4">
                <div className="flex items-start gap-3">
                  {waves.map((wave, wi) => (
                    <div key={wi} className="flex items-center gap-3">
                      <div className="space-y-1.5">
                        {wave.map((name) => (
                          <div key={name} className="px-3 py-1.5 bg-surface-2 border border-hairline rounded-lg text-[12px] font-medium text-ink">
                            {name || "(unnamed)"}
                          </div>
                        ))}
                      </div>
                      {wi < waves.length - 1 && (
                        <div className="text-ink-subtle text-[16px]">&rarr;</div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* Generated YAML */}
          <div>
            <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-3">
              Generated YAML
            </div>
            <div className="bg-surface-1 border border-hairline rounded-xl overflow-hidden">
              <pre className="p-5 font-mono text-[12px] leading-[1.6] text-ink overflow-auto max-h-[600px]">
                {generated}
              </pre>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <button onClick={validate} className="inline-flex items-center gap-2 px-[18px] py-[10px] bg-inverse-canvas text-inverse-ink text-[14px] font-semibold rounded-lg hover:bg-ink-muted transition-colors">
              Validate
            </button>
            <button onClick={() => navigator.clipboard.writeText(generated)} className="inline-flex items-center gap-2 px-[18px] py-[10px] bg-surface-2 text-ink text-[14px] font-semibold rounded-lg hover:bg-surface-3 transition-colors">
              <Copy size={14} /> Copy
            </button>
            <button onClick={() => {
              const blob = new Blob([generated], { type: "text/yaml" })
              const url = URL.createObjectURL(blob)
              const a = document.createElement("a"); a.href = url; a.download = "restitch.yaml"; a.click()
              URL.revokeObjectURL(url)
            }} className="inline-flex items-center gap-2 px-[18px] py-[10px] bg-surface-2 text-ink text-[14px] font-semibold rounded-lg hover:bg-surface-3 transition-colors">
              <Download size={14} /> Download
            </button>
          </div>

          {result && (
            <div className={`rounded-xl border p-4 ${result.valid ? "bg-success/8 border-success/20" : "bg-error/8 border-error/20"}`}>
              <div className="flex items-center gap-2">
                {result.valid ? <CheckCircle2 size={16} className="text-success" /> : <XCircle size={16} className="text-error" />}
                <span className={`text-[13px] font-semibold ${result.valid ? "text-success" : "text-error"}`}>
                  {result.valid ? "Valid" : "Invalid"}
                </span>
              </div>
              {!result.valid && result.errors.map((e, i) => (
                <div key={i} className="text-[12px] text-error/80 font-mono mt-2 pl-6">{e}</div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-3">{title}</div>
      {children}
    </div>
  )
}

function Input({ label, value, placeholder, onChange }: { label: string; value: string; placeholder?: string; onChange: (v: string) => void }) {
  return (
    <div>
      {label && <label className="block text-[12px] text-ink-muted mb-1">{label}</label>}
      <input
        className="w-full bg-canvas border border-hairline rounded-lg px-3 py-[7px] text-[13px] text-ink outline-none focus:border-rs-accent/50"
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  )
}

function Select({ label, value, options, onChange }: { label: string; value: string; options: string[]; onChange: (v: string) => void }) {
  return (
    <div>
      {label && <label className="block text-[12px] text-ink-muted mb-1">{label}</label>}
      <select
        className="w-full bg-canvas border border-hairline rounded-lg px-3 py-[7px] text-[13px] text-ink outline-none focus:border-rs-accent/50"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </div>
  )
}
