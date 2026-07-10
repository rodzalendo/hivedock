import { useState } from "react";
import Home from "./views/Home";
import Stacks from "./views/Stacks";
import Dashboard from "./views/Dashboard";
import { useLiveUpdates } from "./useLiveUpdates";

type View = "home" | "stacks" | "status";

const nav: { id: View; label: string; icon: string }[] = [
  { id: "home", label: "Home", icon: "▦" },
  { id: "stacks", label: "Stacks", icon: "▤" },
  { id: "status", label: "Status", icon: "◉" },
];

export default function App() {
  const [view, setView] = useState<View>("home");
  useLiveUpdates();

  return (
    <div className="flex min-h-screen bg-zinc-950 text-zinc-100">
      <aside className="flex w-52 shrink-0 flex-col border-r border-zinc-800 px-3 py-4">
        <div className="mb-6 flex items-center gap-2 px-2">
          <span className="text-xl" aria-hidden>
            🐝
          </span>
          <span className="font-semibold tracking-tight">Hivedock</span>
        </div>
        <nav className="space-y-1">
          {nav.map((item) => (
            <button
              key={item.id}
              onClick={() => setView(item.id)}
              className={`flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition ${
                view === item.id
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200"
              }`}
            >
              <span className="text-zinc-500" aria-hidden>
                {item.icon}
              </span>
              {item.label}
            </button>
          ))}
        </nav>
        <p className="mt-auto px-2 text-[10px] text-zinc-600">
          Phase 1 — read-only truth
        </p>
      </aside>

      <main className="flex-1 overflow-x-hidden px-6 py-6 lg:px-8">
        {view === "home" && <Dashboard />}
        {view === "stacks" && <Stacks />}
        {view === "status" && <Home />}
      </main>
    </div>
  );
}
