import { useEffect, useMemo, useRef, useState } from "react";
import { useLogs } from "../useLogs";

// A palette so each service's lines get a stable color prefix.
const serviceColors = [
  "text-sky-400",
  "text-emerald-400",
  "text-violet-400",
  "text-amber-400",
  "text-pink-400",
  "text-teal-400",
];

export default function LogsPanel({
  stack,
  services,
}: {
  stack: string;
  services: string[];
}) {
  const [follow, setFollow] = useState(true);
  const [serviceFilter, setServiceFilter] = useState<string>("all");
  const { lines, error, connected } = useLogs(stack, follow);

  const colorFor = useMemo(() => {
    const map = new Map<string, string>();
    services.forEach((s, i) => map.set(s, serviceColors[i % serviceColors.length]));
    return (svc: string) => map.get(svc) ?? "text-zinc-400";
  }, [services]);

  const shown = useMemo(
    () => (serviceFilter === "all" ? lines : lines.filter((l) => l.service === serviceFilter)),
    [lines, serviceFilter],
  );

  const scrollRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (follow && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [shown, follow]);

  return (
    <div className="flex flex-col">
      <div className="flex flex-wrap items-center gap-3 px-5 py-3 text-xs">
        <label className="flex items-center gap-1.5 text-zinc-400">
          <input
            type="checkbox"
            checked={follow}
            onChange={(e) => setFollow(e.target.checked)}
            className="accent-accent-500"
          />
          Follow
        </label>

        {services.length > 1 && (
          <select
            value={serviceFilter}
            onChange={(e) => setServiceFilter(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-900 px-2 py-1 text-zinc-300"
          >
            <option value="all">All services</option>
            {services.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        )}

        <span className="ml-auto flex items-center gap-1.5 text-zinc-500">
          <span
            className={`inline-block h-2 w-2 rounded-full ${
              connected ? "bg-green-500" : "bg-zinc-600"
            }`}
          />
          {connected ? "streaming" : follow ? "connecting…" : "paused"}
        </span>
      </div>

      {error && (
        <div className="mx-5 mb-2 rounded border border-red-900/50 bg-red-950/30 px-3 py-2 text-xs text-red-400">
          {error}
        </div>
      )}

      <div
        ref={scrollRef}
        className="mx-3 mb-3 h-96 overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs leading-relaxed"
      >
        {shown.length === 0 ? (
          <p className="text-zinc-600">
            {follow ? "Waiting for output…" : "Follow is paused."}
          </p>
        ) : (
          shown.map((l) => (
            <div key={l.id} className="whitespace-pre-wrap break-all">
              {services.length > 1 && (
                <span className={`mr-2 ${colorFor(l.service)}`}>{l.service}</span>
              )}
              <span className={l.stream === "stderr" ? "text-red-300" : "text-zinc-300"}>
                {l.line}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
