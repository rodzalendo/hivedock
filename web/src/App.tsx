import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import Home from "./views/Home";
import Stacks from "./views/Stacks";
import Dashboard from "./views/Dashboard";
import Updates from "./views/Updates";
import Settings from "./views/Settings";
import ErrorBoundary from "./components/ErrorBoundary";
import { useLiveUpdates } from "./useLiveUpdates";
import { fetchAuthStatus, fetchUpdates, logout, type AuthStatus } from "./api";
import {
  LogoMark,
  HomeIcon,
  StacksIcon,
  UpdatesIcon,
  StatusIcon,
  SettingsIcon,
} from "./components/icons";

type View = "home" | "stacks" | "updates" | "status" | "settings";

const nav: {
  id: View;
  label: string;
  Icon: (p: { className?: string }) => JSX.Element;
}[] = [
  { id: "home", label: "Home", Icon: HomeIcon },
  { id: "stacks", label: "Stacks", Icon: StacksIcon },
  { id: "updates", label: "Updates", Icon: UpdatesIcon },
  { id: "status", label: "Status", Icon: StatusIcon },
  { id: "settings", label: "Settings", Icon: SettingsIcon },
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
          <div className="flex items-center gap-2.5 px-1">
            <LogoMark />
            <span className="font-mono text-[14.5px] font-semibold tracking-[0.02em] text-zinc-50">
              hivedock
            </span>
          </div>
          <div className="md:hidden">
            <SessionControls auth={auth} onLogout={handleLogout} />
          </div>
        </div>

        <nav className="flex gap-1 overflow-x-auto px-2 pb-2 md:flex-col md:space-y-0.5 md:overflow-visible md:px-3 md:pb-0">
          {nav.map(({ id, label, Icon }) => (
            <button
              key={id}
              onClick={() => setView(id)}
              className={`flex shrink-0 items-center gap-2.5 whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition ${
                view === id
                  ? "bg-zinc-800 text-zinc-50"
                  : "text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200"
              }`}
            >
              <Icon className="h-[15px] w-[15px] shrink-0" />
              {label}
              {id === "updates" && updateCount > 0 && (
                <span className="rounded-full border border-hive-500/30 bg-hive-500/[0.14] px-1.5 py-0.5 font-mono text-[10.5px] font-medium leading-none text-hive-500 md:ml-auto">
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
        <ErrorBoundary resetKey={view}>
          {view === "home" && <Dashboard />}
          {view === "stacks" && <Stacks />}
          {view === "updates" && <Updates />}
          {view === "status" && <Home />}
          {view === "settings" && <Settings />}
        </ErrorBoundary>
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
