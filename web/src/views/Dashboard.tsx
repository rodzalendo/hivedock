import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchHome,
  fetchHomeLayout,
  saveHomeLayout,
  setServiceVisibility,
  setServiceIcon,
  setServiceName,
  type HomeEntry,
  type HomeLayout,
} from "../api";
import AppIcon from "../components/AppIcon";
import HostStrip from "../components/HostStrip";
import { ChevronsDownIcon, EyeIcon, EyeOffIcon, ImageIcon } from "../components/icons";

// The pool every card lives in unless the user (or a compose label) says
// otherwise. Outside of Customize mode it renders with no header at all —
// ungrouped apps are just a dense grid filling the screen.
const DEFAULT_GROUP = "Apps";

const keyOf = (e: HomeEntry) => `${e.stack}/${e.service}`;

// Group columns on wider screens; small screens always stack to one column.
const columnsClass: Record<number, string> = {
  1: "columns-1",
  2: "columns-1 sm:columns-2",
  3: "columns-1 sm:columns-2 xl:columns-3",
  4: "columns-1 sm:columns-2 lg:columns-3 xl:columns-4",
};

// Tile size presets (the Customize slider): min card width for the ungrouped
// grid plus icon/padding/font scaling. The compact size drops the subtitle so
// titles keep room to breathe.
const tileSizes: Record<
  number,
  { minW: number; icon: number; pad: string; name: string; sub: string; showSub: boolean }
> = {
  1: { minW: 190, icon: 28, pad: "p-2", name: "text-[13px]", sub: "text-[10px]", showSub: false },
  2: { minW: 250, icon: 40, pad: "p-3", name: "text-sm", sub: "text-xs", showSub: true },
  3: { minW: 320, icon: 52, pad: "p-4", name: "text-base", sub: "text-sm", showSub: true },
};

// statusRank orders cards when sorting by status: running first, stopped
// middle, absent last; ties break alphabetically.
function statusRank(status: string): number {
  if (status === "running") return 0;
  if (status === "absent") return 2;
  return 1;
}

// groupFor resolves which group a card belongs to: the user's assignment
// wins, then an explicit compose-label group, then the default pool.
function groupFor(e: HomeEntry, layout: HomeLayout): string {
  const assigned = layout.cardGroups?.[keyOf(e)];
  if (assigned !== undefined) {
    if (assigned === "") return DEFAULT_GROUP;
    if ((layout.groups ?? []).includes(assigned) || assigned === e.group) {
      return assigned;
    }
  }
  if (e.explicitGroup && e.group) return e.group;
  return DEFAULT_GROUP;
}

