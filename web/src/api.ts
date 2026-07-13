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

// A 401 from any request means the session lapsed (or never existed): notify the
// auth gate so it re-checks and drops back to the login screen.
function onUnauthorized() {
  window.dispatchEvent(new Event("hivedock:unauthorized"));
}

async function errorMessage(res: Response): Promise<string> {
  let msg = `${res.status} ${res.statusText}`;
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) msg = body.error;
  } catch {
    /* non-JSON error body */
  }
  return msg;
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    if (res.status === 401) onUnauthorized();
    throw new Error(await errorMessage(res));
  }
  return (await res.json()) as T;
}

// csrfToken reads the (non-HttpOnly) CSRF cookie the server set at login; it is
// echoed back in the X-CSRF-Token header on every state-changing request
// (double-submit-cookie CSRF defense).
function csrfToken(): string {
  const m = document.cookie.match(/(?:^|;\s*)hivedock_csrf=([^;]+)/);
  return m ? decodeURIComponent(m[1]) : "";
}

// mutate issues a state-changing request with the CSRF header attached, and maps
// error responses to thrown Errors.
async function mutate(
  url: string,
  method: "POST" | "PUT" | "DELETE",
  body?: unknown,
): Promise<Response> {
  const res = await fetch(url, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken(),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    if (res.status === 401) onUnauthorized();
    throw new Error(await errorMessage(res));
  }
  return res;
}

export interface HostStats {
  available: boolean;
  cpuPercent: number;
  memUsedBytes: number;
  memTotalBytes: number;
  diskUsedBytes?: number;
  diskTotalBytes?: number;
  numCpu: number;
  sampledAt?: string;
}

export interface PruneReport {
  imagesDeleted: number;
  spaceReclaimed: number;
}

// pruneSystem removes dangling images and stale build cache. Never touches
// tagged images, containers, volumes, or networks.
export async function pruneSystem(): Promise<PruneReport> {
  const res = await mutate("/api/system/prune", "POST");
  return (await res.json()) as PruneReport;
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
  explicitGroup?: boolean; // group came from a compose label, not the stack-name fallback
  url?: string;
  ports?: PortLink[];
  iconSlug?: string;
  stackSlug?: string; // icon fallback when the image slug has no asset
  icon?: string; // explicit icon label (may be a URL or a slug)
  description?: string;
  status: string;
  hidden: boolean;
  sidecar?: boolean; // visible helper service rolled up under its stack's primary card
}

export const fetchHome = () => getJSON<HomeEntry[]>("/api/home");

// HomeLayout is the user's dashboard arrangement. All fields optional; the
// server stores it as an opaque JSON object.
export interface HomeLayout {
  columns?: number; // group columns on wide screens (1-4); default 1
  tileSize?: number; // 1 (compact) | 2 (default) | 3 (large)
  sort?: "name" | "status" | "manual";
  groups?: string[]; // user-created group names
  cardGroups?: Record<string, string>; // "stack/service" -> group name ("" = default)
  cardOrder?: Record<string, string[]>; // group name -> card keys (manual sort)
  groupOrder?: string[]; // group names in display order
  groupColumns?: Record<string, number>; // group name -> column index (0-based)
}

export async function fetchHomeLayout(): Promise<HomeLayout> {
  const v = await getJSON<unknown>("/api/home/layout");
  // Tolerate anything non-object (older servers, mocks): fall back to {}.
  if (v && typeof v === "object" && !Array.isArray(v)) return v as HomeLayout;
  return {};
}

export async function saveHomeLayout(layout: HomeLayout): Promise<void> {
  await mutate("/api/home/layout", "PUT", layout);
}

export async function setServiceVisibility(
  stack: string,
  service: string,
  hidden: boolean,
): Promise<void> {
  await mutate(
    `/api/home/${encodeURIComponent(stack)}/${encodeURIComponent(service)}/visibility`,
    "PUT",
    { hidden },
  );
}

// setServiceName persists a custom display name for a service's card; pass an
// empty string to revert to the automatic name.
export async function setServiceName(
  stack: string,
  service: string,
  name: string,
): Promise<void> {
  await mutate(
    `/api/home/${encodeURIComponent(stack)}/${encodeURIComponent(service)}/name`,
    "PUT",
    { name },
  );
}

// setServiceIcon persists a custom icon (URL or dashboard-icons slug) for a
// service; pass an empty string to clear it and revert to the automatic icon.
export async function setServiceIcon(
  stack: string,
  service: string,
  icon: string,
): Promise<void> {
  await mutate(
    `/api/home/${encodeURIComponent(stack)}/${encodeURIComponent(service)}/icon`,
    "PUT",
    { icon },
  );
}

// ---- Auth ----

export interface AuthStatus {
  authDisabled: boolean;
  needsSetup: boolean;
  authenticated: boolean;
  username?: string;
}

export const fetchAuthStatus = () => getJSON<AuthStatus>("/api/auth/status");

export async function setupAdmin(
  username: string,
  password: string,
): Promise<void> {
  await mutate("/api/auth/setup", "POST", { username, password });
}

export async function login(
  username: string,
  password: string,
): Promise<void> {
  await mutate("/api/auth/login", "POST", { username, password });
}

export async function logout(): Promise<void> {
  await mutate("/api/auth/logout", "POST");
}

// ---- Stack mutations ----

export type StackAction = "up" | "down" | "restart" | "pull" | "stop" | "recreate";

export interface DeployAck {
  id: string;
  stack: string;
  action: StackAction;
}

// runStackAction triggers a compose lifecycle operation. It returns as soon as
// the operation is accepted (202); the streamed output arrives over the
// WebSocket as deploy:* messages (see useLiveUpdates → hivedock:deploy events).
export async function runStackAction(
  name: string,
  action: StackAction,
): Promise<DeployAck> {
  const res = await mutate(
    `/api/stacks/${encodeURIComponent(name)}/actions/${action}`,
    "POST",
  );
  return (await res.json()) as DeployAck;
}

