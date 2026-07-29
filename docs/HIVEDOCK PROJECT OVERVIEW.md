# HiveDock — Project Overview

A single-page briefing on what HiveDock is, how it's built, and what it deliberately
isn't — meant as context for anyone (or anything) picking the project up cold. Read
`docs/PRD.md` for *what/why* and `docs/ARCHITECTURE.md` for the design of record.

*Written 2026-07-20, reflects release v1.2.6.*

## What it is

HiveDock is a self-hosted Docker Compose manager for a **single home server**. It is one
small Go container that replaces three tools people usually run side by side:

1. a stack manager (Dockge / Portainer),
2. an app start page (Homepage / Heimdall),
3. an image update checker (Watchtower / WUD).

Because all three live in one process reading the same compose files and the same Docker
socket, they know about each other: the dashboard builds itself from what is actually
running, and update checks know the currently deployed version so a stale tag can't pose
as a new release.

**Core promise:** your compose files stay the source of truth. HiveDock reads and edits
them in place — it never imports them into a database. Delete the container and every
stack keeps running.

- Repo: `github.com/rodzalendo/hivedock` (public, MIT)
- Image: `ghcr.io/rodzalendo/hivedock` — `:latest`, `:X.Y.Z`, `:edge` (per push to main)
- Go module path: `github.com/rogalinski/hivedock` (deliberately different from the GHCR org)
- Latest release: **v1.2.6**; running in production on a Proxmox LXC homelab host
- Written end to end (backend, frontend, tests, CI) with Claude

## The three pillars

### 1. Stacks
Every stack and container in one list. Compose and `.env` editing in the browser with
validation before save; deploy / pull / restart / stop / recreate with output streamed
live; per-service restart; per-service log tailing with severity coloring. Drift detection
compares a running container's compose config-hash against the current file and badges the
difference. Container health (`healthy` / `unhealthy` / `starting`) is parsed from the
Docker status string and surfaced as a dot + badge. Containers HiveDock didn't create are
shown read-only rather than hidden — the UI never lies about what's on the host.

### 2. Dashboard (Home)
Zero-config auto-discovered app grid. Cards, icons, and clickable links are derived from
compose files plus live container state — there is no dashboard YAML to write. Notable
behavior:
- Icons resolved via dashboard-icons slugs or a custom URL (proxied server-side, SSRF-guarded).
- Sidecar rollup: helper containers (redis, db) bundle under their primary app instead of
  becoming separate cards.
- Apps behind a VPN sidecar (`network_mode: service:gluetun`) borrow the sibling's published
  ports so e.g. qBittorrent-behind-gluetun still gets a working link.
- Per-card overrides (rename, icon, URL, hide), drag-and-drop groups/columns, persisted server-side.
- Legacy `homepage.*` labels are read as-is, so migrating costs nothing.

### 3. Updates
Checks Docker Hub, GHCR, LinuxServer, Quay, and any other v2 registry, with per-registry
credentials and custom TLS for private ones. Semver-aware: only suggests versions on your
current track, and cross-checks image build dates so a stale tag never reads as a new
release. Applying an update rewrites **only the `image:` line** in the compose file
(comments and formatting byte-preserved, verified by reconstruct-and-reparse) and
redeploys — one image or all at once, with a diff preview and confirm step. Nothing ever
updates on its own. Per-image pinning/ignore.

### Plus: it updates itself
The sidebar version pill offers one-click self-update. Releases are cosign-signed in CI
(keyless, GitHub OIDC); the running container verifies the signature **offline** using a
bundled cosign and a baked identity regexp, then pins the exact digest before touching
anything. A verification failure never becomes an offer.

## Architecture

Single Go binary, single container, single host. No agents, no external services, no
database server.

```
HTTP (chi) ──┬── /api/stacks, /api/home, /api/updates, /api/settings, /api/auth/*
             ├── /api/ws  (one multiplexed WebSocket: stacks:changed, logs:*,
             │             deploy:*, updates:changed)
             └── embedded React SPA (go:embed)

watchers:  fsnotify(STACKS_DIR)  +  docker events  ──► events.Hub (debounced)

stacks.Manager ── scans compose files (source of truth)
               └─ docker.Client for reads; subprocess `docker compose` for mutations
store          ── SQLite (modernc, no CGO): app state only
```

**Backend:** Go 1.23, net/http + chi, `docker/docker` client for reads, subprocess
`docker compose` for writes, `modernc.org/sqlite`, gorilla/websocket, yaml.v3, fsnotify.
~12.5k lines across `internal/` (auth, compose, config, discovery, docker, events,
hoststats, registry, server, stacks, store, updates, watch).

