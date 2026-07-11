import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchHome,
  setServiceVisibility,
  setServiceIcon,
  type HomeEntry,
} from "../api";
import AppIcon from "../components/AppIcon";
import HostStrip from "../components/HostStrip";
import { EyeIcon, EyeOffIcon, ImageIcon } from "../components/icons";

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

      {filtered.length > 0 && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
          {filtered.map((e) => (
            <Card key={`${e.stack}/${e.service}`} entry={e} />
          ))}
        </div>
      )}
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
        <IconEditor entry={entry} />
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

// IconEditor lets the user set a custom icon (image URL or dashboard-icons
// slug) for a card, or reset to the automatic one. Persisted server-side.
function IconEditor({ entry }: { entry: HomeEntry }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState(entry.icon ?? "");

  const save = useMutation({
    mutationFn: (icon: string) => setServiceIcon(entry.stack, entry.service, icon),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["home"] });
      setOpen(false);
    },
  });

  return (
    <div className="relative">
      <button
        onClick={() => {
          setValue(entry.icon ?? "");
          setOpen((v) => !v);
        }}
        className="rounded px-1.5 py-1 text-zinc-600 opacity-0 transition hover:bg-zinc-800 hover:text-zinc-300 group-hover:opacity-100"
        title="Set icon"
      >
        <ImageIcon className="h-4 w-4" />
      </button>
      {open && (
        <div className="absolute right-0 z-20 mt-1 w-64 rounded-lg border border-zinc-700 bg-zinc-900 p-3 shadow-xl">
          <label className="mb-1 block text-[11px] font-medium text-zinc-400">
            Icon URL or slug
          </label>
          <input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") save.mutate(value.trim());
              if (e.key === "Escape") setOpen(false);
            }}
            placeholder="https://…/icon.png  or  jellyfin"
            className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-xs outline-none focus:border-accent-500"
          />
          <p className="mt-1 text-[10px] leading-snug text-zinc-600">
            A full image URL, or a{" "}
            <a
              href="https://github.com/homarr-labs/dashboard-icons"
              target="_blank"
              rel="noreferrer"
              className="text-accent-500 hover:underline"
            >
              dashboard-icons
            </a>{" "}
            name. Leave empty to auto-detect.
          </p>
          <div className="mt-2 flex items-center gap-2">
            <button
              onClick={() => save.mutate(value.trim())}
              disabled={save.isPending}
              className="rounded-md bg-accent-600 px-2.5 py-1 text-xs font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-50"
            >
              Save
            </button>
            {entry.icon && (
              <button
                onClick={() => save.mutate("")}
                disabled={save.isPending}
                className="rounded-md px-2 py-1 text-xs text-zinc-400 hover:text-zinc-200 disabled:opacity-50"
              >
                Reset
              </button>
            )}
            <button
              onClick={() => setOpen(false)}
              className="ml-auto rounded-md px-2 py-1 text-xs text-zinc-500 hover:text-zinc-300"
            >
              Cancel
            </button>
          </div>
          {save.isError && (
            <p className="mt-1 text-[10px] text-red-400">
              {(save.error as Error).message}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function StatusDotSmall({ status }: { status: string }) {
  const color =
    status === "running" ? "bg-green-500" : status === "absent" ? "bg-zinc-600" : "bg-amber-500";
  return <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${color}`} title={status} />;
}
