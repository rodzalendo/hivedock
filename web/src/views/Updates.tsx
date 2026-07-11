import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  fetchUpdates,
  checkUpdates,
  updateService,
  runStackAction,
  type UpdateEntry,
} from "../api";

const diffColor: Record<string, string> = {
  major: "bg-red-500/15 text-red-400",
  minor: "bg-amber-500/15 text-amber-400",
  patch: "bg-sky-500/15 text-sky-400",
};

export default function Updates() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["updates"],
    queryFn: fetchUpdates,
  });
  const [checking, setChecking] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [note, setNote] = useState<string | null>(null);
  const [applying, setApplying] = useState<string | null>(null);

  // A completed check (updates:changed) clears the checking state.
  useEffect(() => {
    const done = () => setChecking(false);
    window.addEventListener("hivedock:updates", done);
    return () => window.removeEventListener("hivedock:updates", done);
  }, []);

  const entries = useMemo(() => data ?? [], [data]);
  const { available, current, other } = useMemo(() => {
    const available: UpdateEntry[] = [];
    const current: UpdateEntry[] = [];
    const other: UpdateEntry[] = [];
    for (const e of entries) {
      if (e.hasUpdate) available.push(e);
      else if (e.kind === "uptodate") current.push(e);
      else other.push(e);
    }
    return { available, current, other };
  }, [entries]);

  async function onCheck() {
    setChecking(true);
    setNote(null);
    try {
      const { images } = await checkUpdates();
      setNote(`Checking ${images} image${images === 1 ? "" : "s"}…`);
    } catch (err) {
      // 409 = a check is already running; keep the spinner, it'll clear on done.
      const msg = err instanceof Error ? err.message : "check failed";
      if (!/already running/i.test(msg)) {
        setChecking(false);
        setNote(msg);
      }
    }
    // Safety: never spin forever if no event arrives.
    window.setTimeout(() => setChecking(false), 90_000);
  }

  function toggle(image: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(image)) next.delete(image);
      else next.add(image);
      return next;
    });
  }

  // Rewrite every usage's compose file to the candidate tag, then redeploy each
  // affected stack, then re-check to refresh status.
  async function applyUpdate(entry: UpdateEntry) {
    if (!entry.candidate || applying) return;
    setApplying(entry.image);
    setNote(null);
    try {
      for (const u of entry.usedBy) {
        await updateService(u.stack, u.service, entry.candidate);
      }
      const stacks = [...new Set(entry.usedBy.map((u) => u.stack))];
      for (const s of stacks) {
        await runStackAction(s, "up");
      }
      setNote(`Updated ${entry.image} → ${entry.candidate}; redeploying…`);
      await checkUpdates();
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Update failed.");
    } finally {
      setApplying(null);
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium uppercase tracking-wide text-zinc-400">
            Updates
          </h2>
          <p className="mt-0.5 text-xs text-zinc-600">
            {available.length > 0
              ? `${available.length} update${available.length === 1 ? "" : "s"} available`
              : "Everything up to date"}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {note && <span className="text-xs text-zinc-500">{note}</span>}
          <button
            onClick={onCheck}
            disabled={checking}
            className="rounded-lg bg-hive-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-hive-500 disabled:opacity-50"
          >
            {checking ? "Checking…" : "Check now"}
          </button>
        </div>
      </div>

      {isLoading && <p className="text-sm text-zinc-500">Loading…</p>}
      {isError && (
        <p className="text-sm text-red-400">
          Failed to load updates — {(error as Error).message}
        </p>
      )}

      {!isLoading && !isError && entries.length === 0 && (
        <div className="rounded-lg border border-dashed border-zinc-800 p-6 text-sm text-zinc-500">
          No managed stacks with images to check. Create a stack, then Check now.
        </div>
      )}

      {available.length > 0 && (
        <Section title="Update available">
          {available.map((e) => (
            <UpdateRow
              key={e.image}
              entry={e}
              open={expanded.has(e.image)}
              onToggle={() => toggle(e.image)}
              onApply={e.kind === "semver" ? () => applyUpdate(e) : undefined}
              applying={applying === e.image}
            />
          ))}
        </Section>
      )}

      {current.length > 0 && (
        <Section title="Up to date">
          {current.map((e) => (
            <UpdateRow
              key={e.image}
              entry={e}
              open={expanded.has(e.image)}
              onToggle={() => toggle(e.image)}
            />
          ))}
        </Section>
      )}

      {other.length > 0 && (
        <Section title="Not checked / other">
          {other.map((e) => (
            <UpdateRow
              key={e.image}
              entry={e}
              open={expanded.has(e.image)}
              onToggle={() => toggle(e.image)}
            />
          ))}
        </Section>
      )}
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <h3 className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-zinc-600">
        {title}
      </h3>
      <ul className="space-y-1">{children}</ul>
    </div>
  );
}

