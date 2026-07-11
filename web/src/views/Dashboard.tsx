import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchHome, setServiceVisibility, type HomeEntry } from "../api";
import AppIcon from "../components/AppIcon";
import HostStrip from "../components/HostStrip";
import { EyeIcon, EyeOffIcon } from "../components/icons";

export default function Dashboard() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["home"],
    queryFn: fetchHome,
    refetchInterval: 30_000,
  });
  const [search, setSearch] = useState("");
  const [showHidden, setShowHidden] = useState(false);

  const entries = useMemo(() => data ?? [], [data]);
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return entries.filter((e) => {
      if (!showHidden && e.hidden) return false;
      if (!q) return true;
      return (
        e.name.toLowerCase().includes(q) ||
        e.group.toLowerCase().includes(q) ||
        e.stack.toLowerCase().includes(q)
      );
    });
  }, [entries, search, showHidden]);

  const groups = useMemo(() => groupBy(filtered), [filtered]);
  const hiddenCount = entries.filter((e) => e.hidden).length;

  return (
    <div className="space-y-5">
      <HostStrip />

      <div className="flex flex-wrap items-center gap-3">
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search apps…"
          className="w-56 rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-200 placeholder-zinc-600 focus:border-accent-500 focus:outline-none"
        />
        {hiddenCount > 0 && (
          <label className="flex items-center gap-1.5 text-xs text-zinc-400">
            <input
              type="checkbox"
              checked={showHidden}
              onChange={(e) => setShowHidden(e.target.checked)}
              className="accent-accent-500"
            />
            Show hidden ({hiddenCount})
          </label>
        )}
      </div>

      {isLoading && <p className="text-sm text-zinc-500">Loading…</p>}
      {isError && (
        <p className="text-sm text-red-400">Failed to load — {(error as Error).message}</p>
      )}

      {!isLoading && !isError && entries.length === 0 && (
        <div className="rounded-xl border border-dashed border-zinc-800 p-10 text-center text-sm text-zinc-500">
          No apps found. Add a compose stack with a published port, or start a
          container — it’ll appear here automatically.
        </div>
      )}

      {groups.map(([group, items]) => (
        <section key={group}>
          <h3 className="mb-2 text-[11px] font-medium uppercase tracking-wider text-zinc-500">
            {group}
          </h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {items.map((e) => (
              <Card key={`${e.stack}/${e.service}`} entry={e} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function Card({ entry }: { entry: HomeEntry }) {
  const qc = useQueryClient();
  const [menuOpen, setMenuOpen] = useState(false);
  const toggleHidden = useMutation({
    mutationFn: () => setServiceVisibility(entry.stack, entry.service, !entry.hidden),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["home"] }),
  });

  const clickable = !!entry.url;
  const inner = (
    <>
      <AppIcon entry={entry} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate font-medium text-zinc-100">{entry.name}</span>
          <StatusDotSmall status={entry.status} />
        </div>
        <div className="truncate text-xs text-zinc-500">
          {entry.description || entry.stack}
        </div>
      </div>
    </>
  );

  return (
    <div
      className={`group relative flex items-center gap-3 rounded-xl border border-zinc-800 bg-zinc-900/40 p-3 transition hover:border-zinc-700 ${
        entry.hidden ? "opacity-60" : ""
      }`}
    >
      {clickable ? (
        <a
          href={entry.url}
          target="_blank"
          rel="noreferrer"
          className="flex min-w-0 flex-1 items-center gap-3"
        >
          {inner}
        </a>
      ) : (
        <div className="flex min-w-0 flex-1 items-center gap-3">{inner}</div>
      )}

      <div className="flex shrink-0 items-center gap-1">
        {entry.ports && entry.ports.length > 1 && (
          <div className="relative">
            <button
              onClick={() => setMenuOpen((v) => !v)}
              className="rounded px-1.5 py-1 text-xs text-zinc-500 hover:bg-zinc-800 hover:text-zinc-300"
              title="Ports"
            >
              ⋯
            </button>
            {menuOpen && (
              <div className="absolute right-0 z-10 mt-1 w-40 rounded-lg border border-zinc-700 bg-zinc-900 py-1 shadow-xl">
                {entry.ports.map((p) => (
                  <a
                    key={p.label}
                    href={p.url}
                    target="_blank"
                    rel="noreferrer"
                    className="block px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
                  >
                    {p.label}
                  </a>
                ))}
              </div>
            )}
          </div>
        )}
        <button
          onClick={() => toggleHidden.mutate()}
          className={`rounded px-1.5 py-1 text-zinc-600 transition hover:bg-zinc-800 hover:text-zinc-300 ${
            entry.hidden ? "" : "opacity-0 group-hover:opacity-100"
          }`}
          title={entry.hidden ? "Show on dashboard" : "Hide from dashboard"}
        >
          {entry.hidden ? (
            <EyeOffIcon className="h-4 w-4" />
          ) : (
            <EyeIcon className="h-4 w-4" />
          )}
        </button>
      </div>
    </div>
  );
}

function StatusDotSmall({ status }: { status: string }) {
  const color =
    status === "running" ? "bg-green-500" : status === "absent" ? "bg-zinc-600" : "bg-amber-500";
  return <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${color}`} title={status} />;
}

function groupBy(entries: HomeEntry[]): [string, HomeEntry[]][] {
  const map = new Map<string, HomeEntry[]>();
  for (const e of entries) {
    const arr = map.get(e.group) ?? [];
    arr.push(e);
    map.set(e.group, arr);
  }
  return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
}
