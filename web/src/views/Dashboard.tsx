import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchHome,
  fetchHomeLayout,
  saveHomeLayout,
  setServiceVisibility,
  setServiceIcon,
  type HomeEntry,
  type HomeLayout,
} from "../api";
import AppIcon from "../components/AppIcon";
import HostStrip from "../components/HostStrip";
import { EyeIcon, EyeOffIcon, ImageIcon } from "../components/icons";

// Group columns on wider screens; small screens always stack to one column.
const columnsClass: Record<number, string> = {
  1: "columns-1",
  2: "columns-1 sm:columns-2",
  3: "columns-1 sm:columns-2 xl:columns-3",
  4: "columns-1 sm:columns-2 lg:columns-3 xl:columns-4",
};

// statusRank orders cards when sorting by status: running first, stopped
// middle, absent last; ties break alphabetically.
function statusRank(status: string): number {
  if (status === "running") return 0;
  if (status === "absent") return 2;
  return 1;
}

export default function Dashboard() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["home"],
    queryFn: fetchHome,
    refetchInterval: 30_000,
  });
  const { data: savedLayout } = useQuery({
    queryKey: ["home-layout"],
    queryFn: fetchHomeLayout,
    staleTime: 60_000,
  });
  const qc = useQueryClient();

  const [search, setSearch] = useState("");
  const [showHidden, setShowHidden] = useState(false);

  // Customize mode edits a draft copy; Save persists it, Cancel discards.
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<HomeLayout>({});
  const [saving, setSaving] = useState(false);
  const layout: HomeLayout = editing ? draft : (savedLayout ?? {});
  const columns = Math.min(4, Math.max(1, layout.columns ?? 3));

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

  // Group, sort within groups, then order the groups per the layout.
  const groups = useMemo(() => {
    const map = new Map<string, HomeEntry[]>();
    for (const e of filtered) {
      const arr = map.get(e.group) ?? [];
      arr.push(e);
      map.set(e.group, arr);
    }
    const bySort =
      layout.sort === "status"
        ? (a: HomeEntry, b: HomeEntry) =>
            statusRank(a.status) - statusRank(b.status) ||
            a.name.localeCompare(b.name)
        : (a: HomeEntry, b: HomeEntry) => a.name.localeCompare(b.name);
    for (const arr of map.values()) arr.sort(bySort);

    const order = layout.groupOrder ?? [];
    const keys = [...map.keys()].sort((a, b) => {
      const ia = order.indexOf(a);
      const ib = order.indexOf(b);
      if (ia >= 0 && ib >= 0) return ia - ib;
      if (ia >= 0) return -1;
      if (ib >= 0) return 1;
      return a.localeCompare(b);
    });
    return keys.map((k) => [k, map.get(k)!] as const);
  }, [filtered, layout.sort, layout.groupOrder]);

  const hiddenCount = entries.filter((e) => e.hidden).length;

  function startEdit() {
    setDraft({
      columns,
      sort: layout.sort ?? "name",
      groupTitles: { ...(savedLayout?.groupTitles ?? {}) },
      // Seed the order with what's currently displayed so dragging starts
      // from a complete list.
      groupOrder: groups.map(([k]) => k),
    });
    setEditing(true);
  }

  async function saveEdit() {
    setSaving(true);
    try {
      await saveHomeLayout(draft);
      await qc.invalidateQueries({ queryKey: ["home-layout"] });
      setEditing(false);
    } finally {
      setSaving(false);
    }
  }

  // HTML5 drag-and-drop group reordering (edit mode only).
  const dragFrom = useRef<string | null>(null);
  function dropOn(target: string) {
    const from = dragFrom.current;
    dragFrom.current = null;
    if (!from || from === target) return;
    setDraft((d) => {
      const order = [...(d.groupOrder ?? [])];
      const fi = order.indexOf(from);
      const ti = order.indexOf(target);
      if (fi < 0 || ti < 0) return d;
      order.splice(fi, 1);
      order.splice(ti, 0, from);
      return { ...d, groupOrder: order };
    });
  }

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

        <span className="flex-1" />

        {editing ? (
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <label className="flex items-center gap-1.5 text-zinc-400">
              Columns
              <select
                value={columns}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, columns: Number(e.target.value) }))
                }
                className="rounded border border-zinc-700 bg-zinc-900 px-1.5 py-1 text-zinc-200"
              >
                {[1, 2, 3, 4].map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex items-center gap-1.5 text-zinc-400">
              Sort by
              <select
                value={draft.sort ?? "name"}
                onChange={(e) =>
                  setDraft((d) => ({
                    ...d,
                    sort: e.target.value as HomeLayout["sort"],
                  }))
                }
                className="rounded border border-zinc-700 bg-zinc-900 px-1.5 py-1 text-zinc-200"
              >
                <option value="name">name</option>
                <option value="status">status</option>
              </select>
            </label>
            <button
              onClick={saveEdit}
              disabled={saving}
              className="rounded-lg bg-accent-600 px-3 py-1.5 font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-50"
            >
              {saving ? "Saving…" : "Save layout"}
            </button>
            <button
              onClick={() => setEditing(false)}
              disabled={saving}
              className="rounded-lg px-2 py-1.5 text-zinc-400 hover:text-zinc-200"
            >
              Cancel
            </button>
          </div>
        ) : (
          <button
            onClick={startEdit}
            className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 transition hover:bg-zinc-800"
            title="Arrange groups: columns, order (drag & drop), titles, sorting"
          >
            Customize
          </button>
        )}
      </div>

      {editing && (
        <p className="text-xs text-zinc-500">
          Drag groups to reorder them, click a title to rename it, then Save
          layout.
        </p>
      )}

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

      {groups.length > 0 && (
        <div className={`${columnsClass[columns]} gap-4`}>
          {groups.map(([key, items]) => (
            <section
              key={key}
              draggable={editing}
              onDragStart={() => (dragFrom.current = key)}
              onDragOver={(e) => editing && e.preventDefault()}
              onDrop={() => editing && dropOn(key)}
              className={`mb-4 break-inside-avoid ${
                editing
                  ? "cursor-grab rounded-xl border border-dashed border-zinc-700 bg-zinc-900/30 p-3"
                  : ""
              }`}
            >
              <div className="mb-2 flex items-center gap-2">
                {editing && (
                  <span className="select-none text-zinc-600" aria-hidden>
                    ⠿
                  </span>
                )}
                {editing ? (
                  <input
                    value={draft.groupTitles?.[key] ?? key}
                    onChange={(e) =>
                      setDraft((d) => ({
                        ...d,
                        groupTitles: {
                          ...(d.groupTitles ?? {}),
                          [key]: e.target.value,
                        },
                      }))
                    }
                    className="w-40 rounded border border-zinc-700 bg-zinc-950 px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-zinc-300 outline-none focus:border-accent-500"
                  />
                ) : (
                  <h3 className="text-[11px] font-medium uppercase tracking-wider text-zinc-500">
                    {layout.groupTitles?.[key]?.trim() || key}
                  </h3>
                )}
              </div>
              <div className="space-y-2">
                {items.map((e) => (
                  <Card key={`${e.stack}/${e.service}`} entry={e} />
                ))}
              </div>
            </section>
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
