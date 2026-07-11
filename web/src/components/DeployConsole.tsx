import { useEffect, useRef, useState } from "react";
import { runStackAction, type StackAction } from "../api";

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

const actions: { id: StackAction; label: string; danger?: boolean }[] = [
  { id: "up", label: "Deploy" },
  { id: "pull", label: "Pull" },
  { id: "restart", label: "Restart" },
  { id: "stop", label: "Stop" },
  { id: "down", label: "Down", danger: true },
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
            className={`rounded-lg px-3 py-1.5 text-sm font-medium transition disabled:opacity-40 ${
              a.id === "up"
                ? "bg-hive-600 text-white hover:bg-hive-500"
                : a.danger
                  ? "border border-red-500/30 text-red-400 hover:bg-red-500/10"
                  : "border border-zinc-700 text-zinc-300 hover:bg-zinc-800"
            }`}
          >
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
