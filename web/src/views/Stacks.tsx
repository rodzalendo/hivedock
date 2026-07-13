import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchStacks,
  createStack,
  deleteStack,
  renameStack,
  runStackAction,
  restartService,
  fetchUpdates,
  type Stack,
  type Service,
} from "../api";
import {
  DriftBadge,
  DriftInfo,
  OriginBadge,
  ServiceDot,
  StatusDot,
} from "../components/ui";
import { PencilIcon, RestartIcon, SpinnerIcon, TrashIcon } from "../components/icons";
import HostStrip from "../components/HostStrip";
import { useHashRoute, navigate } from "../useHashRoute";
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

  // The selected stack lives in the URL (#/stacks/<name>), so a refresh or a
  // shared link reopens the same stack.
  const route = useHashRoute();
  const selected = route[0] === "stacks" ? (route[1] ?? null) : null;
  const setSelected = (name: string | null) =>
    name ? navigate("stacks", name) : navigate("stacks");
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
            onDeleted={async () => {
              setSelected(null);
              await qc.invalidateQueries({ queryKey: ["stacks"] });
            }}
            onRenamed={async (newName) => {
              setSelected(newName);
              await qc.invalidateQueries({ queryKey: ["stacks"] });
            }}
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

// imageTag extracts the tag from an image reference, guarding against the
// registry-port colon (registry:5000/app) and digests (app@sha256:…).
function imageTag(image: string): string | null {
  const ref = image.split("@")[0];
  const lastColon = ref.lastIndexOf(":");
  const lastSlash = ref.lastIndexOf("/");
  if (lastColon > lastSlash) return ref.slice(lastColon + 1);
  return null; // no tag → implicit :latest
}

// A version-looking tag starts with a digit or a "v" + digit ("5.1.2",
// "v1.135.0", "11.4.12"); "latest", "stable", "alpine" don't qualify.
const isVersionTag = (tag: string) => /^v?\d/.test(tag);

