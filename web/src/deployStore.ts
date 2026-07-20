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
    stack?: string;
    action?: string;
    line?: string;
    ok?: boolean;
    error?: string;
  };
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

function update(stack: string, next: Partial<DeployState>) {
  const prev = states.get(stack) ?? IDLE;
  states.set(stack, { ...prev, ...next });
  emit();
}

function appendLine(stack: string, line: string) {
  const prev = states.get(stack) ?? IDLE;
  const lines = [...prev.lines, line];
  states.set(stack, {
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
  switch (msg.type) {
    case "deploy:start":
      states.set(stack, {
        phase: "running",
        action: (msg.payload.action as StackAction) ?? null,
        lines: [],
        error: null,
      });
      emit();
      break;
    case "deploy:line":
      if (msg.payload.line !== undefined) appendLine(stack, msg.payload.line);
      break;
    case "deploy:end":
      update(stack, {
        phase: msg.payload.ok ? "ok" : "error",
        error: msg.payload.ok ? null : (msg.payload.error ?? "operation failed"),
      });
      break;
  }
});

// markStarted flips a stack to running the moment the user clicks, before the
// server's deploy:start arrives, so the buttons disable without a round-trip.
export function markStarted(stack: string, action: StackAction) {
  states.set(stack, { phase: "running", action, lines: [], error: null });
  emit();
}

// markFailed records a failure to *launch* the operation (the POST itself
// failed) — no deploy:end will ever arrive for it.
export function markFailed(stack: string, error: string) {
  update(stack, { phase: "error", error });
}

export function getDeployState(stack: string): DeployState {
  return states.get(stack) ?? IDLE;
}

// isAnyRunning reports whether any stack has an operation in flight, so the UI
// can show that work continues while the user is on another page.
export function runningStacks(): string[] {
  const out: string[] = [];
  for (const [stack, st] of states) {
    if (st.phase === "running") out.push(stack);
  }
  return out;
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

// useDeployState subscribes a component to one stack's operation state.
export function useDeployState(stack: string): DeployState {
  return useSyncExternalStore(
    subscribe,
    () => getDeployState(stack),
    () => IDLE,
  );
}