// ---- Stack creation ----

export interface CreatedStack {
  name: string;
  dir: string;
  composeFile: string;
}

// createStack scaffolds a new managed stack (directory + template compose.yaml).
// It does not deploy — the caller edits on the Compose tab, then deploys.
export async function createStack(name: string): Promise<CreatedStack> {
  const res = await mutate("/api/stacks", "POST", { name });
  return (await res.json()) as CreatedStack;
}

// deleteStack stops any running containers, then removes the stack's directory
// under STACKS_DIR. Destructive and irreversible.
export async function deleteStack(name: string): Promise<void> {
  await mutate(`/api/stacks/${encodeURIComponent(name)}`, "DELETE");
}

// renameStack renames a managed stack's directory. The stack must be stopped
// first (a running project's name can't change without orphaning containers).
export async function renameStack(
  name: string,
  newName: string,
): Promise<CreatedStack> {
  const res = await mutate(
    `/api/stacks/${encodeURIComponent(name)}/rename`,
    "POST",
    { newName },
  );
  return (await res.json()) as CreatedStack;
}

// ---- Compose file editing ----

export interface ComposeFile {
  path: string;
  content: string;
}

export interface ValidateResult {
  valid: boolean;
  error?: string;
}

export const fetchCompose = (name: string) =>
  getJSON<ComposeFile>(`/api/stacks/${encodeURIComponent(name)}/compose`);

// validateCompose asks the server to run `docker compose config` on the draft
// without saving it. Always resolves (valid true/false); rejects only on a
// transport/auth error.
export async function validateCompose(
  name: string,
  content: string,
): Promise<ValidateResult> {
  const res = await mutate(
    `/api/stacks/${encodeURIComponent(name)}/compose/validate`,
    "POST",
    { content },
  );
  return (await res.json()) as ValidateResult;
}

// saveCompose validates server-side then writes the file (save ≠ deploy). A
// 422 (invalid compose) surfaces as a thrown Error carrying compose's message.
export async function saveCompose(
  name: string,
  content: string,
): Promise<ComposeFile> {
  const res = await mutate(
    `/api/stacks/${encodeURIComponent(name)}/compose`,
    "PUT",
    { content },
  );
  return (await res.json()) as ComposeFile;
}

// ---- Updates ----

export interface UpdateUsage {
  stack: string;
  service: string;
}

export type UpdateKind =
  | "unchecked"
  | "semver"
  | "digest"
  | "uptodate"
  | "error"
  | "unsupported";

export interface UpdateEntry {
  image: string;
  kind: UpdateKind;
  hasUpdate: boolean;
  current?: string;
  candidate?: string;
  diff?: "major" | "minor" | "patch";
  currentDigest?: string;
  latestDigest?: string;
  source?: string;
  error?: string;
  checkedAt?: string;
  ignored?: boolean;
  usedBy: UpdateUsage[];
}

export const fetchUpdates = () => getJSON<UpdateEntry[]>("/api/updates");

// setImageIgnore records/clears a user's choice to keep a pinned version and
// hide its update from "Update all". Keyed by the full image reference.
export async function setImageIgnore(
  image: string,
  ignored: boolean,
): Promise<void> {
  await mutate("/api/updates/ignore", "PUT", { image, ignored });
}

// updateService rewrites a service's image tag in its compose file (comment-
// preserving). Save ≠ deploy — deploy the stack separately to apply.
export async function updateService(
  stack: string,
  service: string,
  tag: string,
): Promise<void> {
  await mutate(
    `/api/stacks/${encodeURIComponent(stack)}/services/${encodeURIComponent(service)}/update`,
    "POST",
    { tag },
  );
}

// checkUpdates triggers a registry check across all managed-stack images. The
// results arrive asynchronously (updates:changed over the WebSocket). Returns
// the number of images queued; a 409 means a check is already running.
export async function checkUpdates(): Promise<{ images: number }> {
  const res = await mutate("/api/updates/check", "POST");
  return (await res.json()) as { images: number };
}

// ---- .env editing ----

export interface EnvFile {
  path: string;
  content: string;
  exists: boolean;
}

export const fetchEnv = (name: string) =>
  getJSON<EnvFile>(`/api/stacks/${encodeURIComponent(name)}/env`);

// saveEnv writes the stack's .env (creating it if needed). Save ≠ deploy — the
// change applies on the next deploy.
export async function saveEnv(name: string, content: string): Promise<EnvFile> {
  const res = await mutate(
    `/api/stacks/${encodeURIComponent(name)}/env`,
    "PUT",
    { content },
  );
  return (await res.json()) as EnvFile;
}

// ---- Settings ----

export interface Settings {
  stacksDir: string;
  dataDir: string;
  checkInterval: string;
  publicHost: string;
  authDisabled: boolean;
  version: string;
}

export const fetchSettings = () => getJSON<Settings>("/api/settings");

// saveSettings patches the editable settings; omit a field to leave it as-is.
// checkInterval: "off", a duration like "30m"/"6h", or "" to revert to env.
export async function saveSettings(patch: {
  checkInterval?: string;
}): Promise<Settings> {
  const res = await mutate("/api/settings", "PUT", patch);
  return (await res.json()) as Settings;
}

export const fetchHealth = () => getJSON<Health>("/api/health");
export const fetchStacks = () => getJSON<Stack[]>("/api/stacks");
export const fetchStack = (name: string) =>
  getJSON<Stack>(`/api/stacks/${encodeURIComponent(name)}`);
export const fetchHostStats = () => getJSON<HostStats>("/api/host/stats");
