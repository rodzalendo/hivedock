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

## Security

The manager↔agent channel is a **trust boundary**: the manager asks the agent to
act on its Docker host. It is gated by:

- **A shared bearer token** (`AGENT_TOKEN` on the manager; `--token` on the
  agent), compared in constant time. No token set ⇒ the connect endpoint is
  disabled entirely (multi-host is opt-in). Rotate by changing both ends.
- **TLS in production.** The agent dials `wss://` so the token and traffic are
  encrypted; the manager should sit behind the same HTTPS it already recommends.
- **Least privilege, phased.** Phase 1 (below) is **read-only** — the agent only
  answers list/inspect calls, so a compromised manager token cannot mutate a
  remote host. Write methods (deploy, restart, exec) arrive in Phase 2 and are
  gated identically, honor the agent's own read-only mode, and are logged on the
  agent side too.
- **Outbound-only + no socket exposure**, as above — the remote host opens no
  ports and never speaks raw Docker to the network.

Per-agent tokens, an enrollment/approval step (an agent appears as "pending"
until the admin approves it), and mTLS are natural hardening follow-ons; the
single shared token is the simple, safe starting point.

## Phases

**Phase 1 — visibility (this milestone).** Agent mode + the manager registry +
read-only remote listing. You add `AGENT_TOKEN`, run `hivedock agent` on each
remote host, and HiveDock shows every host and its containers in one place. No
remote mutations yet — the smallest end-to-end slice that proves the transport,
enrollment, and UI, with the smallest blast radius.

**Phase 2 — control.** Route the write path (stack up/down, per-service restart,
compose read/write, logs stream, image updates) to the selected host over the
same RPC. Each remote op is gated by the token, respects the agent's read-only
mode, and is logged on both ends. A host switcher threads through Stacks / Home /
Updates so every view can target a chosen host.

**Phase 3 — polish.** Per-agent tokens + an approve-pending-agents enrollment
flow; remote per-container exec (reusing the Phase-1 exec stream, tunneled);
remote host resource stats; reconnect/health surfaced in the UI.

Keeping Phase 1 read-only is the "safe, simple" choice: it delivers the headline
value (all your hosts in one dashboard) without opening a remote-mutation surface
before its gating is designed and reviewed.