function UpdateRow({
  entry,
  open,
  onToggle,
  onApply,
  applying,
}: {
  entry: UpdateEntry;
  open: boolean;
  onToggle: () => void;
  onApply?: () => void;
  applying?: boolean;
}) {
  return (
    <li className="rounded-lg border border-zinc-800 bg-zinc-900/40">
      <div className="flex items-center gap-2 pr-3">
      <button
        onClick={onToggle}
        className="flex flex-1 items-center gap-3 px-4 py-2.5 text-left"
      >
        <span className="text-zinc-500" aria-hidden>
          {open ? "▾" : "▸"}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-sm text-zinc-200">
          {entry.image}
        </span>

        {entry.hasUpdate && entry.kind === "semver" && (
          <span className="flex items-center gap-2 text-xs">
            <span className="text-zinc-500">{entry.current}</span>
            <span className="text-zinc-600">→</span>
            <span className="font-medium text-zinc-100">{entry.candidate}</span>
            {entry.diff && (
              <span
                className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${diffColor[entry.diff] ?? "bg-zinc-700/40 text-zinc-300"}`}
              >
                {entry.diff}
              </span>
            )}
          </span>
        )}
        {entry.hasUpdate && entry.kind === "digest" && (
          <span className="rounded bg-sky-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-sky-400">
            new digest
          </span>
        )}
        {!entry.hasUpdate && <StatusChip entry={entry} />}

        <span className="ml-1 text-xs text-zinc-600">
          {entry.usedBy.length} use{entry.usedBy.length === 1 ? "" : "s"}
        </span>
      </button>
        {onApply && (
          <button
            onClick={onApply}
            disabled={applying}
            className="shrink-0 rounded-lg bg-hive-600 px-2.5 py-1 text-xs font-medium text-white transition hover:bg-hive-500 disabled:opacity-50"
          >
            {applying ? "Updating…" : "Update & redeploy"}
          </button>
        )}
      </div>

      {open && (
        <div className="space-y-2 border-t border-zinc-800 px-4 py-3 text-xs text-zinc-400">
          <div>
            <span className="text-zinc-500">Used by:</span>{" "}
            {entry.usedBy.map((u) => `${u.stack}/${u.service}`).join(", ")}
          </div>
          {entry.source && (
            <div>
              <a
                href={entry.source}
                target="_blank"
                rel="noreferrer"
                className="text-hive-500 hover:underline"
              >
                Changelog / source ↗
              </a>
            </div>
          )}
          {entry.error && <div className="text-red-400">Error: {entry.error}</div>}
          {(entry.currentDigest || entry.latestDigest) && (
            <div className="space-y-0.5 font-mono text-[11px] text-zinc-500">
              {entry.currentDigest && <div>current: {entry.currentDigest}</div>}
              {entry.latestDigest && <div>latest:&nbsp; {entry.latestDigest}</div>}
            </div>
          )}
          {entry.checkedAt && (
            <div className="text-[11px] text-zinc-600">
              Checked {new Date(entry.checkedAt).toLocaleString()}
            </div>
          )}
        </div>
      )}
    </li>
  );
}

function StatusChip({ entry }: { entry: UpdateEntry }) {
  const map: Record<string, { label: string; cls: string }> = {
    uptodate: { label: "up to date", cls: "bg-green-500/15 text-green-400" },
    unchecked: { label: "not checked", cls: "bg-zinc-700/40 text-zinc-400" },
    error: { label: "error", cls: "bg-red-500/15 text-red-400" },
    unsupported: { label: "unsupported", cls: "bg-zinc-700/40 text-zinc-500" },
    digest: { label: "checked", cls: "bg-zinc-700/40 text-zinc-400" },
    semver: { label: "up to date", cls: "bg-green-500/15 text-green-400" },
  };
  const { label, cls } = map[entry.kind] ?? map.unchecked;
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${cls}`}
    >
      {label}
    </span>
  );
}
