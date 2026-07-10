// Minimal API client. Server state flows through TanStack Query (see main.tsx);
// there is no global state store.

export interface Health {
  status: string;
  version: string;
  stacksDir: string;
  authDisabled: boolean;
  time: string;
}

export type Origin = "managed" | "external";
export type StackStatus = "running" | "partial" | "stopped" | "unknown";

export interface Port {
  ip?: string;
  public?: number;
  private: number;
  type: string;
}

export interface Service {
  name: string;
  image: string;
  runningImage?: string;
  state: string; // running | exited | created | absent | ...
  status?: string;
  containerId?: string;
  ports?: Port[];
}

export interface Stack {
  name: string;
  project: string;
  origin: Origin;
  status: StackStatus;
  dir?: string;
  composeFile?: string;
  services: Service[];
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* non-JSON error body */
    }
    throw new Error(msg);
  }
  return (await res.json()) as T;
}

export const fetchHealth = () => getJSON<Health>("/api/health");
export const fetchStacks = () => getJSON<Stack[]>("/api/stacks");
export const fetchStack = (name: string) =>
  getJSON<Stack>(`/api/stacks/${encodeURIComponent(name)}`);
