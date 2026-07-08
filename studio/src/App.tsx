import { BrowserRouter, Routes, Route, NavLink } from "react-router-dom"
import { LayoutDashboard, Activity, Settings, Zap } from "lucide-react"
import { usePoll } from "./hooks/usePoll"
import { api } from "./lib/api"
import Dashboard from "./pages/Dashboard"
import Requests from "./pages/Requests"
import Config from "./pages/Config"

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/requests", label: "Requests", icon: Activity },
  { to: "/config", label: "Config", icon: Settings },
]

export default function App() {
  const { data: info } = usePoll(() => api.info(), 30000)

  return (
    <BrowserRouter>
      <div className="flex h-screen">
        {/* Sidebar — canvas surface with hairline right edge */}
        <nav className="w-52 flex flex-col border-r border-hairline bg-canvas">
          {/* Brand mark */}
          <div className="px-4 py-5 border-b border-hairline flex items-center gap-2.5">
            <Zap size={18} className="text-accent" />
            <span className="text-[15px] font-semibold tracking-[-0.2px] text-ink">
              Restitch
            </span>
            <span className="text-[11px] font-600 tracking-[0.6px] uppercase text-ink-subtle ml-auto">
              Studio
            </span>
          </div>

          {/* Nav links */}
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

          {/* Footer — version + hash */}
          {info && (
            <div className="px-4 py-3 border-t border-hairline-soft">
              <div className="text-[11px] font-semibold tracking-[0.6px] uppercase text-ink-subtle">
                Gateway
              </div>
              <div className="text-[12px] text-ink-muted mt-0.5 font-mono">
                {info.config_hash.slice(0, 8)}
              </div>
            </div>
          )}
        </nav>

        {/* Main content — canvas background */}
        <main className="flex-1 overflow-auto bg-canvas">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/requests" element={<Requests />} />
            <Route path="/config" element={<Config />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
