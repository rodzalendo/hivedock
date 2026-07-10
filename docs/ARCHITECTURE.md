# Hivedock — Architecture

How Hivedock is built. Consolidates the decisions in `docs/CLAUDE.md` and
`docs/PLAN.md`; where those disagree with this file, this file is the design of
record — update both together. Read `docs/PRD.md` for *what/why*.

## 1. System overview

Single Go binary, single container, single host. No agents, no external
services, no database server.

```
┌────────────────────────── container ──────────────────────────┐
│  hivedock (Go)                                                  │
│                                                                 │
│   HTTP (chi)                     watchers                       │
│   ├── /api/health                ├── fsnotify(STACKS_DIR)       │
│   ├── /api/stacks[/{name}]       └── docker events ────┐        │
│   ├── /api/host/stats                                  ▼        │
│   ├── /api/home  (Phase 2)               events.Hub (debounced) │
│   ├── /api/updates (Phase 4)                  │                 │
│   └── /api/ws  ◄──────────────────────────────┘                 │
│         multiplexed: stacks:changed, logs:*, deploy:* (Ph3),    │
│         stats (later)                                           │
│                                                                 │
│   stacks.Manager ── scan compose files (source of truth)        │
│                  └─ docker.Client (read) ─┐                     │
│   compose runner (subprocess) ────────────┤                     │
│   store (SQLite: app state only) ─────────┘                     │
│   embedded SPA (React) via go:embed                             │
└──────────────┬─────────────────────────────┬───────────────────┘
      /var/run/docker.sock            STACKS_DIR (same path in/out)
```

**Backend:** Go 1.23, net/http + chi, `github.com/docker/docker/client` for
reads, subprocess `docker compose` for mutations, `modernc.org/sqlite` (no CGO).
**Frontend:** React 18 + TS + Vite + Tailwind, TanStack Query, embedded.
**Real-time:** one multiplexed WebSocket at `/api/ws`.

### Invariants (see CLAUDE.md for the full list)

1. Compose files are the source of truth. SQLite holds only app state.
2. File edits preserve formatting (targeted scalar edits, never parse-and-dump).
3. The UI never lies: external containers shown read-only; drift surfaced; real
   stderr on failure.
4. `STACKS_DIR` resolves to the same path inside and outside the container.

## 2. Data model & state

### 2.1 Truth model (Phase 1, implemented)

`stacks.Manager.List` = **scan** (`STACKS_DIR`, one level deep, parse compose
read-only) **⋈ merge** (running containers grouped by
`com.docker.compose.project`) → classified stacks:

- **managed** — a compose file exists under `STACKS_DIR`. Editable.
- **external** — a running compose project (or a plain `docker run` container)
  with no compose file here. Read-only.

Per-service status merges desired (compose `image:`) with runtime (container
state, ports). Drift = a running container whose `com.docker.compose.config-hash`
differs from `docker compose config --hash` of the current file (mtime-cached).
Daemon down → managed stacks still show, status `unknown`.

### 2.2 SQLite scope (app state only)

`settings` (kv: check interval, webhook, etc.), `service_prefs`
(`stack,service → hidden` for the homepage unhide toggle), and (Phase 4) cached
update-check results. **Never** stack definitions.

### 2.3 Real-time

`events.Hub` fans debounced (300 ms) change signals to WS clients. Sources:
fsnotify on `STACKS_DIR` (+ new-subdir watches), Docker events (filtered to
meaningful lifecycle actions), and a 30 s periodic rescan as a fallback where
inotify doesn't propagate (LXC double-bind, Docker Desktop virtiofs). Clients
refetch REST on `stacks:changed`; log lines stream inline on the same socket.

## 3. Discovery & homepage (Phase 2)

The differentiator: a **zero-config** dashboard. Every attribute has a priority
chain that ends in a sensible automatic default, so nothing needs labels — but
labels always win, and legacy `homepage.*` labels are honored for migration.

### 3.1 What becomes a card

A homepage **entry is derived per service** (not per container). A service is a
**candidate** if it is user-facing — see the auto-hide heuristic (§3.5). Hidden
services produce no card unless the user unhides them.

For a single-candidate stack the card represents the stack; multi-candidate
stacks yield one card per candidate service.

### 3.2 Attribute resolution (priority chains)

Namespaces: `hivedock.*` is primary, `homepage.*` is the migration fallback.
Labels may be set on the service (compose `labels:`) — service labels win over
stack-level. For each attribute, first match wins:

| Attribute | Chain |
|---|---|
| **name** | `hivedock.name` → `homepage.name` → humanized service name (single-candidate stack → stack name) |
| **group** | `hivedock.group` → `homepage.group` → stack name |
| **url** | `hivedock.url` → `homepage.href`/`homepage.url` → URL heuristic (§3.3) |
| **icon** | `hivedock.icon` → `homepage.icon` → icon matcher on the image (§3.4) → letter avatar |
| **description** | `hivedock.description` → `homepage.description` → empty |
| **hidden** | `hivedock.hidden` → `homepage.hidden`(=/showstats absent) → auto-hide heuristic (§3.5) → **user override in SQLite wins over all** |

