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
  drifted?: boolean;
  containerId?: string;
  ports?: Port[];
}

export interface Stack {
  name: string;
  project: string;
  origin: Origin;
  status: StackStatus;
  drifted?: boolean;
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

export interface HostStats {
  available: boolean;
  cpuPercent: number;
  memUsedBytes: number;
  memTotalBytes: number;
  numCpu: number;
  sampledAt?: string;
}

export interface PortLink {
  label: string;
  url: string;
}

export interface HomeEntry {
  stack: string;
  service: string;
  name: string;
  group: string;
  url?: string;
  ports?: PortLink[];
  iconSlug?: string;
  icon?: string; // explicit icon label (may be a URL or a slug)
  description?: string;
  status: string;
  hidden: boolean;
}

export const fetchHome = () => getJSON<HomeEntry[]>("/api/home");

export async function setServiceVisibility(
  stack: string,
  service: string,
  hidden: boolean,
): Promise<void> {
  const res = await fetch(
    `/api/home/${encodeURIComponent(stack)}/${encodeURIComponent(service)}/visibility`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ hidden }),
    },
  );
  if (!res.ok) throw new Error(`failed to update visibility: ${res.status}`);
}

export const fetchHealth = () => getJSON<Health>("/api/health");
export const fetchStacks = () => getJSON<Stack[]>("/api/stacks");
export const fetchStack = (name: string) =>
  getJSON<Stack>(`/api/stacks/${encodeURIComponent(name)}`);
export const fetchHostStats = () => getJSON<HostStats>("/api/host/stats");
