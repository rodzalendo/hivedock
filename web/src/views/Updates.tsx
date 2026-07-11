import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchUpdates,
  checkUpdates,
  updateService,
  runStackAction,
  setImageIgnore,
  type UpdateEntry,
} from "../api";
import { SpinnerIcon } from "../components/icons";

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
  const [applyingImages, setApplyingImages] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // A completed check (updates:changed) clears the checking state and any
  // in-flight "applying" rows — the re-check after an update is what confirms
  // the new state, so this is the real end of the operation.
  useEffect(() => {
    const done = () => {
      setChecking(false);
      setApplyingImages(new Set());
    };
    window.addEventListener("hivedock:updates", done);
    return () => window.removeEventListener("hivedock:updates", done);
  }, []);

  const qc = useQueryClient();
  const entries = useMemo(() => data ?? [], [data]);
  const { available, ignored, current, other } = useMemo(() => {
    const available: UpdateEntry[] = [];
    const ignored: UpdateEntry[] = [];
    const current: UpdateEntry[] = [];
    const other: UpdateEntry[] = [];
    for (const e of entries) {
      if (e.hasUpdate && e.ignored) ignored.push(e);
      else if (e.hasUpdate) available.push(e);
      else if (e.kind === "uptodate") current.push(e);
      else other.push(e);
    }
    return { available, ignored, current, other };
  }, [entries]);

  async function toggleIgnore(e: UpdateEntry) {
    await setImageIgnore(e.image, !e.ignored);
    await qc.invalidateQueries({ queryKey: ["updates"] });
  }

  // Only semver updates can be one-click applied (digest updates aren't a tag
  // rewrite). Those are the selectable/bulk-updatable rows.
  const applicable = useMemo(
    () => available.filter((e) => e.kind === "semver" && e.candidate),
    [available],
  );
  const busy = applyingImages.size > 0;

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

  function toggleSelect(image: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(image)) next.delete(image);
      else next.add(image);
      return next;
    });
  }

  const allSelected =
    applicable.length > 0 && applicable.every((e) => selected.has(e.image));
  function toggleSelectAll() {
    setSelected(allSelected ? new Set() : new Set(applicable.map((e) => e.image)));
  }

  // Rewrite every selected update's compose files, redeploy each affected stack
  // once (a single `up` picks up all changed services in that stack), then
  // re-check to refresh status. The rows stay in "applying" state (spinner)
  // until the re-check's updates:changed event lands — that's when the new
  // state is actually confirmed — with a timeout as a safety net.
  async function applyMany(list: UpdateEntry[]) {
    const targets = list.filter((e) => e.kind === "semver" && e.candidate);
    if (targets.length === 0 || busy) return;
    setApplyingImages(new Set(targets.map((e) => e.image)));
    setNote(null);
    try {
      const stacks = new Set<string>();
      for (const e of targets) {
        for (const u of e.usedBy) {
          await updateService(u.stack, u.service, e.candidate!);
          stacks.add(u.stack);
        }
      }
      for (const s of stacks) {
        await runStackAction(s, "up");
      }
      setNote(
        `Updating ${targets.length} image${targets.length === 1 ? "" : "s"} across ${stacks.size} stack${stacks.size === 1 ? "" : "s"} — pulling & redeploying, this can take a minute…`,
      );
      setSelected(new Set());
      await checkUpdates();
      // Cleared by the updates:changed event; never spin forever.
      window.setTimeout(() => setApplyingImages(new Set()), 180_000);
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Update failed.");
      setApplyingImages(new Set());
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
          {note && (
            <span className="flex items-center gap-1.5 text-xs text-zinc-500">
              {busy && <SpinnerIcon className="h-3.5 w-3.5 text-hive-500" />}
              {note}
            </span>
          )}
          <button
            onClick={onCheck}
            disabled={checking}
            className="rounded-lg border border-zinc-700 px-3 py-1.5 text-sm font-medium text-zinc-200 transition hover:border-zinc-600 hover:bg-zinc-800 disabled:opacity-50"
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
        <div>
          <div className="mb-1.5 flex flex-wrap items-center gap-3">
            <h3 className="text-[11px] font-medium uppercase tracking-wider text-zinc-600">
              Update available
            </h3>
            {applicable.length > 0 && (
              <div className="flex items-center gap-2">
                <label className="flex items-center gap-1.5 text-xs text-zinc-400">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={toggleSelectAll}
                    disabled={busy}
                    className="accent-accent-500"
                  />
                  Select all
                </label>
                <button
                  onClick={() =>
                    applyMany(applicable.filter((e) => selected.has(e.image)))
                  }
                  disabled={busy || selected.size === 0}
                  className="rounded-lg border border-zinc-700 px-2.5 py-1 text-xs text-zinc-200 transition hover:bg-zinc-800 disabled:opacity-40"
                >
                  Update selected ({selected.size})
                </button>
                <button
                  onClick={() => applyMany(applicable)}
                  disabled={busy}
                  className="rounded-lg bg-hive-500 px-2.5 py-1 text-xs font-medium text-zinc-950 transition hover:bg-hive-400 disabled:opacity-50"
                >
                  {busy ? "Updating…" : `Update all (${applicable.length})`}
                </button>
              </div>
            )}
          </div>
          <ul className="space-y-1">
            {available.map((e) => (
              <UpdateRow
                key={e.image}
                entry={e}
                open={expanded.has(e.image)}
                onToggle={() => toggle(e.image)}
                onApply={
                  e.kind === "semver" && e.candidate
                    ? () => applyMany([e])
                    : undefined
                }
                applying={applyingImages.has(e.image)}
                selectable={e.kind === "semver" && !!e.candidate}
                checked={selected.has(e.image)}
                onCheck={() => toggleSelect(e.image)}
                onIgnore={() => toggleIgnore(e)}
                disabled={busy}
              />
            ))}
          </ul>
        </div>
      )}

      {ignored.length > 0 && (
        <div>
          <h3 className="text-[11px] font-medium uppercase tracking-wider text-zinc-600">
            Ignored
          </h3>
          <p className="mb-1.5 text-[11px] text-zinc-600">
            An update exists, but you chose to stay on the pinned tag. These are
            left out of “Update all”. Bump the tag in the compose file (or
            un-ignore) to act on them.
          </p>
          <ul className="space-y-1">
            {ignored.map((e) => (
              <UpdateRow
                key={e.image}
                entry={e}
                open={expanded.has(e.image)}
                onToggle={() => toggle(e.image)}
                onIgnore={() => toggleIgnore(e)}
              />
            ))}
          </ul>
        </div>
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
  selectable,
  checked,
  onCheck,
  onIgnore,
  disabled,
}: {
  entry: UpdateEntry;
  open: boolean;
  onToggle: () => void;
  onApply?: () => void;
  applying?: boolean;
  selectable?: boolean;
  checked?: boolean;
  onCheck?: () => void;
  onIgnore?: () => void;
  disabled?: boolean;
}) {
  return (
    <li className="rounded-lg border border-zinc-800 bg-zinc-900/40">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1.5 pr-3">
      {selectable && (
        <input
          type="checkbox"
          checked={!!checked}
          onChange={onCheck}
          disabled={disabled}
          aria-label={`Select ${entry.image}`}
          className="ml-3 accent-accent-500"
        />
      )}
      <button
        onClick={onToggle}
        className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2.5 text-left"
      >
        <span className="text-zinc-500" aria-hidden>
          {open ? "▾" : "▸"}
        </span>
        <span className="min-w-0 max-w-full shrink truncate font-mono text-sm text-zinc-200">
          {entry.image}
        </span>
        <span
          className="hidden max-w-[14rem] shrink-0 truncate text-xs text-zinc-500 sm:inline"
          title={entry.usedBy.map((u) => `${u.stack}/${u.service}`).join(", ")}
        >
          {usageLabel(entry.usedBy)}
        </span>
        <span className="min-w-0 flex-1" />

        {entry.hasUpdate && entry.kind === "semver" && (
          <span className="flex flex-wrap items-center gap-2 text-xs">
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
      </button>
        {onApply && (
          <button
            onClick={onApply}
            disabled={applying || disabled}
            className="flex shrink-0 items-center gap-1.5 rounded-lg bg-hive-500 px-2.5 py-1 text-xs font-medium text-zinc-950 transition hover:bg-hive-400 disabled:opacity-60"
          >
            {applying && <SpinnerIcon className="h-3 w-3" />}
            {applying ? "Updating…" : "Update & redeploy"}
          </button>
        )}
        {onIgnore && (
          <button
            onClick={onIgnore}
            disabled={disabled}
            title={
              entry.ignored
                ? "Start showing this update again"
                : "Keep your pinned version and exclude this from Update all"
            }
            className="shrink-0 rounded-lg border border-zinc-700 px-2.5 py-1 text-xs text-zinc-400 transition hover:bg-zinc-800 hover:text-zinc-200 disabled:opacity-50"
          >
            {entry.ignored ? "Un-ignore" : "Ignore"}
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
                className="text-accent-500 hover:underline"
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

// usageLabel summarizes where an image is used: the distinct stack names, with
// an overflow count when there are more than two.
function usageLabel(usedBy: UpdateEntry["usedBy"]): string {
  const stacks = [...new Set(usedBy.map((u) => u.stack))];
  if (stacks.length === 0) return "";
  if (stacks.length <= 2) return stacks.join(", ");
  return `${stacks[0]}, ${stacks[1]} +${stacks.length - 2}`;
}

function StatusChip({ entry }: { entry: UpdateEntry }) {
  // An unresolved env-var tag (e.g. redis:${REDIS_TAG}) can't be version-checked.
  if (entry.kind === "unsupported" && /\$[{(]/.test(entry.image)) {
    return (
      <span className="rounded bg-zinc-700/40 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-zinc-400">
        env-managed
      </span>
    );
  }
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
