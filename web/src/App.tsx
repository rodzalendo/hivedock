import { useQuery } from "@tanstack/react-query";
import { fetchHealth } from "./api";

export default function App() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
    refetchInterval: 10_000,
  });

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <header className="border-b border-zinc-800">
        <div className="mx-auto flex max-w-5xl items-center gap-3 px-6 py-4">
          <span className="text-2xl" aria-hidden>
            🐝
          </span>
          <h1 className="text-lg font-semibold tracking-tight">Hivedock</h1>
          <span className="ml-auto text-xs text-zinc-500">
            {data ? `v${data.version}` : ""}
          </span>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-6 py-12">
        <section className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-6">
          <h2 className="mb-4 text-sm font-medium uppercase tracking-wide text-zinc-400">
            Backend status
          </h2>

          {isLoading && <StatusRow color="bg-zinc-500" label="Connecting…" />}

          {isError && (
            <StatusRow
              color="bg-red-500"
              label={`Unreachable — ${(error as Error).message}`}
            />
          )}

          {data && (
            <div className="space-y-3">
              <StatusRow
                color={data.status === "ok" ? "bg-green-500" : "bg-amber-500"}
                label={`Health: ${data.status}`}
              />
              <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
                <Field label="Stacks dir" value={data.stacksDir} />
                <Field
                  label="Auth"
                  value={data.authDisabled ? "disabled" : "enabled"}
                />
                <Field label="Server time" value={data.time} />
                <Field label="Version" value={data.version} />
              </dl>
            </div>
          )}
        </section>

        <p className="mt-6 text-sm text-zinc-500">
          Phase 0 skeleton. Stacks, Home, Updates, and Settings views arrive in
          later phases (see <code>docs/PLAN.md</code>).
        </p>
      </main>
    </div>
  );
}

function StatusRow({ color, label }: { color: string; label: string }) {
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
      <dd className="mt-0.5 break-all font-mono text-xs text-zinc-200">
        {value}
      </dd>
    </div>
  );
}