**Frontend:** React 18 + TypeScript + Vite + Tailwind, TanStack Query, CodeMirror for the
compose/.env editors, IBM Plex fonts. ~8k lines, four views (Dashboard, Stacks, Updates,
Settings) plus an auth screen. Built and embedded into the binary.

### Design invariants
1. Compose files are the source of truth; SQLite holds only app state.
2. File edits preserve formatting — targeted scalar edits, never parse-and-dump.
3. The UI never lies: external containers read-only, drift surfaced, real stderr on failure.
4. `STACKS_DIR` must resolve to the same path inside and outside the container.
5. No auth-bypass switch; proxy trust is decided by the real TCP peer, not a header.
6. No shell interpolation anywhere — `exec.Command` argument arrays only, CI-enforced.
7. Self-update applies only cosign-verified, digest-pinned images strictly newer than the
   running version.

## Security posture

HiveDock holds the Docker socket, which is root-equivalent, so the threat model assumes
LAN or authenticated-proxy exposure. A five-phase hardening program (documented in
`docs/HARDENING.md`, phases A–E) is complete and shipped as of v1.1.0:

- Single-admin auth, first-run setup gated by a one-time token printed to the log;
  HttpOnly session cookie (SHA-256 stored, 7d idle / 30d absolute), CSRF on every mutation,
  per-(user, IP) exponential-backoff login rate limiting.
- Optional SSO via forward-auth proxy (`AUTH_TRUSTED_HEADER` + `AUTH_TRUSTED_PROXY_CIDRS`).
- Symlink-resolved path containment on every file operation; lowercase stack-name allowlist.
- CSP with zero external origins; remote icons fetched through an SSRF-guarded server proxy.
- Optimistic locking on compose/.env saves (409 + reconcile UI); diff preview before any
  rewrite; fuzz tests on the rewriter.
- Opt-in git auto-commit of stack changes.
- Per-registry credentials and TLS; startup self-check that drops to read-only mode if the
  stacks bind mount isn't path-identical; read-only API token for monitoring tools.
- Cosign-signed releases with SLSA provenance and SBOM; CI runs staticcheck (blocking) and
  govulncheck.
- No telemetry, no phone-home. Outbound calls are limited to the GHCR version check, the
  registries your own stacks use, and icon lookups (cached).

Docs: `SECURITY.md`, `THREAT_MODEL.md`, `deploy/compose.hardened.yaml` (includes a
socket-proxy variant).

## UI / product surface

- **Six themes**, CSS-variable based, re-skinning the whole app: Hive Dark (default),
  Modern Glossy, Minimalist Paper, Fallout, Cyberpunk, Nord.
- **Five languages** — English, Polish, German, Spanish, French — via a dependency-free
  i18n layer (`useI18n().t(key)`, `{{var}}` interpolation, browser auto-detect).
- Design system: IBM Plex Sans/Mono/Serif, a remapped cool-gray scale, amber "hive" brand
  color plus a blue accent. No emoji anywhere in the product.
- URL-hash routing (`#/stacks/<name>`), live WebSocket updates throughout, host disk meter
  and one-click image-layer prune.

## Deliberate non-goals

- **Multi-host / clustering.** Scope is one box, on purpose.
- **Webhooks and notifications.** Built once, then removed at the owner's request.
- **Automatic updates.** Checks are automatic; applying is always a human click.
- **A host shell / terminal.** Considered and parked. If revisited it would be a
  per-container `exec` shell, never a host shell.

## Configuration reference

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `5001` | Listen port |
| `STACKS_DIR` | `/opt/stacks` | Scanned for `<stack>/compose.yaml`; must be path-identical in/out of container |
| `DATA_DIR` | `/app/data` | SQLite + app state |
| `PUBLIC_HOST` | request host | Host used for dashboard links |
| `CHECK_INTERVAL` | `30m` | Update-check cadence (`off` disables) |
| `AUTH_TRUSTED_HEADER` / `AUTH_TRUSTED_PROXY_CIDRS` | unset | Forward-auth SSO |
| `ADMIN_USER` / `ADMIN_PASSWORD_FILE` | unset | Bootstrap admin without the setup screen |
| `LOG_LEVEL` | `info` | debug / info / warn / error |

Optional per-service compose labels override any dashboard card:
`hivedock.name`, `hivedock.group`, `hivedock.icon`, `hivedock.url`, `hivedock.hidden`,
`hivedock.primary`.

## Repo map

```
cmd/hivedock/      entrypoint; also dispatches the `apply-update` self-update subcommand
internal/          Go packages (see architecture above)
web/               React SPA, embedded into the binary at build time
deploy/            production + hardened compose examples
dev-stacks/        sample stacks for local development
docs/              PRD, ARCHITECTURE, PLAN, HARDENING, DEPLOYMENT, CLAUDE, screenshots
.github/           CI (build/test/staticcheck/govulncheck) and signed release workflow
```
