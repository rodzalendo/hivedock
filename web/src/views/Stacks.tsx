import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchStacks,
  createStack,
  fetchUpdates,
  type Stack,
  type Service,
} from "../api";
import { DriftBadge, OriginBadge, ServiceDot, StatusDot } from "../components/ui";
import HostStrip from "../components/HostStrip";
import LogsPanel from "../components/LogsPanel";
import DeployConsole from "../components/DeployConsole";
import ComposeEditor from "../components/ComposeEditor";
import EnvEditor from "../components/EnvEditor";

export default function Stacks() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["stacks"],
    queryFn: fetchStacks,
    // WebSocket push (useLiveUpdates) drives freshness; this slow interval is a
    // fallback for when the socket is down.
    refetchInterval: 30_000,
  });

  const [selected, setSelected] = useState<string | null>(null);
  const [createdName, setCreatedName] = useState<string | null>(null);
  const qc = useQueryClient();

  // Which stacks have an available image update (drives the row badge).
  const { data: updates } = useQuery({
    queryKey: ["updates"],
    queryFn: fetchUpdates,
    staleTime: 30_000,
  });
  const stacksWithUpdate = useMemo(() => {
    const set = new Set<string>();
    for (const u of updates ?? []) {
      if (u.hasUpdate) u.usedBy.forEach((x) => set.add(x.stack));
    }
    return set;
  }, [updates]);

  const stacks = useMemo(() => data ?? [], [data]);
  const selectedStack = useMemo(
    () => stacks.find((s) => s.name === selected) ?? null,
    [stacks, selected],
  );

  const managed = stacks.filter((s) => s.origin === "managed");
  const external = stacks.filter((s) => s.origin === "external");

  async function handleCreate(name: string) {
    const created = await createStack(name);
    await qc.invalidateQueries({ queryKey: ["stacks"] });
    setCreatedName(created.name);
    setSelected(created.name);
  }

  return (
    <div className="space-y-5">
      <HostStrip />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,22rem)_1fr]">
      <div>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-medium uppercase tracking-wide text-zinc-400">
            Stacks
          </h2>
          <div className="flex items-center gap-2">
            <span className="text-xs text-zinc-600">{stacks.length}</span>
            <NewStack onCreate={handleCreate} existing={stacks.map((s) => s.name)} />
          </div>
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
                hasUpdate={stacksWithUpdate.has(s.name)}
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
          <StackDetail
            key={selectedStack.name}
            stack={selectedStack}
            initialTab={
              createdName === selectedStack.name ? "compose" : "containers"
            }
          />
        ) : (
          <div className="flex h-full min-h-40 items-center justify-center rounded-xl border border-zinc-800 text-sm text-zinc-600">
            Select a stack to see its containers
          </div>
        )}
      </div>
      </div>
    </div>
  );
}

// NewStack is a small inline form for scaffolding a stack: name → dir + template
// compose.yaml. On success the caller selects the stack on its Compose tab.
function NewStack({
  onCreate,
  existing,
}: {
  onCreate: (name: string) => Promise<void>;
  existing: string[];
}) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function reset() {
    setOpen(false);
    setName("");
    setError(null);
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    if (existing.includes(trimmed)) {
      setError("A stack with that name already exists.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await onCreate(trimmed);
      reset();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create stack.");
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="rounded-md border border-zinc-700 px-2 py-1 text-xs text-zinc-300 transition hover:bg-zinc-800"
      >
        + New
      </button>
    );
  }

  return (
    <form onSubmit={submit} className="flex items-center gap-1">
      <input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="stack-name"
        onKeyDown={(e) => e.key === "Escape" && reset()}
        className="w-28 rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1 text-xs outline-none focus:border-accent-500"
      />
      <button
        type="submit"
        disabled={busy}
        className="rounded-md bg-accent-600 px-2 py-1 text-xs font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-40"
      >
        Create
      </button>
      <button
        type="button"
        onClick={reset}
        className="rounded-md px-1.5 py-1 text-xs text-zinc-500 hover:text-zinc-300"
      >
        ✕
      </button>
      {error && (
        <span className="ml-1 max-w-40 truncate text-[11px] text-red-400" title={error}>
          {error}
        </span>
      )}
    </form>
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
  hasUpdate,
  onClick,
}: {
  stack: Stack;
  active: boolean;
  hasUpdate?: boolean;
  onClick: () => void;
}) {
  const running = stack.services.filter((s) => s.state === "running").length;
  return (
    <li>
      <button
        onClick={onClick}
        className={`flex w-full items-center gap-2.5 rounded-lg border px-3 py-2 text-left transition ${
          active
            ? "border-accent-500/40 bg-zinc-800/60"
            : "border-transparent hover:bg-zinc-900"
        }`}
      >
        <StatusDot status={stack.status} />
        <span className="truncate text-sm text-zinc-100">{stack.name}</span>
        <span className="ml-auto flex items-center gap-2">
          {hasUpdate && (
            <span
              className="rounded bg-hive-600/20 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-hive-500"
              title="An image update is available for this stack"
            >
              update
            </span>
          )}
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

type DetailTab = "containers" | "logs" | "compose" | "env" | "deploy";

function StackDetail({
  stack,
  initialTab = "containers",
}: {
  stack: Stack;
  initialTab?: DetailTab;
}) {
  const managed = stack.origin === "managed";
  const [tab, setTab] = useState<DetailTab>(
    initialTab === "compose" && !managed ? "containers" : initialTab,
  );

  const key = stack.name;

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

      <div className="flex gap-1 border-b border-zinc-800 px-3 pt-2">
        <Tab active={tab === "containers"} onClick={() => setTab("containers")}>
          Containers
        </Tab>
        <Tab active={tab === "logs"} onClick={() => setTab("logs")}>
          Logs
        </Tab>
        {managed && (
          <Tab active={tab === "compose"} onClick={() => setTab("compose")}>
            Compose
          </Tab>
        )}
        {managed && (
          <Tab active={tab === "env"} onClick={() => setTab("env")}>
            Env
          </Tab>
        )}
        {managed && (
          <Tab active={tab === "deploy"} onClick={() => setTab("deploy")}>
            Deploy
          </Tab>
        )}
      </div>

      {tab === "compose" && managed ? (
        <ComposeEditor key={key} stack={stack.name} />
      ) : tab === "env" && managed ? (
        <EnvEditor key={key} stack={stack.name} />
      ) : tab === "deploy" && managed ? (
        <DeployConsole key={key} stack={stack.name} />
      ) : tab === "containers" ? (
        <div className="px-5 py-4">
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
      ) : (
        <LogsPanel
          key={key}
          stack={stack.name}
          services={stack.services.map((s) => s.name)}
        />
      )}
    </div>
  );
}

function Tab({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-t-lg px-3 py-1.5 text-sm transition ${
        active
          ? "bg-zinc-800/60 text-zinc-100"
          : "text-zinc-500 hover:text-zinc-300"
      }`}
    >
      {children}
    </button>
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
