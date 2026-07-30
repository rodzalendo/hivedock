import { useSyncExternalStore } from "react";

// hostStore holds the currently-selected host for the Stacks view, outside the
// React tree so the choice survives navigation and every consumer (the list
// query, the editors, deploy/log streams, the exec terminal) reads the same
// value. "local" is the manager's own host; any other name is a connected agent
// (docs/MULTIHOST.md). Mirrors deployStore's useSyncExternalStore singleton.

let current = "local";
const listeners = new Set<() => void>();

export function getHost(): string {
  return current;
}

export function setHost(name: string): void {
  const next = name || "local";
  if (next === current) return;
  current = next;
  for (const l of listeners) l();
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

// useHost subscribes a component to the selected host.
export function useHost(): string {
  return useSyncExternalStore(subscribe, getHost, () => "local");
}
