import { useState } from "react"
import { api } from "../lib/api"

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
    <div className="p-8">
      <h1 className="text-2xl font-semibold mb-6">Config</h1>
      <textarea
        className="w-full h-80 bg-zinc-900 border border-zinc-700 rounded-lg p-4 font-mono text-sm resize-y text-zinc-200"
        value={yaml}
        onChange={(e) => setYaml(e.target.value)}
        placeholder="Paste your restitch.yaml here..."
      />
      <div className="flex gap-3 mt-4">
        <button onClick={validate} className="px-4 py-2 bg-amber-600 hover:bg-amber-500 rounded text-sm font-medium">
          Validate
        </button>
        <button onClick={download} className="px-4 py-2 bg-zinc-700 hover:bg-zinc-600 rounded text-sm font-medium">
          Download
        </button>
      </div>
      {result && (
        <div className={`mt-4 p-4 rounded-lg border ${result.valid ? "bg-green-950/30 border-green-800 text-green-400" : "bg-red-950/30 border-red-800 text-red-400"}`}>
          {result.valid ? "Valid" : result.errors.map((e, i) => <div key={i}>{e}</div>)}
        </div>
      )}
    </div>
  )
}