// stackVersion summarizes a stack's version for the header: the primary
// service's tag (name matching the stack, e.g. immich-server → immich) when
// there is one, else a single version shared by every service, else null.
function stackVersion(stack: Stack): string | null {
  const tagged = stack.services
    .map((s) => ({ name: s.name.toLowerCase(), tag: imageTag(s.image) }))
    .filter((s): s is { name: string; tag: string } => !!s.tag && isVersionTag(s.tag));
  if (tagged.length === 0) return null;
  const slug = stack.name.toLowerCase();
  const primary =
    tagged.find((s) => s.name === slug) ??
    tagged.find((s) => s.name.startsWith(slug));
  if (primary) return primary.tag;
  const distinct = new Set(tagged.map((s) => s.tag));
  return distinct.size === 1 ? tagged[0].tag : null;
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

type ActionMode = "idle" | "rename" | "delete";

function StackDetail({
  stack,
  onDeleted,
  onRenamed,
}: {
  stack: Stack;
  onDeleted: () => void | Promise<void>;
  onRenamed: (newName: string) => void | Promise<void>;
}) {
  const managed = stack.origin === "managed";
  const services = stack.services ?? [];
  const driftedServices = services.filter((s) => s.drifted).map((s) => s.name);
  const version = stackVersion(stack);
  const key = stack.name;

  return (
    <div className="space-y-4">
      {/* Header card: name + manage actions, deploy buttons, containers. */}
      <div className="rounded-xl border border-zinc-800 bg-zinc-900/40">
        <div className="flex flex-wrap items-center gap-3 px-5 py-4">
          <StatusDot status={stack.status} />
          <div className="flex flex-col">
            <h2 className="text-base font-semibold leading-tight text-zinc-100">
              {stack.name}
            </h2>
            {version && (
              <span
                className="font-mono text-[11px] text-zinc-500"
                title="Version (from the image tag)"
              >
                {version}
              </span>
            )}
          </div>
          <OriginBadge origin={stack.origin} />
          {stack.drifted && (
            <DriftInfo
              services={driftedServices}
              onForceRecreate={
                managed ? () => void runStackAction(stack.name, "recreate") : undefined
              }
            />
          )}
          <span className="text-xs capitalize text-zinc-500">{stack.status}</span>
          {managed && (
            <div className="ml-auto">
              <StackActions
                stack={stack}
                onDeleted={onDeleted}
                onRenamed={onRenamed}
              />
            </div>
          )}
        </div>

        {stack.composeFile && (
          <div className="border-t border-zinc-800 px-5 py-2 font-mono text-[11px] text-zinc-500">
            {stack.composeFile}
          </div>
        )}

        {managed && (
          <div className="border-t border-zinc-800">
            <DeployConsole key={key} stack={stack.name} />
          </div>
        )}

        <div className="border-t border-zinc-800 px-5 py-4">
          <h3 className="mb-2 text-[11px] font-medium uppercase tracking-wider text-zinc-500">
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
                  {managed && <th className="pb-2" />}
                </tr>
              </thead>
              <tbody className="align-top">
                {services.map((svc) => (
                  <ServiceRow
                    key={svc.name}
                    svc={svc}
                    stack={managed ? stack.name : undefined}
                  />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* Below: Compose/Env (toggled) on the left, always-on Logs right. */}
      <div className={`grid gap-4 ${managed ? "xl:grid-cols-2" : ""}`}>
        {managed && <ConfigCard key={`cfg-${key}`} stack={stack.name} />}

        <div className="self-start rounded-xl border border-zinc-800 bg-zinc-900/40">
          <div className="border-b border-zinc-800 px-5 py-3 text-xs font-medium uppercase tracking-wide text-zinc-400">
            Logs
          </div>
          <LogsPanel
            key={key}
            stack={stack.name}
            services={services.map((s) => s.name)}
          />
        </div>
      </div>
    </div>
  );
}

// ConfigCard holds the stack's editable files — the compose file and its
// .env — in one card with a small toggle between them.
function ConfigCard({ stack }: { stack: string }) {
  const [file, setFile] = useState<"compose" | "env">("compose");
  return (
    <div className="self-start rounded-xl border border-zinc-800 bg-zinc-900/40">
      <div className="flex items-center gap-1 border-b border-zinc-800 px-4 py-2">
        {(["compose", "env"] as const).map((f) => (
          <button
            key={f}
            onClick={() => setFile(f)}
            className={`rounded-md px-2.5 py-1 text-xs font-medium uppercase tracking-wide transition ${
              file === f
                ? "bg-zinc-800 text-zinc-100"
                : "text-zinc-500 hover:text-zinc-300"
            }`}
          >
            {f === "compose" ? "Compose" : "Env"}
          </button>
        ))}
      </div>
      {file === "compose" ? (
        <ComposeEditor key={stack} stack={stack} />
      ) : (
        <EnvEditor key={stack} stack={stack} />
      )}
    </div>
  );
}

// StackActions holds the rename + delete controls for a managed stack. Rename
// is blocked while the stack is running (its compose project name can't change
// without orphaning containers); delete stops it first, then removes the dir.
function StackActions({
  stack,
  onDeleted,
  onRenamed,
}: {
  stack: Stack;
  onDeleted: () => void | Promise<void>;
  onRenamed: (newName: string) => void | Promise<void>;
}) {
  const [mode, setMode] = useState<ActionMode>("idle");
  const [newName, setNewName] = useState(stack.name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const running = stack.status === "running" || stack.status === "partial";

  function reset() {
    setMode("idle");
    setError(null);
    setNewName(stack.name);
  }

  async function doRename(e: React.FormEvent) {
    e.preventDefault();
    const target = newName.trim();
    if (!target || target === stack.name) return reset();
    setBusy(true);
    setError(null);
    try {
      await renameStack(stack.name, target);
      await onRenamed(target);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Rename failed.");
      setBusy(false);
    }
  }

  async function doDelete() {
    setBusy(true);
    setError(null);
    try {
      await deleteStack(stack.name);
      await onDeleted();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete failed.");
      setBusy(false);
    }
  }

  if (mode === "rename") {
    return (
      <form onSubmit={doRename} className="flex flex-col items-end gap-1">
        <div className="flex items-center gap-1.5">
          <input
            autoFocus
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && reset()}
            className="w-40 rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1 text-xs outline-none focus:border-accent-500"
          />
          <button
            type="submit"
            disabled={busy}
            className="rounded-md bg-accent-600 px-2 py-1 text-xs font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-40"
          >
            {busy ? "…" : "Save"}
          </button>
          <button
            type="button"
            onClick={reset}
            className="rounded-md px-1.5 py-1 text-xs text-zinc-500 hover:text-zinc-300"
          >
            ✕
          </button>
        </div>
        {error && <span className="max-w-64 text-right text-[11px] text-red-400">{error}</span>}
      </form>
    );
  }

  if (mode === "delete") {
    return (
      <div className="flex flex-col items-end gap-1">
        <div className="flex items-center gap-2">
          <span className="text-xs text-zinc-400">
            Delete <span className="font-medium text-zinc-200">{stack.name}</span>?
            {running && " Containers will be stopped first."}
          </span>
          <button
            onClick={doDelete}
            disabled={busy}
            className="rounded-md border border-red-500/40 bg-red-500/10 px-2 py-1 text-xs font-medium text-red-400 transition hover:bg-red-500/20 disabled:opacity-40"
          >
            {busy ? "Deleting…" : "Delete"}
          </button>
          <button
            onClick={reset}
            disabled={busy}
            className="rounded-md px-1.5 py-1 text-xs text-zinc-500 hover:text-zinc-300"
          >
            Cancel
          </button>
        </div>
        <span className="max-w-72 text-right text-[11px] text-zinc-600">
          {error ? (
            <span className="text-red-400">{error}</span>
          ) : (
            "Removes the stack directory and its compose file. This can't be undone."
          )}
        </span>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <button
        onClick={() => setMode("rename")}
        disabled={running}
        title={running ? "Stop the stack first to rename it" : "Rename this stack"}
        className="flex items-center gap-1.5 rounded-lg border border-accent-500/40 px-3 py-1.5 text-sm font-medium text-accent-500 transition hover:bg-accent-500/10 disabled:cursor-not-allowed disabled:opacity-40"
      >
        <PencilIcon className="h-3.5 w-3.5" />
        Rename
      </button>
      <button
        onClick={() => setMode("delete")}
        title="Delete this stack"
        className="flex items-center gap-1.5 rounded-lg border border-red-500/40 px-3 py-1.5 text-sm font-medium text-red-400 transition hover:bg-red-500/10"
      >
        <TrashIcon className="h-3.5 w-3.5" />
        Delete
      </button>
    </div>
  );
}

// ServiceRow is one container line; managed stacks (stack prop set) get a
// per-service restart button. The spinner clears when the operation's
// deploy:end event lands (the restart itself streams to the console above).
function ServiceRow({ svc, stack }: { svc: Service; stack?: string }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!busy) return;
    const onDeploy = (ev: Event) => {
      const msg = (ev as CustomEvent).detail as {
        type: string;
        payload?: { stack?: string; service?: string };
      };
      if (
        msg.type === "deploy:end" &&
        msg.payload?.stack === stack &&
        msg.payload?.service === svc.name
      ) {
        setBusy(false);
      }
    };
    window.addEventListener("hivedock:deploy", onDeploy);
    // Never spin forever if the socket drops mid-operation.
    const timer = window.setTimeout(() => setBusy(false), 120_000);
    return () => {
      window.removeEventListener("hivedock:deploy", onDeploy);
      window.clearTimeout(timer);
    };
  }, [busy, stack, svc.name]);

  async function onRestart() {
    if (!stack || busy) return;
    setBusy(true);
    setError(null);
    try {
      await restartService(stack, svc.name);
    } catch (err) {
      setBusy(false);
      setError(err instanceof Error ? err.message : "Restart failed.");
    }
  }

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
      <td className="py-2 pr-4 text-xs text-zinc-400">
        {svc.ports && svc.ports.length > 0
          ? svc.ports
              .filter((p) => p.public)
              .map((p) => `${p.public}:${p.private}/${p.type}`)
              .join(", ") || "—"
          : "—"}
      </td>
      {stack && (
        <td className="py-1.5 text-right">
          <span className="inline-flex items-center gap-2">
            {error && (
              <span className="max-w-40 truncate text-[11px] text-red-400" title={error}>
                {error}
              </span>
            )}
            <button
              onClick={onRestart}
              disabled={busy}
              title={`Restart ${svc.name}`}
              className="rounded-md border border-zinc-700 p-1.5 text-zinc-400 transition hover:bg-zinc-800 hover:text-zinc-200 disabled:opacity-50"
            >
              {busy ? (
                <SpinnerIcon className="h-3.5 w-3.5" />
              ) : (
                <RestartIcon className="h-3.5 w-3.5" />
              )}
            </button>
          </span>
        </td>
      )}
    </tr>
  );
}
