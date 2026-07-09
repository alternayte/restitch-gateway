import { BrowserRouter, Routes, Route, NavLink } from "react-router-dom"
import { LayoutDashboard, Activity, GitBranch, Hammer, Settings, Zap, RefreshCw } from "lucide-react"
import { Toaster, toast } from "sonner"
import { useState } from "react"
import { usePoll } from "./hooks/usePoll"
import { api } from "./lib/api"
import Dashboard from "./pages/Dashboard"
import Compositions from "./pages/Compositions"
import CompositionDetail from "./pages/CompositionDetail"
import Requests from "./pages/Requests"
import Builder from "./pages/Builder"
import Config from "./pages/Config"

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/compositions", label: "Compositions", icon: GitBranch },
  { to: "/requests", label: "Requests", icon: Activity },
  { to: "/builder", label: "Builder", icon: Hammer },
  { to: "/config", label: "Config", icon: Settings },
]

export default function App() {
  const { data: info } = usePoll(() => api.info(), 30000)
  const [reloading, setReloading] = useState(false)

  const handleReload = async () => {
    if (!confirm("Reload gateway configuration?")) return
    setReloading(true)
    try {
      const res = await api.reload()
      if (res.ok) {
        toast.success("Config reloaded", { description: `Hash: ${res.config_hash?.slice(0, 8)}` })
      } else {
        toast.error("Reload failed", { description: res.errors?.join(", ") })
      }
    } catch (e) {
      toast.error("Reload failed", { description: String(e) })
    } finally {
      setReloading(false)
    }
  }

  return (
    <BrowserRouter>
      <Toaster
        theme="dark"
        toastOptions={{
          style: { background: "#1c1c1f", border: "1px solid rgba(178,182,189,0.12)", color: "#fff" },
        }}
      />
      <div className="flex h-screen">
        <nav className="w-52 flex flex-col border-r border-hairline bg-canvas">
          <div className="px-4 py-5 border-b border-hairline flex items-center gap-2.5">
            <Zap size={18} className="text-rs-accent" />
            <span className="text-[15px] font-semibold tracking-[-0.2px] text-ink">
              Restitch
            </span>
            <span className="text-[11px] font-semibold tracking-[0.6px] uppercase text-ink-subtle ml-auto">
              Studio
            </span>
          </div>

          <div className="flex-1 px-2 pt-3">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) =>
                  `flex items-center gap-2.5 px-3 py-[7px] rounded-lg text-[13px] font-medium mb-0.5 transition-colors ${
                    isActive
                      ? "bg-surface-2 text-ink"
                      : "text-ink-muted hover:text-ink hover:bg-surface-1"
                  }`
                }
              >
                <item.icon size={16} strokeWidth={1.8} />
                {item.label}
              </NavLink>
            ))}
          </div>

          {info && (
            <div className="px-4 py-3 border-t border-hairline-soft">
              <div className="flex items-center justify-between mb-1">
                <div className="text-[11px] font-semibold tracking-[0.6px] uppercase text-ink-subtle">
                  Gateway
                </div>
                <button
                  onClick={handleReload}
                  disabled={reloading}
                  title="Reload config"
                  className="p-1 rounded text-ink-subtle hover:text-rs-accent hover:bg-surface-2 transition-colors disabled:opacity-40"
                >
                  <RefreshCw size={12} className={reloading ? "animate-spin" : ""} />
                </button>
              </div>
              <div className="text-[12px] text-ink-muted font-mono">
                {info.config_hash.slice(0, 8)}
              </div>
            </div>
          )}
        </nav>

        <main className="flex-1 overflow-auto bg-canvas">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/compositions" element={<Compositions />} />
            <Route path="/compositions/:name" element={<CompositionDetail />} />
            <Route path="/requests" element={<Requests />} />
            <Route path="/builder" element={<Builder />} />
            <Route path="/config" element={<Config />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
