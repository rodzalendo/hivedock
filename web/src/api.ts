// Minimal API client. Server state flows through TanStack Query (see main.tsx);
// there is no global state store.

export interface Health {
  status: string;
  version: string;
  stacksDir: string;
  authDisabled: boolean;
  time: string;
}

export async function fetchHealth(): Promise<Health> {
  const res = await fetch("/api/health");
  if (!res.ok) {
    throw new Error(`health check failed: ${res.status}`);
  }
  return (await res.json()) as Health;
}
