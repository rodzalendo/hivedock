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
