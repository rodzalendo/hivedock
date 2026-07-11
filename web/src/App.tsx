import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import Home from "./views/Home";
import Stacks from "./views/Stacks";
import Dashboard from "./views/Dashboard";
import Updates from "./views/Updates";
import Settings from "./views/Settings";
import { useLiveUpdates } from "./useLiveUpdates";
import { fetchAuthStatus, fetchUpdates, logout, type AuthStatus } from "./api";

type View = "home" | "stacks" | "updates" | "status" | "settings";

const nav: { id: View; label: string; icon: string }[] = [
  { id: "home", label: "Home", icon: "▦" },
  { id: "stacks", label: "Stacks", icon: "▤" },
  { id: "updates", label: "Updates", icon: "↑" },
  { id: "status", label: "Status", icon: "◉" },
  { id: "settings", label: "Settings", icon: "⚙" },
];

export default function App() {
  const [view, setView] = useState<View>("home");
  const qc = useQueryClient();
  useLiveUpdates();

  const { data: auth } = useQuery({
    queryKey: ["auth-status"],
    queryFn: fetchAuthStatus,
    staleTime: 60_000,
  });

  // Drives the nav badge; shares the cache with the Updates view.
  const { data: updates } = useQuery({
    queryKey: ["updates"],
    queryFn: fetchUpdates,
    staleTime: 30_000,
  });
  const updateCount = updates?.filter((u) => u.hasUpdate).length ?? 0;

  async function handleLogout() {
    try {
      await logout();
    } finally {
      await qc.invalidateQueries({ queryKey: ["auth-status"] });
    }
  }

  return (
    <div className="flex min-h-screen flex-col bg-zinc-950 text-zinc-100 md:flex-row">
      <aside className="flex shrink-0 flex-col border-zinc-800 md:w-52 md:border-r">
        <div className="flex items-center justify-between border-b border-zinc-800 px-3 py-3 md:border-b-0 md:py-4">
          <div className="flex items-center gap-2 px-1">
            <span className="text-xl" aria-hidden>
              🐝
            </span>
            <span className="font-semibold tracking-tight">Hivedock</span>
          </div>
          <div className="md:hidden">
            <SessionControls auth={auth} onLogout={handleLogout} />
          </div>
        </div>

        <nav className="flex gap-1 overflow-x-auto px-2 pb-2 md:flex-col md:space-y-1 md:overflow-visible md:px-3 md:pb-0">
          {nav.map((item) => (
            <button
              key={item.id}
              onClick={() => setView(item.id)}
              className={`flex shrink-0 items-center gap-2.5 whitespace-nowrap rounded-lg px-3 py-2 text-sm transition ${
                view === item.id
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200"
              }`}
            >
              <span className="text-zinc-500" aria-hidden>
                {item.icon}
              </span>
              {item.label}
              {item.id === "updates" && updateCount > 0 && (
                <span className="rounded-full bg-hive-600 px-1.5 py-0.5 text-[10px] font-semibold text-white md:ml-auto">
                  {updateCount}
                </span>
              )}
            </button>
          ))}
        </nav>

        <div className="mt-auto hidden p-3 md:block">
          <SessionControls auth={auth} onLogout={handleLogout} />
        </div>
      </aside>

      <main className="min-w-0 flex-1 overflow-x-hidden px-4 py-5 sm:px-6 lg:px-8">
        {view === "home" && <Dashboard />}
        {view === "stacks" && <Stacks />}
        {view === "updates" && <Updates />}
        {view === "status" && <Home />}
        {view === "settings" && <Settings />}
      </main>
    </div>
  );
}

function SessionControls({
  auth,
  onLogout,
}: {
  auth: AuthStatus | undefined;
  onLogout: () => void;
}) {
  if (!auth) return null;
  if (auth.authDisabled) {
    return <span className="text-[10px] text-amber-500/70">Auth disabled</span>;
  }
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="truncate text-xs text-zinc-500">
        {auth.username ?? "admin"}
      </span>
      <button
        onClick={onLogout}
        className="rounded-md px-2 py-1 text-xs text-zinc-400 transition hover:bg-zinc-900 hover:text-zinc-200"
      >
        Sign out
      </button>
    </div>
  );
}
