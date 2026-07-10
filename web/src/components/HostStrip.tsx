import { useQuery } from "@tanstack/react-query";
import { fetchHostStats } from "../api";

function fmtBytes(n: number): string {
  if (!n) return "0";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

// HostStrip shows CPU/mem the container can see. Inside a container these are
// the cgroup-limited numbers, not the physical host (see docs/DEPLOYMENT.md).
export default function HostStrip() {
  const { data } = useQuery({
    queryKey: ["host-stats"],
    queryFn: fetchHostStats,
    refetchInterval: 3_000,
  });

  if (!data || !data.available) return null;

  const memPct = data.memTotalBytes
    ? (data.memUsedBytes / data.memTotalBytes) * 100
    : 0;

  return (
    <div className="flex flex-wrap items-center gap-6 rounded-lg border border-zinc-800 bg-zinc-900/40 px-4 py-2 text-xs">
      <Meter label="CPU" pct={data.cpuPercent} text={`${data.cpuPercent.toFixed(0)}%`} />
      <Meter
        label="Memory"
        pct={memPct}
        text={`${fmtBytes(data.memUsedBytes)} / ${fmtBytes(data.memTotalBytes)}`}
      />
      {data.numCpu > 0 && (
        <span className="text-zinc-500">
          {data.numCpu} vCPU
        </span>
      )}
    </div>
  );
}

function Meter({ label, pct, text }: { label: string; pct: number; text: string }) {
  const clamped = Math.max(0, Math.min(100, pct));
  const color =
    clamped > 85 ? "bg-red-500" : clamped > 60 ? "bg-amber-500" : "bg-hive-500";
  return (
    <div className="flex items-center gap-2">
      <span className="text-zinc-500">{label}</span>
      <div className="h-1.5 w-20 overflow-hidden rounded-full bg-zinc-800">
        <div className={`h-full ${color}`} style={{ width: `${clamped}%` }} />
      </div>
      <span className="tabular-nums text-zinc-300">{text}</span>
    </div>
  );
}
