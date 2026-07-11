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
export function DriftInfo({ services }: { services?: string[] }) {
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
            The running container was created from a{" "}
            <span className="font-medium text-zinc-100">
              different version of this compose file
            </span>{" "}
            than what's on disk now. Either the file was edited after the last
            deploy, or the container was started with other settings.
          </p>
          {services && services.length > 0 && (
            <p className="mt-2 text-xs text-zinc-400">
              Drifted service{services.length === 1 ? "" : "s"}:{" "}
              <span className="font-mono text-zinc-200">{services.join(", ")}</span>
            </p>
          )}
          <p className="mt-2 text-xs leading-relaxed text-zinc-500">
            This is <span className="text-zinc-300">not</span> about image
            updates — it compares your file against the running config. To
            resolve it, press{" "}
            <span className="font-medium text-accent-500">Deploy</span>: compose
            recreates only the services whose config changed. If the running
            state is the one you want instead, edit the file to match it.
          </p>
        </div>
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
          : "Running but not managed by Hivedock — read-only"
      }
    >
      {origin}
    </span>
  );
}
