import { useEffect, useRef, useState } from "react";
import type { Origin, StackStatus } from "../api";

const statusColor: Record<StackStatus, string> = {
  running: "bg-green-500",
  partial: "bg-amber-500",
  stopped: "bg-zinc-500",
  unknown: "bg-zinc-600",
};

export function StatusDot({ status }: { status: StackStatus }) {
  return (
    <span
      className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ${statusColor[status]}`}
      title={status}
      aria-label={status}
    />
  );
}

// A per-service state dot (running/exited/absent/...).
export function ServiceDot({ state }: { state: string }) {
  const color =
    state === "running"
      ? "bg-green-500"
      : state === "absent"
        ? "bg-zinc-700"
        : "bg-amber-500";
  return (
    <span
      className={`inline-block h-2 w-2 shrink-0 rounded-full ${color}`}
      title={state}
      aria-label={state}
    />
  );
}

export function DriftBadge() {
  return (
    <span
      className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-400"
      title="The running container's config differs from the compose file on disk — click the badge in the stack view for details"
    >
      drift
    </span>
  );
}

// DriftInfo is the clickable version of the drift badge: it opens a popover
// explaining what drift means, which services drifted, and how to resolve it.
// Use it anywhere that isn't nested inside another button (detail header,
// table cells); lists keep the plain DriftBadge.
export function DriftInfo({
  services,
  onForceRecreate,
}: {
  services?: string[];
  onForceRecreate?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  return (
    <div ref={ref} className="relative inline-block">
      <button
        onClick={() => setOpen((v) => !v)}
        className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-400 transition hover:bg-amber-500/25"
        title="What does drift mean?"
      >
        drift
      </button>
      {open && (
        <div className="absolute left-0 z-20 mt-1.5 w-80 rounded-lg border border-zinc-700 bg-zinc-900 p-4 text-left shadow-xl">
          <h4 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-amber-400">
            Configuration drift
          </h4>
          <p className="text-xs leading-relaxed text-zinc-300">
            The running container's configuration fingerprint doesn't match the
            compose file on disk. Common causes: the file was edited after the
            last deploy, or the container was{" "}
            <span className="font-medium text-zinc-100">
              deployed by another tool
            </span>{" "}
            (or another compose version), which stamps a different fingerprint
            even for an identical file. It is not about image updates.
          </p>
          {services && services.length > 0 && (
            <p className="mt-2 text-xs text-zinc-400">
              Drifted service{services.length === 1 ? "" : "s"}:{" "}
              <span className="font-mono text-zinc-200">{services.join(", ")}</span>
            </p>
          )}
          <p className="mt-2 text-xs leading-relaxed text-zinc-500">
            <span className="font-medium text-accent-500">Deploy</span> recreates
            only what compose itself considers changed — so a fingerprint from
            another tool can survive it. Force recreate rebuilds the containers
            from this file unconditionally, which always clears the badge.
          </p>
          {onForceRecreate && (
            <button
              onClick={() => {
                setOpen(false);
                onForceRecreate();
              }}
              className="mt-3 rounded-lg border border-amber-500/40 px-2.5 py-1 text-xs font-medium text-amber-400 transition hover:bg-amber-500/10"
            >
              Force recreate now
            </button>
          )}
        </div>
      )}
    </div>
  );
}

// HelpTip is a round "?" next to a section header that reveals the section's
// explanation in a popover — keeps the page clean without losing the docs.
export function HelpTip({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  return (
    <span ref={ref} className="relative inline-flex">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label="What is this?"
        title="What is this?"
        className="flex h-4 w-4 items-center justify-center rounded-full border border-zinc-600 text-[10px] font-semibold leading-none text-zinc-500 transition hover:border-zinc-400 hover:text-zinc-200"
      >
        ?
      </button>
      {open && (
        <span className="absolute left-0 top-5 z-20 block w-72 rounded-lg border border-zinc-700 bg-zinc-900 p-3 text-left text-[11px] font-normal normal-case leading-relaxed tracking-normal text-zinc-400 shadow-xl">
          {children}
        </span>
      )}
    </span>
  );
}

// ProgressBar is a slim theme-aware bar. Pass `value` (0..1) for determinate
// progress, or `indeterminate` for an animated sweep when the total isn't
// known (a single compose op of unknown duration). `tone` picks the fill color.
export function ProgressBar({
  value,
  indeterminate,
  tone = "accent",
  className = "",
}: {
  value?: number;
  indeterminate?: boolean;
  tone?: "accent" | "hive";
  className?: string;
}) {
  const pct = Math.max(0, Math.min(1, value ?? 0)) * 100;
  const fill = tone === "hive" ? "bg-hive-500" : "bg-accent-500";
  return (
    <div
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={indeterminate ? undefined : 100}
      aria-valuenow={indeterminate ? undefined : Math.round(pct)}
      className={`h-1.5 w-full overflow-hidden rounded-full bg-zinc-800 ${className}`}
    >
      {indeterminate ? (
        <div className={`progress-indeterminate h-full w-1/3 rounded-full ${fill}`} />
      ) : (
        <div
          className={`h-full rounded-full transition-[width] duration-500 ease-out ${fill}`}
          style={{ width: `${pct}%` }}
        />
      )}
    </div>
  );
}

export function OriginBadge({ origin }: { origin: Origin }) {
  const managed = origin === "managed";
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${
        managed
          ? "bg-hive-600/20 text-hive-500"
          : "bg-zinc-700/40 text-zinc-400"
      }`}
      title={
        managed
          ? "Defined by a compose file in your stacks directory"
          : "Running but not managed by HiveDock — read-only"
      }
    >
      {origin}
    </span>
  );
}
