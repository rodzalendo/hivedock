import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchHosts, fetchHostContainers, type RemoteContainer } from "../api";
import { useI18n } from "../i18n";

// Hosts is the multi-host view (docs/MULTIHOST.md): pick a host — the local one
// or any connected agent — and see its containers. Phase 1 is read-only.
export default function Hosts() {
  const { t } = useI18n();
  const { data: hosts } = useQuery({
    queryKey: ["hosts"],
    queryFn: fetchHosts,
    refetchInterval: 10_000,
  });
  const list = useMemo(() => hosts ?? [], [hosts]);
  const [selected, setSelected] = useState<string | null>(null);
  // Default to the first remote host (the interesting case), else local.
  const active =
    selected ?? list.find((h) => !h.local)?.name ?? list[0]?.name ?? "local";

  const {
    data: containers,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["host-containers", active],
    queryFn: () => fetchHostContainers(active),
    enabled: list.length > 0,
    refetchInterval: 10_000,
  });

  return (
    <div className="space-y-5">
      <div>
        <h2 className="text-sm font-medium uppercase tracking-wide text-zinc-400">
          {t("nav.hosts")}
        </h2>
        <p className="mt-0.5 text-sm text-zinc-500">
          Containers across every connected host.
        </p>
      </div>

      <div className="flex flex-wrap gap-2">
        {list.map((h) => (
          <button
            key={h.name}
            onClick={() => setSelected(h.name)}
            className={`flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm transition ${
              h.name === active
                ? "border-accent-500/50 bg-zinc-800/60 text-zinc-100"
                : "border-zinc-800 text-zinc-300 hover:bg-zinc-900"
            }`}
          >
            <span
              className={`inline-block h-2 w-2 rounded-full ${h.online ? "bg-green-500" : "bg-zinc-600"}`}
            />
            {h.name}
            {h.local && <span className="text-[11px] text-zinc-500">this host</span>}
            {h.version && (
              <span className="font-mono text-[10px] text-zinc-600">v{h.version}</span>
            )}
          </button>
        ))}
      </div>

      {isLoading && <p className="text-sm text-zinc-500">Loading…</p>}
      {isError && (
        <p className="text-sm text-red-400">
          Failed to load — {(error as Error).message}
        </p>
      )}
      {!isLoading && !isError && (containers?.length ?? 0) === 0 && (
        <div className="rounded-lg border border-dashed border-zinc-800 p-6 text-sm text-zinc-500">
          No containers on this host.
        </div>
      )}

      {(containers?.length ?? 0) > 0 && (
        <div className="overflow-x-auto rounded-xl border border-zinc-800">
          <table className="w-full text-left text-sm">
            <thead className="text-[11px] uppercase tracking-wider text-zinc-500">
              <tr className="border-b border-zinc-800">
                <th className="px-4 py-2 font-medium">Container</th>
                <th className="px-4 py-2 font-medium">Image</th>
                <th className="px-4 py-2 font-medium">State</th>
                <th className="px-4 py-2 font-medium">Stack</th>
              </tr>
            </thead>
            <tbody>
              {containers!.map((c) => (
                <tr key={c.name} className="border-t border-zinc-800/60">
                  <td className="px-4 py-2 text-zinc-200">{c.name}</td>
                  <td className="px-4 py-2 font-mono text-xs text-zinc-400">{c.image}</td>
                  <td className="px-4 py-2">
                    <span className="inline-flex items-center gap-1.5 text-xs text-zinc-300">
                      <span className={`inline-block h-2 w-2 rounded-full ${dotColor(c)}`} />
                      {c.state}
                      {c.health ? ` (${c.health})` : ""}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-xs text-zinc-400">
                    {c.stack ? `${c.stack}${c.service ? ` / ${c.service}` : ""}` : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function dotColor(c: RemoteContainer): string {
  if (c.health === "unhealthy") return "bg-red-500";
  if (c.health === "starting") return "bg-amber-500";
  if (c.state === "running") return "bg-green-500";
  if (c.state === "exited") return "bg-amber-500";
  return "bg-zinc-600";
}
