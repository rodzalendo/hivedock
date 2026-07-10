import { useQuery } from "@tanstack/react-query";
import { fetchHealth } from "../api";

// Home is the health/status card for Phase 1. The zero-config dashboard
// (grouped card grid, icons) lands in Phase 2.
export default function Home() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
    refetchInterval: 10_000,
  });

  return (
    <div className="mx-auto max-w-3xl">
      <h2 className="mb-4 text-sm font-medium uppercase tracking-wide text-zinc-400">
        Backend status
      </h2>
      <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-6">
        {isLoading && <Row color="bg-zinc-500" label="Connecting…" />}
        {isError && (
          <Row color="bg-red-500" label={`Unreachable — ${(error as Error).message}`} />
        )}
        {data && (
          <div className="space-y-4">
            <Row
              color={data.status === "ok" ? "bg-green-500" : "bg-amber-500"}
              label={`Health: ${data.status}`}
            />
            <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
              <Field label="Stacks dir" value={data.stacksDir} />
              <Field label="Auth" value={data.authDisabled ? "disabled" : "enabled"} />
              <Field label="Server time" value={data.time} />
              <Field label="Version" value={data.version} />
            </dl>
          </div>
        )}
      </div>
    </div>
  );
}

function Row({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-3">
      <span className={`inline-block h-2.5 w-2.5 rounded-full ${color}`} />
      <span className="text-sm">{label}</span>
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950/50 px-3 py-2">
      <dt className="text-xs text-zinc-500">{label}</dt>
      <dd className="mt-0.5 break-all font-mono text-xs text-zinc-200">{value}</dd>
    </div>
  );
}
