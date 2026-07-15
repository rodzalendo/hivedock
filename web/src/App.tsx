import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import Stacks from "./views/Stacks";
import Dashboard from "./views/Dashboard";
import Updates from "./views/Updates";
import Settings from "./views/Settings";
import ErrorBoundary from "./components/ErrorBoundary";
import { useLiveUpdates } from "./useLiveUpdates";
import { useHashRoute, navigate } from "./useHashRoute";
import {
  fetchAppUpdate,
  fetchAuthStatus,
  fetchHealth,
  fetchUpdates,
  logout,
  selfUpdate,
  type AuthStatus,
} from "./api";
import {
  LogoMark,
  HomeIcon,
  StacksIcon,
  UpdatesIcon,
  SettingsIcon,
} from "./components/icons";

type View = "home" | "stacks" | "updates" | "settings";

const nav: {
  id: View;
  label: string;
  Icon: (p: { className?: string }) => JSX.Element;
}[] = [
  { id: "home", label: "Home", Icon: HomeIcon },
  { id: "stacks", label: "Stacks", Icon: StacksIcon },
  { id: "updates", label: "Updates", Icon: UpdatesIcon },
  { id: "settings", label: "Settings", Icon: SettingsIcon },
];

export default function App() {
  // The URL hash is the router (#/stacks, #/updates, …), so a refresh stays
  // on the current page. Unknown/empty hashes land on Home.
  const route = useHashRoute();
  const view: View = nav.some((n) => n.id === route[0])
    ? (route[0] as View)
    : "home";
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
  // Ignored (keep-pinned) updates don't count toward the badge.
  const updateCount =
    updates?.filter((u) => u.hasUpdate && !u.ignored).length ?? 0;

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
        <div className="flex items-center justify-between border-b border-zinc-800 py-3 pl-6 pr-3 md:border-b-0 md:py-4">
          <button
            onClick={() => navigate("home")}
            className="flex items-center gap-2.5 rounded-lg transition hover:opacity-80"
            title="Home"
          >
            <LogoMark />
            <span className="font-mono text-[14.5px] font-semibold tracking-[0.02em] text-zinc-50">
              hivedock
            </span>
          </button>
          <div className="md:hidden">
            <SessionControls auth={auth} onLogout={handleLogout} />
          </div>
        </div>

        <nav className="flex gap-1 overflow-x-auto px-2 pb-2 md:flex-col md:space-y-0.5 md:overflow-visible md:px-3 md:pb-0">
          {nav.map(({ id, label, Icon }) => (
            <button
              key={id}
              onClick={() => navigate(id)}
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

        <div className="mt-auto hidden space-y-2 p-3 md:block">
          <VersionLine />
          <BackendStatus />
          <SessionControls auth={auth} onLogout={handleLogout} />
        </div>
      </aside>

      <main className="min-w-0 flex-1 overflow-x-hidden px-4 py-5 sm:px-6 lg:px-8">
        <ErrorBoundary resetKey={view}>
          {view === "home" && <Dashboard />}
          {view === "stacks" && <Stacks />}
          {view === "updates" && <Updates />}
          {view === "settings" && <Settings />}
        </ErrorBoundary>
      </main>
    </div>
  );
}

// VersionLine pins the running version to the sidebar bottom. It checks for
// a newer HiveDock release on every page load; when one exists the version
// becomes an amber pill that opens the update panel — release notes link and
// a one-click "Update now" that swaps the container and reloads the page.
function VersionLine() {
  const { data } = useQuery({
    queryKey: ["app-update"],
    queryFn: fetchAppUpdate,
    staleTime: Infinity, // one check per page load
    refetchOnWindowFocus: false,
    retry: false,
  });
  const [open, setOpen] = useState(false);
  const [phase, setPhase] = useState<"idle" | "updating" | "failed">("idle");
  const [error, setError] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open || phase === "updating") return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open, phase]);

  async function startUpdate() {
    setPhase("updating");
    setError(null);
    try {
      await selfUpdate();
    } catch (err) {
      setPhase("failed");
      setError(err instanceof Error ? err.message : "Update failed to start.");
      return;
    }
    // The server is about to be replaced: poll health until the new version
    // answers, then reload. Fetch errors are expected mid-swap.
    const current = data?.current;
    const started = Date.now();
    const timer = window.setInterval(async () => {
      try {
        const h = await fetchHealth();
        if (h.version && h.version !== current) {
          window.clearInterval(timer);
          window.location.reload();
        }
      } catch {
        /* server restarting */
      }
      if (Date.now() - started > 240_000) {
        window.clearInterval(timer);
        setPhase("failed");
        setError(
          "Still on the old version. Check the server: docker logs hivedock-self-update",
        );
      }
    }, 3000);
  }

  if (!data) return null;
  // Release builds report a number ("0.2.0" -> "v0.2.0"); edge/dev builds
  // report a word — show it as-is, with a hint that update checks need a
  // release build.
  const label = /^\d/.test(data.current) ? `v${data.current}` : data.current;
  return (
    <div ref={ref} className="relative px-1">
      {data.hasUpdate ? (
        <button
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 rounded-full border border-hive-500/40 bg-hive-500/10 px-2 py-0.5 text-[11px] font-medium text-hive-500 transition hover:bg-hive-500/20"
          title={`HiveDock ${data.candidate} is available`}
        >
          {label}
          <span aria-hidden>→</span>
          {data.candidate}
        </button>
      ) : data.checkable ? (
        <button
          onClick={() => setOpen((v) => !v)}
          className="text-[11px] text-zinc-600 transition hover:text-zinc-400"
          title="HiveDock version — click for update status"
        >
          {label}
        </button>
      ) : (
        <span
          className="text-[11px] text-zinc-600"
          title={`Rolling ${data.current} build — update checks and one-click updates need a release build (the :latest image)`}
        >
          {label}
        </span>
      )}

      {open && (
        <div className="absolute bottom-7 left-0 z-30 w-64 rounded-lg border border-zinc-700 bg-zinc-900 p-3 shadow-xl">
          {data.hasUpdate ? (
            <>
              <h4 className="text-xs font-semibold text-zinc-100">
                Update HiveDock
              </h4>
              <p className="mt-1 text-[11px] leading-relaxed text-zinc-400">
                {data.current} → {data.candidate}. Pulls the new image and
                recreates the container; this page reconnects by itself
                (~30&nbsp;seconds).
              </p>
              {data.notesUrl && (
                <a
                  href={data.notesUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="mt-1.5 inline-block text-[11px] text-accent-500 hover:underline"
                >
                  Release notes ↗
                </a>
              )}
              <div className="mt-2.5 flex items-center gap-2">
                <button
                  onClick={() => void startUpdate()}
                  disabled={phase === "updating"}
                  className="rounded-lg bg-hive-500 px-2.5 py-1 text-xs font-medium text-zinc-950 transition hover:bg-hive-400 disabled:opacity-60"
                >
                  {phase === "updating" ? "Updating…" : "Update now"}
                </button>
                {phase !== "updating" && (
                  <button
                    onClick={() => setOpen(false)}
                    className="rounded-lg px-2 py-1 text-xs text-zinc-500 hover:text-zinc-300"
                  >
                    Later
                  </button>
                )}
              </div>
              {phase === "updating" && (
                <p className="mt-2 text-[11px] text-zinc-500">
                  Swapping containers — waiting for the new version…
                </p>
              )}
              {error && (
                <p className="mt-2 text-[11px] text-red-400">{error}</p>
              )}
            </>
          ) : (
            <>
              <h4 className="text-xs font-semibold text-zinc-100">
                HiveDock {label}
              </h4>
              <p className="mt-1 flex items-center gap-1.5 text-[11px] text-green-400">
                <span
                  className="inline-block h-1.5 w-1.5 rounded-full bg-green-500"
                  aria-hidden
                />
                Up to date — newest release. Checked on page load.
              </p>
              <a
                href={`https://github.com/rodzalendo/hivedock/releases/tag/v${data.current}`}
                target="_blank"
                rel="noreferrer"
                className="mt-1.5 inline-block text-[11px] text-accent-500 hover:underline"
              >
                Release notes ↗
              </a>
            </>
          )}
        </div>
      )}
    </div>
  );
}

// BackendStatus is the compact health line at the bottom of the sidebar
// (replaces the old Status page). Hover for details.
function BackendStatus() {
  const { data, isError } = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
    refetchInterval: 30_000,
  });
  const ok = !!data && !isError && data.status === "ok";
  return (
    <div
      className="flex items-center gap-2 px-1 text-[11px] text-zinc-500"
      title={
        data
          ? `Stacks dir: ${data.stacksDir}\nServer time: ${data.time}`
          : "Backend unreachable"
      }
    >
      <span
        className={`inline-block h-2 w-2 shrink-0 rounded-full ${
          ok ? "bg-green-500" : "bg-red-500"
        }`}
      />
      {ok ? "Backend ok" : "Backend unreachable"}
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
  // Trusted-proxy (forward-auth) sessions have no local session to end — the
  // proxy owns sign-out — so show the user with an SSO tag and no button.
  if (auth.viaProxy) {
    return (
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-xs text-zinc-500">
          {auth.username ?? "admin"}
        </span>
        <span className="text-[10px] text-zinc-600" title="Authenticated by your reverse proxy">
          SSO
        </span>
      </div>
    );
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