// dragged is what a drag started on: a whole group, or a single card.
type Dragged = { kind: "group" | "card"; key: string } | null;

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
  const [newGroup, setNewGroup] = useState("");
  const layout: HomeLayout = useMemo(
    () => (editing ? draft : (savedLayout ?? {})),
    [editing, draft, savedLayout],
  );
  const columns = Math.min(4, Math.max(1, layout.columns ?? 1));
  const tile = tileSizes[Math.min(3, Math.max(1, layout.tileSize ?? 2))];

  const entries = useMemo(() => data ?? [], [data]);

  // Hidden sidecars per stack: shown as a collapsible sub-list under the
  // stack's visible card instead of disappearing entirely.
  const hiddenByStack = useMemo(() => {
    const map = new Map<string, HomeEntry[]>();
    for (const e of entries) {
      if (!e.hidden) continue;
      const arr = map.get(e.stack) ?? [];
      arr.push(e);
      map.set(e.stack, arr);
    }
    for (const arr of map.values()) arr.sort((a, b) => a.name.localeCompare(b.name));
    return map;
  }, [entries]);

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

  // Bucket cards into groups, sort within each, order the groups.
  const groups = useMemo(() => {
    const map = new Map<string, HomeEntry[]>();
    // While editing, user groups render even when empty so cards can be
    // dropped into them.
    if (editing) {
      map.set(DEFAULT_GROUP, []);
      for (const g of layout.groups ?? []) map.set(g, []);
    }
    for (const e of filtered) {
      const g = groupFor(e, layout);
      const arr = map.get(g) ?? [];
      arr.push(e);
      map.set(g, arr);
    }

    const byName = (a: HomeEntry, b: HomeEntry) => a.name.localeCompare(b.name);
    const byStatus = (a: HomeEntry, b: HomeEntry) =>
      statusRank(a.status) - statusRank(b.status) || byName(a, b);
    for (const [g, arr] of map.entries()) {
      if (layout.sort === "manual") {
        const order = layout.cardOrder?.[g] ?? [];
        arr.sort((a, b) => {
          const ia = order.indexOf(keyOf(a));
          const ib = order.indexOf(keyOf(b));
          if (ia >= 0 && ib >= 0) return ia - ib;
          if (ia >= 0) return -1;
          if (ib >= 0) return 1;
          return byName(a, b);
        });
      } else if (layout.sort === "status") {
        arr.sort(byStatus);
      } else {
        arr.sort(byName);
      }
    }

    const order = layout.groupOrder ?? [];
    const keys = [...map.keys()].sort((a, b) => {
      const ia = order.indexOf(a);
      const ib = order.indexOf(b);
      if (ia >= 0 && ib >= 0) return ia - ib;
      if (ia >= 0) return -1;
      if (ib >= 0) return 1;
      if (a === DEFAULT_GROUP) return -1;
      if (b === DEFAULT_GROUP) return 1;
      return a.localeCompare(b);
    });
    return keys.map((k) => [k, map.get(k)!] as const);
  }, [filtered, layout, editing]);

  // Outside Customize, the default pool renders headerless as a dense grid;
  // only user/label groups get section chrome.
  const ungrouped = groups.find(([k]) => k === DEFAULT_GROUP)?.[1] ?? [];
  const namedGroups = groups.filter(([k]) => k !== DEFAULT_GROUP);

  const hiddenCount = entries.filter((e) => e.hidden).length;
  const userGroups = layout.groups ?? [];

  function startEdit() {
    setDraft({
      columns,
      tileSize: savedLayout?.tileSize ?? 2,
      sort: savedLayout?.sort ?? "name",
      groups: [...(savedLayout?.groups ?? [])],
      cardGroups: { ...(savedLayout?.cardGroups ?? {}) },
      cardOrder: { ...(savedLayout?.cardOrder ?? {}) },
      groupOrder: groups.map(([k]) => k),
    });
    setNewGroup("");
    setEditing(true);
  }

  // resetLayout wipes every customization back to the defaults: no groups,
  // dense ungrouped grid, name sort. Takes effect on Save.
  function resetLayout() {
    setDraft({
      columns: 1,
      tileSize: 2,
      sort: "name",
      groups: [],
      cardGroups: {},
      cardOrder: {},
      groupOrder: [],
    });
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

  function addGroup() {
    const name = newGroup.trim();
    if (!name || name === DEFAULT_GROUP) return;
    setDraft((d) => {
      if ((d.groups ?? []).includes(name)) return d;
      return {
        ...d,
        groups: [...(d.groups ?? []), name],
        groupOrder: [...(d.groupOrder ?? []), name],
      };
    });
    setNewGroup("");
  }

  // renameGroup remaps every reference to the old name (commits on blur).
  function renameGroup(from: string, to: string) {
    const next = to.trim();
    if (!next || next === from || next === DEFAULT_GROUP) return;
    setDraft((d) => {
      if ((d.groups ?? []).includes(next)) return d; // avoid collisions
      const cardGroups: Record<string, string> = {};
      for (const [k, v] of Object.entries(d.cardGroups ?? {})) {
        cardGroups[k] = v === from ? next : v;
      }
      const cardOrder: Record<string, string[]> = {};
      for (const [g, arr] of Object.entries(d.cardOrder ?? {})) {
        cardOrder[g === from ? next : g] = arr;
      }
      return {
        ...d,
        groups: (d.groups ?? []).map((g) => (g === from ? next : g)),
        groupOrder: (d.groupOrder ?? []).map((g) => (g === from ? next : g)),
        cardGroups,
        cardOrder,
      };
    });
  }

  // deleteGroup sends its cards back to the default pool.
  function deleteGroup(name: string) {
    setDraft((d) => {
      const cardGroups: Record<string, string> = {};
      for (const [k, v] of Object.entries(d.cardGroups ?? {})) {
        if (v !== name) cardGroups[k] = v;
      }
      const cardOrder = { ...(d.cardOrder ?? {}) };
      delete cardOrder[name];
      return {
        ...d,
        groups: (d.groups ?? []).filter((g) => g !== name),
        groupOrder: (d.groupOrder ?? []).filter((g) => g !== name),
        cardGroups,
        cardOrder,
      };
    });
  }

  // placeCard moves a card into a group at a specific position (before
  // `beforeKey`, or appended). Manual position only shows under manual sort,
  // so a same-group reorder flips the sort mode to manual automatically.
  function placeCard(cardKey: string, group: string, beforeKey?: string) {
    setDraft((d) => {
      const cardOrder: Record<string, string[]> = {};
      for (const [g, arr] of Object.entries(d.cardOrder ?? {})) {
        cardOrder[g] = arr.filter((k) => k !== cardKey);
      }
      const target = [...(cardOrder[group] ?? [])];
      // Seed the target order from what's currently displayed so a first
      // drag doesn't scramble the rest of the group.
      if (target.length === 0) {
        const shown = groups.find(([g]) => g === group)?.[1] ?? [];
        for (const e of shown) {
          if (keyOf(e) !== cardKey) target.push(keyOf(e));
        }
      }
      const idx = beforeKey ? target.indexOf(beforeKey) : -1;
      if (idx >= 0) target.splice(idx, 0, cardKey);
      else target.push(cardKey);
      cardOrder[group] = target;

      const fromGroup = Object.entries(d.cardGroups ?? {}).find(([k]) => k === cardKey)?.[1];
      const sameGroup = (fromGroup ?? "") === (group === DEFAULT_GROUP ? "" : group);
      return {
        ...d,
        cardGroups: {
          ...(d.cardGroups ?? {}),
          [cardKey]: group === DEFAULT_GROUP ? "" : group,
        },
        cardOrder,
        // Reordering within a group only means something under manual sort.
        sort: sameGroup && beforeKey ? "manual" : d.sort,
      };
    });
  }

  // One shared drag state for both group headers and cards.
  const dragged = useRef<Dragged>(null);

  function dropOnGroup(target: string) {
    const d = dragged.current;
    dragged.current = null;
    if (!d) return;
    if (d.kind === "card") {
      placeCard(d.key, target);
      return;
    }
    if (d.key === target) return;
    setDraft((prev) => {
      const order = [...(prev.groupOrder ?? [])];
      const fi = order.indexOf(d.key);
      const ti = order.indexOf(target);
      if (fi < 0 || ti < 0) return prev;
      order.splice(fi, 1);
      order.splice(ti, 0, d.key);
      return { ...prev, groupOrder: order };
    });
  }

  function dropOnCard(group: string, beforeKey: string) {
    const d = dragged.current;
    dragged.current = null;
    if (!d || d.kind !== "card" || d.key === beforeKey) return;
    placeCard(d.key, group, beforeKey);
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
            <form
              onSubmit={(e) => {
                e.preventDefault();
                addGroup();
              }}
              className="flex items-center gap-1.5"
            >
              <input
                value={newGroup}
                onChange={(e) => setNewGroup(e.target.value)}
                placeholder="New group name…"
                className="w-36 rounded border border-zinc-700 bg-zinc-900 px-2 py-1 text-zinc-200 placeholder-zinc-600 outline-none focus:border-accent-500"
              />
              <button
                type="submit"
                disabled={!newGroup.trim()}
                className="rounded-lg border border-zinc-700 px-2 py-1 text-zinc-300 transition hover:bg-zinc-800 disabled:opacity-40"
              >
                + Add group
              </button>
            </form>
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
                <option value="manual">manual</option>
              </select>
            </label>
            <label
              className="flex items-center gap-1.5 text-zinc-400"
              title="Tile size"
            >
              Size
              <input
                type="range"
                min={1}
                max={3}
                step={1}
                value={draft.tileSize ?? 2}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, tileSize: Number(e.target.value) }))
                }
                className="w-20 accent-accent-500"
              />
            </label>
            <button
              onClick={resetLayout}
              disabled={saving}
              title="Clear all groups, assignments, and ordering (applies on Save)"
              className="rounded-lg border border-zinc-700 px-2 py-1.5 text-zinc-400 transition hover:bg-zinc-800 hover:text-zinc-200 disabled:opacity-40"
            >
              Reset
            </button>
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
            title="Create groups, drag apps between them, arrange columns and order"
          >
            Customize
          </button>
        )}
      </div>

      {editing && (
        <p className="text-xs text-zinc-500">
          Drag an app onto a group (or onto another app to set its position —
          this switches sorting to manual). Drag a group header to reorder
          groups, rename inline, then Save layout.
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

      {!editing && ungrouped.length > 0 && (
        // Ungrouped apps: a headerless dense grid that fills the screen.
        <div
          className="grid gap-3"
          style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${tile.minW}px, 1fr))` }}
        >
          {ungrouped.map((e) => (
            <Card
              key={keyOf(e)}
              entry={e}
              tile={tile}
              hiddenSiblings={hiddenByStack.get(e.stack)}
            />
          ))}
        </div>
      )}

      {editing && (
        // In Customize, the ungrouped pool is a full-width dense grid too —
        // just with visible chrome so it works as a drop target.
        <section
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault();
            dropOnGroup(DEFAULT_GROUP);
          }}
          className="rounded-xl border border-dashed border-zinc-700 bg-zinc-900/30 p-3"
        >
          <div className="mb-2 flex items-center gap-2">
            <h3 className="text-[11px] font-medium uppercase tracking-wider text-zinc-500">
              Ungrouped
            </h3>
            <span className="text-[10px] text-zinc-600">
              (drop an app here to remove it from its group)
            </span>
          </div>
          <div
            className="grid gap-3"
            style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${tile.minW}px, 1fr))` }}
          >
            {ungrouped.map((e) => (
              <div
                key={keyOf(e)}
                draggable
                onDragStart={(ev) => {
                  ev.stopPropagation();
                  dragged.current = { kind: "card", key: keyOf(e) };
                }}
                onDragOver={(ev) => {
                  ev.preventDefault();
                  ev.stopPropagation();
                }}
                onDrop={(ev) => {
                  ev.preventDefault();
                  ev.stopPropagation();
                  dropOnCard(DEFAULT_GROUP, keyOf(e));
                }}
                className="cursor-grab"
              >
                <Card
                  entry={e}
                  editing
                  tile={tile}
                  hiddenSiblings={hiddenByStack.get(e.stack)}
                />
              </div>
            ))}
          </div>
        </section>
      )}

      {namedGroups.length > 0 ? (
        <div className={`${columnsClass[columns]} gap-4`}>
          {namedGroups.map(([key, items]) => (
            <section
              key={key}
              draggable={editing}
              onDragStart={() => (dragged.current = { kind: "group", key })}
              onDragOver={(e) => editing && e.preventDefault()}
              onDrop={(e) => {
                if (!editing) return;
                e.preventDefault();
                dropOnGroup(key);
              }}
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
                {editing && userGroups.includes(key) ? (
                  <>
                    <GroupTitle name={key} onRename={(next) => renameGroup(key, next)} />
                    <button
                      onClick={() => deleteGroup(key)}
                      title="Delete this group (its apps return to the default pool)"
                      className="rounded px-1 text-zinc-600 hover:text-red-400"
                    >
                      ✕
                    </button>
                  </>
                ) : (
                  <h3 className="text-[11px] font-medium uppercase tracking-wider text-zinc-500">
                    {key}
                  </h3>
                )}
                {editing && items.length === 0 && (
                  <span className="text-[10px] text-zinc-600">(drop apps here)</span>
                )}
              </div>
              <div className="space-y-2">
                {items.map((e) => (
                  <div
                    key={keyOf(e)}
                    draggable={editing}
                    onDragStart={(ev) => {
                      ev.stopPropagation();
                      dragged.current = { kind: "card", key: keyOf(e) };
                    }}
                    onDragOver={(ev) => {
                      if (editing) {
                        ev.preventDefault();
                        ev.stopPropagation();
                      }
                    }}
                    onDrop={(ev) => {
                      if (!editing) return;
                      ev.preventDefault();
                      ev.stopPropagation();
                      dropOnCard(key, keyOf(e));
                    }}
                    className={editing ? "cursor-grab" : ""}
                  >
                    <Card
                      entry={e}
                      editing={editing}
                      tile={tile}
                      hiddenSiblings={hiddenByStack.get(e.stack)}
                    />
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
      ) : null}
    </div>
  );
}

// GroupTitle is an inline rename input that commits on blur or Enter.
function GroupTitle({
  name,
  onRename,
}: {
  name: string;
  onRename: (next: string) => void;
}) {
  const [value, setValue] = useState(name);
  return (
    <input
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={() => onRename(value)}
      onKeyDown={(e) => {
        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
        if (e.key === "Escape") setValue(name);
      }}
      className="w-36 rounded border border-zinc-700 bg-zinc-950 px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-zinc-300 outline-none focus:border-accent-500"
    />
  );
}

function Card({
  entry,
  editing,
  tile = tileSizes[2],
  hiddenSiblings,
}: {
  entry: HomeEntry;
  editing?: boolean;
  tile?: (typeof tileSizes)[number];
  hiddenSiblings?: HomeEntry[];
}) {
  const qc = useQueryClient();
  const [menuOpen, setMenuOpen] = useState(false);
  const [subOpen, setSubOpen] = useState(false);
  const toggleHidden = useMutation({
    mutationFn: () => setServiceVisibility(entry.stack, entry.service, !entry.hidden),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["home"] }),
  });

  // Hidden sidecars of the same stack, minus this card itself (when it is a
  // hidden card revealed via "Show hidden").
  const subs = (hiddenSiblings ?? []).filter((s) => keyOf(s) !== keyOf(entry));

  const clickable = !!entry.url && !editing;
  const inner = (
    <>
      <AppIcon entry={entry} size={tile.icon} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className={`truncate font-medium text-zinc-100 ${tile.name}`}>
            {entry.name}
          </span>
          <StatusDotSmall status={entry.status} />
        </div>
        {tile.showSub && (
          <div className={`truncate text-zinc-500 ${tile.sub}`}>
            {entry.description || entry.stack}
          </div>
        )}
      </div>
    </>
  );

  return (
    <div
      className={`group relative rounded-xl border border-zinc-800 bg-zinc-900/40 transition hover:border-zinc-700 ${
        entry.hidden ? "opacity-60" : ""
      }`}
    >
      <div className={`flex items-center gap-3 ${tile.pad}`}>
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
          {subs.length > 0 && (
            <button
              onClick={() => setSubOpen((v) => !v)}
              title={`${subs.length} more container${subs.length === 1 ? "" : "s"} in this stack`}
              className={`flex items-center gap-1 rounded px-1.5 py-1 text-xs text-zinc-500 transition hover:bg-zinc-800 hover:text-zinc-300 ${
                subOpen ? "text-zinc-300" : ""
              }`}
            >
              {subs.length}
              <ChevronsDownIcon
                className={`h-3.5 w-3.5 transition-transform ${subOpen ? "rotate-180" : ""}`}
              />
            </button>
          )}
          {!editing && entry.ports && entry.ports.length > 1 && (
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
          {!editing && <CardEditor entry={entry} />}
          {!editing && (
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
          )}
        </div>
      </div>

      {subOpen && subs.length > 0 && (
        <ul className="border-t border-zinc-800/60 px-3 py-2">
          {subs.map((s) => (
            <SubRow key={keyOf(s)} entry={s} />
          ))}
        </ul>
      )}
    </div>
  );
}

// SubRow is a compact line for a hidden sidecar (db, worker, …) revealed via
// the chevron on its stack's visible card.
function SubRow({ entry }: { entry: HomeEntry }) {
  const content = (
    <>
      <AppIcon entry={entry} size={20} />
      <span className="truncate text-xs text-zinc-300">{entry.name}</span>
      <span className="truncate font-mono text-[10px] text-zinc-600">{entry.service}</span>
      <span className="flex-1" />
      <StatusDotSmall status={entry.status} />
    </>
  );
  return (
    <li>
      {entry.url ? (
        <a
          href={entry.url}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-2 rounded-md px-1.5 py-1 hover:bg-zinc-800/60"
        >
          {content}
        </a>
      ) : (
        <div className="flex items-center gap-2 rounded-md px-1.5 py-1">{content}</div>
      )}
    </li>
  );
}

// CardEditor lets the user rename a card and set a custom icon (image URL or
// dashboard-icons slug), or reset either to the automatic value. Persisted
// server-side.
function CardEditor({ entry }: { entry: HomeEntry }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(entry.name);
  const [icon, setIcon] = useState(entry.icon ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      // A changed (or cleared) name updates the override; clearing reverts
      // to the automatic name.
      if (name.trim() !== entry.name) {
        await setServiceName(entry.stack, entry.service, name.trim());
      }
      if (icon.trim() !== (entry.icon ?? "")) {
        await setServiceIcon(entry.stack, entry.service, icon.trim());
      }
      await qc.invalidateQueries({ queryKey: ["home"] });
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="relative">
      <button
        onClick={() => {
          setName(entry.name);
          setIcon(entry.icon ?? "");
          setError(null);
          setOpen((v) => !v);
        }}
        className="rounded px-1.5 py-1 text-zinc-600 opacity-0 transition hover:bg-zinc-800 hover:text-zinc-300 group-hover:opacity-100"
        title="Edit name & icon"
      >
        <ImageIcon className="h-4 w-4" />
      </button>
      {open && (
        <div className="absolute right-0 z-20 mt-1 w-64 rounded-lg border border-zinc-700 bg-zinc-900 p-3 shadow-xl">
          <label className="mb-1 block text-[11px] font-medium text-zinc-400">
            Display name
          </label>
          <input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void save();
              if (e.key === "Escape") setOpen(false);
            }}
            placeholder="Automatic"
            className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-xs outline-none focus:border-accent-500"
          />
          <label className="mb-1 mt-2 block text-[11px] font-medium text-zinc-400">
            Icon URL or slug
          </label>
          <input
            value={icon}
            onChange={(e) => setIcon(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void save();
              if (e.key === "Escape") setOpen(false);
            }}
            placeholder="https://…/icon.png  or  jellyfin"
            className="w-full rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-xs outline-none focus:border-accent-500"
          />
          <p className="mt-1 text-[10px] leading-snug text-zinc-600">
            Icon: a full image URL, or a{" "}
            <a
              href="https://github.com/homarr-labs/dashboard-icons"
              target="_blank"
              rel="noreferrer"
              className="text-accent-500 hover:underline"
            >
              dashboard-icons
            </a>{" "}
            name. Leave either field empty to go back to automatic.
          </p>
          <div className="mt-2 flex items-center gap-2">
            <button
              onClick={() => void save()}
              disabled={busy}
              className="rounded-md bg-accent-600 px-2.5 py-1 text-xs font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-50"
            >
              {busy ? "Saving…" : "Save"}
            </button>
            <button
              onClick={() => setOpen(false)}
              className="ml-auto rounded-md px-2 py-1 text-xs text-zinc-500 hover:text-zinc-300"
            >
              Cancel
            </button>
          </div>
          {error && <p className="mt-1 text-[10px] text-red-400">{error}</p>}
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
