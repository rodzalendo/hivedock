# Multi-host (remote environments)

HiveDock manages one Docker host: the daemon its container's socket is bound to.
This document describes how it manages **many** hosts — a VPS plus a home server,
several VPSes, a homelab of nodes — from a single instance, and the design that
gets us there safely and simply.

## The model: an outbound agent

The modern, safe consensus (Portainer Edge, Komodo Periphery, Dockge agents,
Dockhand Hawser) is an **agent that dials out to the manager**. HiveDock follows
it:

```
   remote host                         your HiveDock (the "manager")
 ┌──────────────┐    outbound wss     ┌───────────────────────────┐
 │ hivedock     │ ──────────────────► │  /api/agent/connect        │
 │   agent      │   (bearer token)    │  hostRegistry              │
 │  → docker.sock                     │  /api/hosts, /api/hosts/…  │
 └──────────────┘                     └───────────────────────────┘
```

Why outbound:

- **No inbound port on the remote host.** The agent opens the connection, so a
  home server behind NAT/CGNAT or a firewall needs no port-forward, no reverse
  proxy, no exposed Docker socket. This is the single biggest reason the field
  converged here.
- **The Docker socket never touches the network.** The agent talks to its local
  socket only; the wire carries HiveDock's own small RPC, not the raw Docker API.
  (Contrast Portainer's TCP+TLS path, which publishes the daemon — the least-safe
  option; HiveDock deliberately does not do that.)
- **The agent is the same binary.** `hivedock agent` reuses the exact
  `internal/docker` client the server already uses, so there is no second
  codebase to keep in sync — just a different entrypoint.

## The wire protocol

One WebSocket per agent. JSON frames, request/response correlated by `id`:

```
manager → agent   {"id":"7","method":"listContainers","params":{}}
agent → manager   {"id":"7","result":[ …containers… ]}
agent → manager   {"id":"7","error":"…"}          // on failure
```

On connect the agent sends a hello it isn't asked for:

```
agent → manager   {"method":"register","params":{"name":"home","version":"1.3.0"}}
```

The manager keeps a `hostRegistry` of live connections keyed by agent name, each
with a pending-call table so `Call(method, params)` sends a frame and blocks on
the matching reply (bounded by a timeout). Writes are serialized (gorilla
requires a single writer). A dropped socket removes the host from the registry;
the agent reconnects with backoff.

**Streaming (Phase 2).** Deploy output, log follow, and the exec shell are streams,
so the same envelope carries a `Kind` (`"stream"` = an intermediate chunk; empty =
the terminal frame) and a machine `Code` on failure (so a remote error maps to the
same HTTP status as a local one — a 409 conflict even carries the current bytes for
the editor's reconcile flow). `CallStream` returns a channel of frames; a `cancel`
control frame (client disconnect / logs-unsubscribe) stops the agent's work, and
`execInput` / `execResize` frames tunnel the interactive shell over the same socket.
On a socket drop the manager drains every in-flight call with a synthetic
`offline` terminal, so remote deploys/logs unblock immediately.

## Security

The manager↔agent channel is a **trust boundary**: the manager asks the agent to
act on its Docker host. It is gated by:

- **A bearer token**, compared in constant time. Two ways to set it, either
  accepted: the env `AGENT_TOKEN` on the manager, or a token **minted in the UI**
  (Settings → Hosts), which is hashed in the DB and shown once — the agent carries
  it as `--token`. No token configured ⇒ the connect endpoint is disabled entirely
  (multi-host is opt-in). Rotate by regenerating (UI) or changing both ends (env).
- **TLS in production.** The agent dials `wss://` so the token and traffic are
  encrypted; the manager should sit behind the same HTTPS it already recommends.
- **The same security invariants run on the file-owning host.** Path confinement,
  the optimistic lock, the stack-name allowlist, and read-only mode all run on the
  host that owns the files — the agent for a remote stack — because both ends link
  the identical `internal/hostops` core. A bind-mismatched agent (`STACKS_DIR`
  parity check, §6.3) refuses writes independently, and every remote op is logged
  on both ends.
- **Outbound-only + no socket exposure**, as above — the remote host opens no
  ports and never speaks raw Docker to the network.

Per-agent tokens with an approve-pending-agents flow and mTLS are natural
hardening follow-ons; the shared/minted token is the simple, safe starting point.

## Enrolling a host

1. On the manager, open **Settings → Hosts** and click **Add a host**. This mints
   an agent token (shown once) and displays a ready-to-run command. (Or set the
   env `AGENT_TOKEN` on the manager instead — both are accepted.)
2. On the host you want to add, run the shown command:

   ```
   docker run -d --name hivedock-agent --restart unless-stopped \
     -v /var/run/docker.sock:/var/run/docker.sock \
     -v /path/to/your/stacks:/stacks \
     ghcr.io/rodzalendo/hivedock:latest \
     agent --manager https://your-manager.example.com \
           --token <token> --name homelab --stacks-dir /stacks
   ```

3. The host appears **online** in the Stacks **host switcher** (top of the Stacks
   page; it shows once at least one agent is connected). Switch to it to manage its
   stacks.

**Agent prerequisites.** The agent host needs the Docker socket **and the Docker
Compose plugin** (`docker compose version`) — the agent shells out to compose just
like the manager. Bind-mount the host's real stacks directory to `--stacks-dir` at
the **same path** the compose files expect; a mismatch trips the parity check and
the agent runs read-only until it's fixed (§6.3).

## Phases

**Phase 1 — visibility (done).** Agent mode + the manager registry + read-only
remote listing: every host and its containers in one place.

**Phase 2 — control (done).** The full stack-management surface is routed to the
selected host over the same RPC: list/detail, deploy with live output
(up/stop/restart/recreate/pull/update), per-service restart, compose & `.env`
edit, logs stream, create/rename/delete, per-stack image-update apply, and the
**remote per-container exec shell**. The portable core lives in
`internal/hostops` (a `Backend` interface with a `LocalBackend` the manager and
agent both link, and a manager-side `remoteBackend` that speaks the RPC), so local
and remote go through identical code with identical security invariants. UI
enrollment mints the token; a host switcher scopes the Stacks view. The Home
dashboard and the Updates page stay **local-only** for now.

**Phase 3 — polish.** Per-agent tokens + an approve-pending-agents enrollment
flow; per-host Home prefs + remote registry update-checks; remote host resource
stats; reconnect/health surfaced in the UI.

See `docs/MULTIHOST-TESTING.md` for the end-to-end acceptance scenario.
