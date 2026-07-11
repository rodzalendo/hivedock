import { useEffect, useRef, useState } from "react";
import { runStackAction, type StackAction } from "../api";
import {
  ChevronsDownIcon,
  DownloadIcon,
  PlayIcon,
  RestartIcon,
  StopIcon,
} from "./icons";

interface DeployMessage {
  type: string;
  payload: {
    id?: string;
    stack?: string;
    action?: string;
    line?: string;
    ok?: boolean;
    error?: string;
  };
}

type Phase = "idle" | "running" | "ok" | "error";

const actions: {
  id: StackAction;
  label: string;
  title: string;
  danger?: boolean;
  Icon: (p: { className?: string }) => JSX.Element;
}[] = [
  {
    id: "up",
    label: "Deploy",
    title:
      "docker compose up -d — create/recreate and start containers from the compose file. Run this to apply edits or pulled images.",
    Icon: PlayIcon,
  },
  {
    id: "pull",
    label: "Pull",
    title:
      "docker compose pull — download newer images for the tags in the compose file. It does NOT restart anything; press Deploy afterward to apply.",
    Icon: DownloadIcon,
  },
  {
    id: "restart",
    label: "Restart",
    title: "docker compose restart — restart the containers without recreating them.",
    Icon: RestartIcon,
  },
  {
    id: "stop",
    label: "Stop",
    title:
      "docker compose stop — stop the containers but keep them (start again with Deploy).",
    Icon: StopIcon,
  },
  {
    id: "down",
    label: "Down",
    title:
      "docker compose down — stop AND remove the containers and the stack's network. Your compose file and named volumes are kept; the stack shows as stopped until you Deploy again.",
    danger: true,
    Icon: ChevronsDownIcon,
  },
];

// DeployConsole renders the lifecycle action buttons for a managed stack plus a
// terminal-style pane that streams the operation's output (received over the
// shared WebSocket as hivedock:deploy events). Keyed by stack name by the
// caller, so switching stacks starts fresh.
export default function DeployConsole({ stack }: { stack: string }) {
  const [phase, setPhase] = useState<Phase>("idle");
  const [action, setAction] = useState<StackAction | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const paneRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (ev: Event) => {
      const msg = (ev as CustomEvent<DeployMessage>).detail;
      if (!msg || msg.payload.stack !== stack) return;
      switch (msg.type) {
        case "deploy:start":
          setPhase("running");
          setAction((msg.payload.action as StackAction) ?? null);
          setLines([]);
          setError(null);
          break;
        case "deploy:line":
          if (msg.payload.line !== undefined) {
            setLines((prev) => [...prev, msg.payload.line as string]);
          }
          break;
        case "deploy:end":
          setPhase(msg.payload.ok ? "ok" : "error");
          if (!msg.payload.ok) setError(msg.payload.error ?? "operation failed");
          break;
      }
    };
    window.addEventListener("hivedock:deploy", handler);
    return () => window.removeEventListener("hivedock:deploy", handler);
  }, [stack]);

  // Auto-scroll to the latest line.
  useEffect(() => {
    const el = paneRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  async function trigger(a: StackAction) {
    if (phase === "running") return;
    if (a === "down" && !window.confirm(`Stop and remove all containers in "${stack}"?`)) {
      return;
    }
    setPhase("running");
    setAction(a);
    setLines([]);
    setError(null);
    try {
      await runStackAction(stack, a);
      // Output + final status arrive via deploy:* events.
    } catch (err) {
      setPhase("error");
      setError(err instanceof Error ? err.message : "failed to start operation");
    }
  }

  const running = phase === "running";

  return (
    <div className="px-5 py-4">
      <div className="mb-3 flex flex-wrap gap-2">
        {actions.map((a) => (
          <button
            key={a.id}
            onClick={() => trigger(a.id)}
            disabled={running}
            title={a.title}
            className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition disabled:opacity-40 ${
              a.id === "up"
                ? "bg-accent-600 text-zinc-950 hover:bg-accent-500"
                : a.danger
                  ? "border border-red-500/30 text-red-400 hover:bg-red-500/10"
                  : "border border-zinc-700 text-zinc-300 hover:bg-zinc-800"
            }`}
          >
            <a.Icon className="h-3.5 w-3.5" />
            {a.label}
          </button>
        ))}
      </div>

      {phase === "idle" && lines.length === 0 ? (
        <p className="text-xs text-zinc-600">
          Run an operation to see its live output here.
        </p>
      ) : (
        <div>
          <div className="mb-1.5 flex items-center gap-2 text-xs">
            <StatusPill phase={phase} />
            {action && (
              <span className="font-mono text-zinc-500">docker compose {action}</span>
            )}
          </div>
          <div
            ref={paneRef}
            className="max-h-72 overflow-auto rounded-lg border border-zinc-800 bg-black/60 p-3 font-mono text-[11px] leading-relaxed text-zinc-300"
          >
            {lines.length === 0 && running && (
              <span className="text-zinc-600">Starting…</span>
            )}
            {lines.map((l, i) => (
              <div key={i} className="whitespace-pre-wrap break-all">
                {l}
              </div>
            ))}
            {error && <div className="mt-1 text-red-400">✗ {error}</div>}
          </div>
        </div>
      )}
    </div>
  );
}

function StatusPill({ phase }: { phase: Phase }) {
  const map: Record<Phase, { label: string; cls: string }> = {
    idle: { label: "idle", cls: "bg-zinc-700/40 text-zinc-400" },
    running: { label: "running", cls: "bg-amber-500/15 text-amber-400" },
    ok: { label: "done", cls: "bg-green-500/15 text-green-400" },
    error: { label: "failed", cls: "bg-red-500/15 text-red-400" },
  };
  const { label, cls } = map[phase];
  return (
    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${cls}`}>
      {label}
    </span>
  );
}