`hivedock.*` labels also carry through into a stack's compose file as the
migration target; a "convert homepage.* → hivedock.*" helper is a post-v1
candidate, not v1.

### 3.3 URL heuristic

If a url label exists, use it verbatim. Otherwise derive from the service's
**published** host ports:

1. none published → no link (card still shows; likely auto-hidden anyway).
2. one → `scheme://<host>:<hostPort>`.
3. many → primary = the "best" port (prefer 80/8080/3000-ish http ports, else
   lowest), plus a **dropdown** of all published ports.

- **scheme**: https if the container port ∈ {443, 8443} or the published port
  hints TLS or a label says so; otherwise http.
- **host**: the host Hivedock is reached at — derived from the request `Host`
  header, overridable via a `PUBLIC_HOST`/base-URL setting. (DEPLOYMENT.md: give
  the box a static IP or links rot.)

### 3.4 Icon matcher

Goal: the right icon with zero config, resilient offline.

1. **Normalize** the image reference into a slug: strip registry
   (`docker.io`, `ghcr.io`, `lscr.io`, `quay.io`), strip org
   (`library/`, `linuxserver/`, …), strip tag/digest, strip arch prefixes
   (`arm64v8-`, `amd64-`), lowercase, kebab-case.
2. **Match** the slug against the **dashboard-icons** manifest
   (walkxcode/dashboard-icons) — a list of known icon slugs fetched **at build
   time** and embedded. Match against the manifest, never construct URLs blind
   (naming is irregular: `ubuntu` → `ubuntu-linux`). Keep a small alias table
   for common mismatches.
3. **Serve**: on first use fetch the actual icon (svg/png) from the CDN and
   cache it under `DATA_DIR/icons/`; subsequent loads are local.
4. **Fallbacks**: dashboard-icons miss → try **selfh.st/icons** → **letter
   avatar** (deterministic colored circle with the first letter). Always a
   fallback; never a broken image.

Icon resolution is server-side (`/api/home` returns an icon ref/URL the SPA
loads through Hivedock, so the browser never depends on external hosts at
runtime beyond the cached asset).

### 3.5 Sidecar auto-hide heuristic

Hide infrastructure that isn't a user destination, so the dashboard shows apps,
not plumbing:

- A service with **no published ports** is auto-hidden (pure backend), **or**
- the image matches a **known datastore/sidecar list** (postgres, mysql,
  mariadb, redis, valkey, mongo, rabbitmq, memcached, nats, etc.) — hidden even
  if it publishes a port.

Explicit `*.hidden` labels and the SQLite user override (unhide/hide toggle,
`service_prefs`) always win. Example: `app-with-db` → the web app shows, the
postgres sidecar is auto-hidden until the user unhides it.

### 3.6 Home view

Grouped card grid (by group), status dot per card (from the truth model),
search/filter, the host-stats strip, and an empty state
("No stacks found in `<STACKS_DIR>`…"). Cards link out via the resolved URL;
multi-port cards expose the dropdown.

## 4. Mutations (Phase 3, planned)

Auth (single admin, bcrypt, session cookie, CSRF, `AUTH_DISABLED` escape hatch)
lands before any mutation. Compose runner = subprocess wrapper for
up/down/restart/pull/stop with WS-streamed output and a per-stack concurrency
lock. Editor validates via `docker compose config` before save; save ≠ deploy.
Create-stack = name → directory + template compose. See PLAN Phase 3.

## 5. Updates (Phase 4, planned)

Registry v2 client (anonymous token auth for Hub/ghcr/lscr→ghcr/quay, HEAD
manifest digests, tag lists), a semver candidate engine (prefix/part-count/
suffix preservation, signature-tag exclusion, calendar-version awareness) built
**test-corpus-first** (`internal/registry/testdata/candidates.yaml`), a digest
path for mutable tags, comment-preserving targeted tag rewrites (env-interpolated
tags surfaced, never rewritten), and a cron scheduler. See PLAN Phase 4 and the
risk register.

## 6. Layout

```
cmd/hivedock/         entrypoint
internal/
  config/             env config
  docker/             read-only Docker SDK wrapper (list, events, logs)
  stacks/             scan + merge + classify + drift (truth model)
  compose/            read-only `docker compose` subcommands (config --hash);
                      Phase 3 adds the mutating runner
  events/             pub/sub hub (debounced)
  watch/              fsnotify + docker events + rescan → hub
  hoststats/          /proc sampler
  server/             chi router, REST handlers, multiplexed WS
  store/              SQLite + migrations (app state only)
  discovery/          (Phase 2) resolver + icon matcher
  registry/           (Phase 4) registry client + semver engine
web/                  React SPA (embedded via go:embed)
deploy/, scripts/, docs/
```
