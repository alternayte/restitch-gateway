export interface Info {
  version: string
  uptime_seconds: number
  config_hash: string
  config_path: string
}

export interface StepRecord {
  name: string
  status: string
  wave: number
  duration_ms: number
  http_status: number
}

export interface RequestRecord {
  id: string
  time: string
  composition: string
  method: string
  path: string
  status: number
  duration_ms: number
  partial: boolean
  steps: StepRecord[]
}

export interface StatsResponse {
  total_requests: number
  total_errors: number
  partial_responses: number
  per_composition: Record<string, { count: number; errors: number; avg_ms: number; p95_ms: number }>
}

export interface ValidateResult {
  valid: boolean
  errors: string[]
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}

async function post<T>(path: string, body: string): Promise<T> {
  const res = await fetch(path, { method: "POST", body, headers: { "Content-Type": "text/plain" } })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}

export const api = {
  info: () => get<Info>("/api/info"),
  requests: (limit = 100) => get<RequestRecord[]>(`/api/requests?limit=${limit}`),
  stats: () => get<StatsResponse>("/api/stats"),
  validate: (yaml: string) => post<ValidateResult>("/api/validate", yaml),
  reload: () => post<{ ok: boolean; config_hash?: string; errors?: string[] }>("/api/reload", ""),
}
