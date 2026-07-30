import { useSyncExternalStore } from "react";
import type { StackAction } from "./api";

// deployStore buffers the output of in-flight compose operations *outside* the
// React tree.
//
// Operations run server-side and stream over the shared WebSocket, which stays
// open regardless of which view is mounted. When the buffer lived in
// DeployConsole's local state, navigating away (Stacks -> Home) unmounted the
// component and threw away everything received so far; coming back showed an
// idle console while the operation was still running. Keeping the buffer in a
// module singleton means the listener never unmounts, so the console is just a
// view over state that outlives it.

export type DeployPhase = "idle" | "running" | "ok" | "error";

export interface DeployState {
  phase: DeployPhase;
  action: StackAction | null;
  lines: string[];
  error: string | null;
}

interface DeployMessage {
  type: string;
  payload: {
    host?: string;
    stack?: string;
    action?: string;
    line?: string;
    ok?: boolean;
    error?: string;
  };
}

// Operations are keyed by host + stack so a same-named stack on two hosts never
// shares deploy state (docs/MULTIHOST.md). "local" is the implicit default.
function keyOf(host: string | undefined, stack: string): string {
  return `${host || "local"}/${stack}`;
}

// A frozen shared instance so getSnapshot returns a stable reference for every
// stack that has never run an operation — a fresh object each call would spin
// useSyncExternalStore forever.
export const IDLE: DeployState = Object.freeze({
  phase: "idle",
  action: null,
  lines: [],
  error: null,
});

// Cap the retained output per stack. A long `pull` on a big stack can emit
// thousands of progress lines, and this buffer is never garbage collected.
const MAX_LINES = 2000;

const states = new Map<string, DeployState>();
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

function update(key: string, next: Partial<DeployState>) {
  const prev = states.get(key) ?? IDLE;
  states.set(key, { ...prev, ...next });
  emit();
}

function appendLine(key: string, line: string) {
  const prev = states.get(key) ?? IDLE;
  const lines = [...prev.lines, line];
  states.set(key, {
    ...prev,
    lines: lines.length > MAX_LINES ? lines.slice(-MAX_LINES) : lines,
  });
  emit();
}

// The WebSocket (useLiveUpdates) fans deploy events out as DOM events on
// window. Subscribing at module scope means this runs for the whole session,
// not just while a console is mounted.
window.addEventListener("hivedock:deploy", (ev: Event) => {
  const msg = (ev as CustomEvent<DeployMessage>).detail;
  const stack = msg?.payload?.stack;
  if (!stack) return;
  const key = keyOf(msg.payload.host, stack);
  switch (msg.type) {
    case "deploy:start":
      states.set(key, {
        phase: "running",
        action: (msg.payload.action as StackAction) ?? null,
        lines: [],
        error: null,
      });
      emit();
      break;
    case "deploy:line":
      if (msg.payload.line !== undefined) appendLine(key, msg.payload.line);
      break;
    case "deploy:end":
      update(key, {
        phase: msg.payload.ok ? "ok" : "error",
        error: msg.payload.ok ? null : (msg.payload.error ?? "operation failed"),
      });
      break;
  }
});

// markStarted flips a stack to running the moment the user clicks, before the
// server's deploy:start arrives, so the buttons disable without a round-trip.
export function markStarted(host: string, stack: string, action: StackAction) {
  states.set(keyOf(host, stack), { phase: "running", action, lines: [], error: null });
  emit();
}

// markFailed records a failure to *launch* the operation (the POST itself
// failed) — no deploy:end will ever arrive for it.
export function markFailed(host: string, stack: string, error: string) {
  update(keyOf(host, stack), { phase: "error", error });
}

export function getDeployState(host: string, stack: string): DeployState {
  return states.get(keyOf(host, stack)) ?? IDLE;
}

// runningStacks reports the host/stack keys with an operation in flight, so the
// UI can show that work continues while the user is on another page.
export function runningStacks(): string[] {
  const out: string[] = [];
  for (const [key, st] of states) {
    if (st.phase === "running") out.push(key);
  }
  return out;
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

// useDeployState subscribes a component to one host/stack's operation state.
export function useDeployState(host: string, stack: string): DeployState {
  return useSyncExternalStore(
    subscribe,
    () => getDeployState(host, stack),
    () => IDLE,
  );
}
