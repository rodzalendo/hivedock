import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchUpdates,
  checkUpdates,
  previewUpdate,
  applyUpdate,
  runStackAction,
  setImageIgnore,
  type UpdateEntry,
} from "../api";
import { SpinnerIcon } from "../components/icons";
import { HelpTip } from "../components/ui";

// One planned compose rewrite awaiting the user's confirmation in the review
// modal — the diff to show and the base hash to lock the apply to (§5.2).
type PlannedEdit = {
  stack: string;
  service: string;
  image: string;
  tag: string;
  diff: string;
  sha256: string;
};

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
  // The pending review: compose diffs to confirm before anything is written.
  const [review, setReview] = useState<PlannedEdit[] | null>(null);

  // A completed check (updates:changed) clears the checking state and any
  // in-flight "applying" rows — the re-check after an update is what confirms
  // the new state, so this is the real end of the operation.
  useEffect(() => {
    const done = () => {
      setChecking(false);
      setApplyingImages(new Set());
      // Clear any progress note ("Checking N images…") so the header falls
      // back to "Last checked just now" instead of a stale in-progress line.
      setNote(null);
    };
    window.addEventListener("hivedock:updates", done);
    return () => window.removeEventListener("hivedock:updates", done);
  }, []);

  const qc = useQueryClient();
  const entries = useMemo(() => data ?? [], [data]);

  // Most recent check across all images (drives "Last checked X ago").
  const lastChecked = useMemo(() => {
    let max = "";
    for (const e of entries) {
      if (e.checkedAt && e.checkedAt > max) max = e.checkedAt;
    }
    return max ? new Date(max) : null;
  }, [entries]);
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

  // Digest updates (a latest-style tag whose digest moved) can't be applied by
  // rewriting the tag — the fix is `compose up --pull always` on each stack
  // using the image. Track those stacks and re-check once their deploys end,
  // which confirms the new digest and clears the row spinners.
  const pendingStacks = useRef<Set<string>>(new Set());
  useEffect(() => {
    const onDeploy = (ev: Event) => {
      const msg = (ev as CustomEvent).detail as {
        type: string;
        payload?: { stack?: string };
      };
      if (msg.type !== "deploy:end" || !msg.payload?.stack) return;
      if (!pendingStacks.current.delete(msg.payload.stack)) return;
      if (pendingStacks.current.size === 0) {
        checkUpdates().catch(() => {});
      }
    };
    window.addEventListener("hivedock:deploy", onDeploy);
    return () => window.removeEventListener("hivedock:deploy", onDeploy);
  }, []);

  async function applyDigest(e: UpdateEntry) {
    if (busy) return;
    setApplyingImages((prev) => new Set(prev).add(e.image));
    setNote(null);
    try {
      const stacks = [...new Set(e.usedBy.map((u) => u.stack))];
      for (const s of stacks) {
        pendingStacks.current.add(s);
        await runStackAction(s, "update");
      }
      setNote("Pulling the new image & redeploying — this can take a minute…");
      // Cleared by the updates:changed event after the post-deploy re-check.
      window.setTimeout(() => setApplyingImages(new Set()), 180_000);
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Update failed.");
      setApplyingImages(new Set());
    }
  }

  // Step 1: gather the exact compose diffs for every selected update WITHOUT
  // writing anything, and open the review modal (§5.2). Nothing touches disk
  // until the user confirms.
  async function reviewMany(list: UpdateEntry[]) {
    const targets = list.filter((e) => e.kind === "semver" && e.candidate);
    if (targets.length === 0 || busy) return;
    setNote(null);
    try {
      const edits: PlannedEdit[] = [];
      for (const e of targets) {
        for (const u of e.usedBy) {
          const p = await previewUpdate(u.stack, u.service, e.candidate!);
          if (p.changed && p.diff) {
            edits.push({
              stack: u.stack,
              service: u.service,
              image: e.image,
              tag: e.candidate!,
              diff: p.diff,
              sha256: p.sha256,
            });
          }
        }
      }
      if (edits.length === 0) {
        setNote("Selected images are already at the target tag.");
        return;
      }
      setReview(edits);
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Couldn't prepare the update.");
    }
  }

  // Step 2: the user confirmed the diffs. Apply each rewrite (locked to the hash
  // captured at preview), then redeploy each affected stack once and re-check.
  async function confirmReview() {
    if (!review || busy) return;
    const edits = review;
    setReview(null);
    setApplyingImages(new Set(edits.map((e) => e.image)));
    setNote(null);
    try {
      const stacks = new Set<string>();
      for (const e of edits) {
        await applyUpdate(e.stack, e.service, e.tag, e.sha256);
        stacks.add(e.stack);
      }
      for (const s of stacks) {
        await runStackAction(s, "up");
      }
      setNote(
        `Updating ${edits.length} image${edits.length === 1 ? "" : "s"} across ${stacks.size} stack${stacks.size === 1 ? "" : "s"} — pulling & redeploying, this can take a minute…`,
      );
      setSelected(new Set());
      await checkUpdates();
      // Cleared by the updates:changed event; never spin forever.
      window.setTimeout(() => setApplyingImages(new Set()), 180_000);
    } catch (err) {
      // A 409 here means the file changed on disk between preview and apply.
      setNote(err instanceof Error ? err.message : "Update failed.");
      setApplyingImages(new Set());
    }
  }

  return (
    <div className="space-y-5">
      {review && (
        <ReviewModal
          edits={review}
          onCancel={() => setReview(null)}
          onConfirm={() => void confirmReview()}
        />
      )}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium uppercase tracking-wide text-zinc-400">
            Updates
          </h2>
          <p
            className={`mt-0.5 flex items-center gap-1.5 text-sm font-medium ${
              available.length > 0 ? "text-hive-500" : "text-green-400"
            }`}
          >
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                available.length > 0 ? "bg-hive-500" : "bg-green-500"
              }`}
              aria-hidden
            />
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
          {!note && lastChecked && (
            <span
              className={`text-xs ${
                timeAgo(lastChecked) === "just now"
                  ? "text-green-400"
                  : "text-zinc-600"
              }`}
              title={lastChecked.toLocaleString()}
            >
              Last checked {timeAgo(lastChecked)}
            </span>
          )}
          <button
            onClick={onCheck}
            disabled={checking}
            className={`flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition ${
              checking
                ? "border-hive-500/60 bg-hive-500/10 text-hive-400"
                : "border-zinc-700 text-zinc-200 hover:border-zinc-600 hover:bg-zinc-800"
            }`}
          >
            {checking && <SpinnerIcon className="h-3.5 w-3.5" />}
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
                    reviewMany(applicable.filter((e) => selected.has(e.image)))
                  }
                  disabled={busy || selected.size === 0}
                  className="rounded-lg border border-zinc-700 px-2.5 py-1 text-xs text-zinc-200 transition hover:bg-zinc-800 disabled:opacity-40"
                >
                  Update selected ({selected.size})
                </button>
                <button
                  onClick={() => reviewMany(applicable)}
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
                    ? () => reviewMany([e])
                    : e.kind === "digest"
                      ? () => applyDigest(e)
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
          <h3 className="mb-1.5 flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wider text-zinc-600">
            Ignored
            <HelpTip>
              An update exists, but you chose to stay on the pinned tag. These
              are left out of “Update all”. Bump the tag in the compose file
              (or un-ignore) to act on them.
            </HelpTip>
          </h3>
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
              onIgnore={() => toggleIgnore(e)}
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
              onIgnore={
                e.kind === "unsupported" ? undefined : () => toggleIgnore(e)
              }
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
            {applying
              ? "Updating…"
              : entry.kind === "digest"
                ? "Pull & redeploy"
                : "Update & redeploy"}
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

// timeAgo renders a compact relative time ("just now", "12m ago", "3h ago").
function timeAgo(d: Date): string {
  const mins = Math.floor((Date.now() - d.getTime()) / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 48) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
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

// ReviewModal shows the exact compose diffs a batch update will write, before
// anything touches disk (§5.2). Diffs render as text nodes (no innerHTML).
function ReviewModal({
  edits,
  onCancel,
  onConfirm,
}: {
  edits: PlannedEdit[];
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const stacks = new Set(edits.map((e) => e.stack)).size;
  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 p-4"
      onClick={onCancel}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-zinc-700 bg-zinc-900 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-b border-zinc-800 px-5 py-3">
          <h3 className="text-sm font-semibold text-zinc-100">
            Review {edits.length} change{edits.length === 1 ? "" : "s"}
          </h3>
          <p className="mt-0.5 text-xs text-zinc-500">
            HiveDock will rewrite the image tag in{" "}
            {edits.length === 1 ? "this compose file" : "these compose files"}{" "}
            (comments and formatting preserved), then redeploy {stacks} stack
            {stacks === 1 ? "" : "s"}. Nothing is written until you apply.
          </p>
        </div>
        <div className="space-y-3 overflow-auto px-5 py-4">
          {edits.map((e, i) => (
            <div key={`${e.stack}/${e.service}/${i}`}>
              <p className="mb-1 font-mono text-[11px] text-zinc-400">
                {e.stack}/{e.service} → {e.tag}
              </p>
              <pre className="overflow-x-auto rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-[11px] leading-relaxed">
                {e.diff.split("\n").map((line, j) => (
                  <div key={j} className={diffLineClass(line)}>
                    {line || " "}
                  </div>
                ))}
              </pre>
            </div>
          ))}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-zinc-800 px-5 py-3">
          <button
            onClick={onCancel}
            className="rounded-lg px-3 py-1.5 text-sm text-zinc-400 transition hover:text-zinc-200"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="rounded-lg bg-hive-500 px-3 py-1.5 text-sm font-medium text-zinc-950 transition hover:bg-hive-400"
          >
            Apply all &amp; redeploy
          </button>
        </div>
      </div>
    </div>
  );
}

// diffLineClass colors a unified-diff line: hunk header blue, adds green,
// removes red, context muted. The +++/--- file headers fall through to add/remove.
function diffLineClass(line: string): string {
  if (line.startsWith("@@")) return "text-sky-400";
  if (line.startsWith("+")) return "text-green-400";
  if (line.startsWith("-")) return "text-red-400";
  return "text-zinc-500";
}
