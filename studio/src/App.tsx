import { BrowserRouter, Routes, Route, NavLink } from "react-router-dom"
import { LayoutDashboard, Activity, Settings } from "lucide-react"
import Dashboard from "./pages/Dashboard"
import Requests from "./pages/Requests"
import Config from "./pages/Config"

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/requests", label: "Requests", icon: Activity },
  { to: "/config", label: "Config", icon: Settings },
]

export default function App() {
  return (
    <BrowserRouter>
      <div className="flex h-screen">
        <nav className="w-56 bg-zinc-950 border-r border-zinc-800 flex flex-col">
          <div className="p-4 border-b border-zinc-800">
            <h1 className="text-lg font-semibold text-amber-500">Restitch Studio</h1>
          </div>
          <div className="flex-1 p-2">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) =>
                  `flex items-center gap-3 px-3 py-2 rounded-lg text-sm mb-1 ${
                    isActive ? "bg-zinc-800 text-white" : "text-zinc-400 hover:text-white hover:bg-zinc-900"
                  }`
                }
              >
                <item.icon size={18} />
                {item.label}
              </NavLink>
            ))}
          </div>
        </nav>
        <main className="flex-1 overflow-auto bg-zinc-950">
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
