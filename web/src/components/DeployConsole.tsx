import { useEffect, useRef } from "react";
import { runStackAction, type StackAction } from "../api";
import { useI18n } from "../i18n";
import {
  markFailed,
  markStarted,
  useDeployState,
  type DeployPhase,
} from "../deployStore";
import {
  DownloadIcon,
  PlayIcon,
  RestartIcon,
  SpinnerIcon,
  StopIcon,
} from "./icons";

const actions: {
  id: StackAction;
  labelKey: string;
  title: string;
  Icon: (p: { className?: string }) => JSX.Element;
}[] = [
  {
    id: "up",
    labelKey: "stacks.deploy",
    title:
      "docker compose up -d — create/recreate and start containers from the compose file. Run this to apply edits or pulled images.",
    Icon: PlayIcon,
  },
  {
    id: "pull",
    labelKey: "stacks.pull",
    title:
      "docker compose pull — download newer images for the tags in the compose file. It does NOT restart anything; press Deploy afterward to apply.",
    Icon: DownloadIcon,
  },
  {
    id: "restart",
    labelKey: "stacks.restart",
    title: "docker compose restart — restart the containers without recreating them.",
    Icon: RestartIcon,
  },
  {
    id: "stop",
    labelKey: "stacks.stop",
    title:
      "docker compose stop — stop the containers but keep them (start again with Deploy).",
    Icon: StopIcon,
  },
  // "down" (stop AND remove containers) was dropped from the UI: Stop covers
  // pausing, and Delete covers removal. The backend action remains available.
];

// DeployActions renders the lifecycle buttons for a managed stack. The output
// they produce is rendered by DeployOutput, which lives in the Logs card — both
// read the same deployStore entry, so neither owns the operation's state and
// unmounting either one (by navigating away) loses nothing.
export function DeployActions({ stack }: { stack: string }) {
  const { t } = useI18n();
  const { phase } = useDeployState(stack);
  const running = phase === "running";

  async function trigger(a: StackAction) {
    if (running) return;
    markStarted(stack, a);
    try {
      await runStackAction(stack, a);
      // Output + final status arrive via deploy:* events.
    } catch (err) {
      markFailed(
        stack,
        err instanceof Error ? err.message : "failed to start operation",
      );
    }
  }

  return (
    <div className="flex flex-wrap gap-2 px-5 py-4">
      {actions.map((a) => (
        <button
          key={a.id}
          onClick={() => trigger(a.id)}
          disabled={running}
          title={a.title}
          className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition disabled:opacity-40 ${
            a.id === "up"
              ? "bg-accent-600 text-zinc-950 hover:bg-accent-500"
              : "border border-zinc-700 text-zinc-300 hover:bg-zinc-800"
          }`}
        >
          <a.Icon className="h-3.5 w-3.5" />
          {t(a.labelKey)}
        </button>
      ))}
      {running && (
        <span className="flex items-center gap-1.5 self-center text-xs text-zinc-500">
          <SpinnerIcon className="h-3.5 w-3.5" />
          {t("stacks.opRunning")}
        </span>
      )}
    </div>
  );
}

// DeployOutput is the terminal-style pane for the current/last operation on a
// stack. It reads deployStore, so output that arrived while the user was on
// another page is all still here when they come back.
export function DeployOutput({ stack }: { stack: string }) {
  const { t } = useI18n();
  const { phase, action, lines, error } = useDeployState(stack);
  const paneRef = useRef<HTMLDivElement>(null);
  const running = phase === "running";

  // Auto-scroll to the latest line, including on remount (returning to the
  // page mid-operation should land at the tail, not the top).
  useEffect(() => {
    const el = paneRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  if (phase === "idle" && lines.length === 0) {
    return <p className="px-5 py-4 text-xs text-zinc-600">{t("stacks.runOp")}</p>;
  }

  return (
    <div className="px-5 py-4">
      <div className="mb-1.5 flex items-center gap-2 text-xs">
        <StatusPill phase={phase} />
        {running && <SpinnerIcon className="h-3.5 w-3.5 text-zinc-500" />}
        {action && (
          <span className="font-mono text-zinc-500">docker compose {action}</span>
        )}
      </div>
      <div
        ref={paneRef}
        className="h-96 overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-[11px] leading-relaxed text-zinc-300"
      >
        {lines.length === 0 && running && (
          <span className="text-zinc-600">{t("stacks.starting")}</span>
        )}
        {lines.map((l, i) => (
          <div key={i} className="whitespace-pre-wrap break-all">
            {l}
          </div>
        ))}
        {error && <div className="mt-1 text-red-400">✗ {error}</div>}
      </div>
    </div>
  );
}

function StatusPill({ phase }: { phase: DeployPhase }) {
  const map: Record<DeployPhase, { label: string; cls: string }> = {
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
