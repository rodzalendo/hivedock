import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchStacks, type Stack, type Service } from "../api";
import { DriftBadge, OriginBadge, ServiceDot, StatusDot } from "../components/ui";

export default function Stacks() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["stacks"],
    queryFn: fetchStacks,
    // WebSocket push (useLiveUpdates) drives freshness; this slow interval is a
    // fallback for when the socket is down.
    refetchInterval: 30_000,
  });

  const [selected, setSelected] = useState<string | null>(null);

  const stacks = data ?? [];
  const selectedStack = useMemo(
    () => stacks.find((s) => s.name === selected) ?? null,
    [stacks, selected],
  );

  const managed = stacks.filter((s) => s.origin === "managed");
  const external = stacks.filter((s) => s.origin === "external");

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,22rem)_1fr]">
      <div>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-medium uppercase tracking-wide text-zinc-400">
            Stacks
          </h2>
          <span className="text-xs text-zinc-600">{stacks.length}</span>
        </div>

        {isLoading && <p className="text-sm text-zinc-500">Loading…</p>}
        {isError && (
          <p className="text-sm text-red-400">
            Failed to load stacks — {(error as Error).message}
          </p>
        )}

        {!isLoading && !isError && stacks.length === 0 && (
          <div className="rounded-lg border border-dashed border-zinc-800 p-6 text-sm text-zinc-500">
            No stacks found. Add a compose file under your stacks directory, or
            start a container — it’ll appear here.
          </div>
        )}

        {managed.length > 0 && (
          <Group title="Managed">
            {managed.map((s) => (
              <StackRow
                key={s.name}
                stack={s}
                active={s.name === selected}
                onClick={() => setSelected(s.name)}
              />
            ))}
          </Group>
        )}

        {external.length > 0 && (
          <Group title="External (read-only)">
            {external.map((s) => (
              <StackRow
                key={s.name}
                stack={s}
                active={s.name === selected}
                onClick={() => setSelected(s.name)}
              />
            ))}
          </Group>
        )}
      </div>

      <div>
        {selectedStack ? (
          <StackDetail stack={selectedStack} />
        ) : (
          <div className="flex h-full min-h-40 items-center justify-center rounded-xl border border-zinc-800 text-sm text-zinc-600">
            Select a stack to see its containers
          </div>
        )}
      </div>
    </div>
  );
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mb-5">
      <h3 className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-zinc-600">
        {title}
      </h3>
      <ul className="space-y-1">{children}</ul>
    </div>
  );
}

function StackRow({
  stack,
  active,
  onClick,
}: {
  stack: Stack;
  active: boolean;
  onClick: () => void;
}) {
  const running = stack.services.filter((s) => s.state === "running").length;
  return (
    <li>
      <button
        onClick={onClick}
        className={`flex w-full items-center gap-2.5 rounded-lg border px-3 py-2 text-left transition ${
          active
            ? "border-hive-600/50 bg-zinc-800/60"
            : "border-transparent hover:bg-zinc-900"
        }`}
      >
        <StatusDot status={stack.status} />
        <span className="truncate text-sm text-zinc-100">{stack.name}</span>
        <span className="ml-auto flex items-center gap-2">
          {stack.drifted && <DriftBadge />}
          <span className="text-xs text-zinc-500">
            {running}/{stack.services.length}
          </span>
          <OriginBadge origin={stack.origin} />
        </span>
      </button>
    </li>
  );
}

function StackDetail({ stack }: { stack: Stack }) {
  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/40">
      <div className="flex flex-wrap items-center gap-3 border-b border-zinc-800 px-5 py-4">
        <StatusDot status={stack.status} />
        <h2 className="text-base font-semibold text-zinc-100">{stack.name}</h2>
        <OriginBadge origin={stack.origin} />
        {stack.drifted && <DriftBadge />}
        <span className="text-xs capitalize text-zinc-500">{stack.status}</span>
      </div>

      {stack.composeFile && (
        <div className="border-b border-zinc-800 px-5 py-2 font-mono text-[11px] text-zinc-500">
          {stack.composeFile}
        </div>
      )}

      <div className="px-5 py-4">
        <h3 className="mb-3 text-[11px] font-medium uppercase tracking-wider text-zinc-500">
          Containers
        </h3>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-zinc-500">
                <th className="pb-2 pr-4 font-medium">Service</th>
                <th className="pb-2 pr-4 font-medium">Image</th>
                <th className="pb-2 pr-4 font-medium">State</th>
                <th className="pb-2 font-medium">Ports</th>
              </tr>
            </thead>
            <tbody className="align-top">
              {stack.services.map((svc) => (
                <ServiceRow key={svc.name} svc={svc} />
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

function ServiceRow({ svc }: { svc: Service }) {
  return (
    <tr className="border-t border-zinc-800/60">
      <td className="py-2 pr-4 text-zinc-200">
        <span className="inline-flex items-center gap-2">
          {svc.name}
          {svc.drifted && <DriftBadge />}
        </span>
      </td>
      <td className="py-2 pr-4 font-mono text-xs text-zinc-400">{svc.image}</td>
      <td className="py-2 pr-4">
        <span className="inline-flex items-center gap-1.5 text-xs text-zinc-300">
          <ServiceDot state={svc.state} />
          {svc.state}
        </span>
      </td>
      <td className="py-2 text-xs text-zinc-400">
        {svc.ports && svc.ports.length > 0
          ? svc.ports
              .filter((p) => p.public)
              .map((p) => `${p.public}:${p.private}/${p.type}`)
              .join(", ") || "—"
          : "—"}
      </td>
    </tr>
  );
}
