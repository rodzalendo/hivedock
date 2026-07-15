# Hivedock — Architecture

How Hivedock is built. Consolidates the decisions in `docs/CLAUDE.md` and
`docs/PLAN.md`; where those disagree with this file, this file is the design of
record — update both together. Read `docs/PRD.md` for *what/why*.

> **Status:** all four original phases (truth model, discovery/homepage,
> mutations, updates) are shipped, plus auth, self-update, per-card link
> overrides, and URL-hash routing. "Phase N" tags below are kept as
> provenance, not a roadmap — everything described here is live.

## 1. System overview

Single Go binary, single container, single host. No agents, no external
services, no database server.

```
┌────────────────────────── container ──────────────────────────┐
│  hivedock (Go)                                                  │
│                                                                 │
│   HTTP (chi)                     watchers                       │
│   ├── /api/health  /api/app/update (self-update)                │
│   ├── /api/stacks[/{name}]       ├── fsnotify(STACKS_DIR)       │
│   ├── /api/host/stats            └── docker events ────┐        │
│   ├── /api/home  /api/settings                         ▼        │
│   ├── /api/updates                       events.Hub (debounced) │
│   ├── /api/auth/*                             │                 │
│   └── /api/ws  ◄──────────────────────────────┘                 │
│         multiplexed: stacks:changed, logs:*, deploy:*,          │
│         updates:changed                                         │
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
5. No auth-bypass switch; trusted-header trust is decided by the real TCP peer.
6. No shell interpolation — `exec.Command` argument arrays only (CI-enforced).
7. Self-update applies only cosign-verified, digest-pinned images strictly newer
   than the running version.

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

`settings` (kv: check interval, home layout blob, admin credentials, session
tokens, plus the hardening keys — `app_update_mode`, `git_autocommit`,
`registry_config` JSON, and the SHA-256 of the read-only API token), `service_prefs`
(`stack,service → {hidden, icon, display_name, url}` — the per-card overrides),
`image_checks` (cached update-check results) and `image_ignores` (per-image pins).
Secrets in `settings` (session tokens, the API-token and registry passwords) are
protected by `DATA_DIR` file permissions, not at-rest encryption (THREAT_MODEL.md).
**Never** stack definitions. Migrations live in `store/migrations/*.sql`, applied
in filename order and tracked in `schema_migrations`.

### 2.3 Real-time

`events.Hub` fans debounced (300 ms) change signals to WS clients. Sources:
fsnotify on `STACKS_DIR` (+ new-subdir watches), Docker events (filtered to
meaningful lifecycle actions), and a 30 s periodic rescan as a fallback where
inotify doesn't propagate (LXC double-bind, Docker Desktop virtiofs). Clients
refetch REST on `stacks:changed`; log lines stream inline on the same socket.

## 3. Discovery & homepage

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
| **url** | user link override (`service_prefs.url`) → `hivedock.url` → `homepage.href`/`homepage.url` → URL heuristic (§3.3) |
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

The heuristic only sees ports published on the service's **own** container, so
two common setups produce no automatic link: host-network containers (no port
mappings exist — e.g. Jellyfin) and services sharing another's netns
(`network_mode: service:x`, ports live on `x` — e.g. qBittorrent behind
Gluetun). The **per-card link override** (`service_prefs.url`, set in the card
editor, wins over labels and the heuristic) is the reliable fix — persisted
server-side, survives stack rename, empty reverts to automatic.

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

A dense card grid — flat by default, or split into **user-defined groups** the
user builds in a Customize mode (create/rename/delete groups, drag cards and
group headers between real columns, pick column count 1–4 and tile size, sort by
name/status/manual). The whole arrangement is one opaque JSON blob in `settings`
(`home_layout`), owned by the frontend. Each card shows a status dot (truth
model), links out via the resolved URL (multi-port → dropdown), and carries
hover controls (edit name/icon/link, hide). **Sidecar rollup:** a stack's
non-primary services (its datastores and helpers) don't get their own tiles —
they collapse under the primary card's chevron (primary = a `hivedock.primary`
label, else the service whose slug matches the stack name). Plus search/filter,
the host-stats strip, and an empty state.

## 4. Mutations

Auth (single admin, bcrypt, HttpOnly session cookie, double-submit CSRF,
`AUTH_DISABLED` escape hatch) gates every mutation. The compose runner is a
subprocess wrapper for up/down/restart/pull/stop/recreate/**update**
(`up -d --pull always`, the digest-apply path) with WS-streamed output and a
per-stack concurrency lock. **Per-service restart** scopes a `restart` to one
service under the same lock. The editor validates via `docker compose config`
before save; save ≠ deploy. Create-stack = name → directory + template compose.
Rename/delete carry running-state guards and move `service_prefs` with the
stack.

## 5. Updates

Registry v2 client (anonymous token auth for Hub/ghcr/lscr→ghcr/quay, HEAD
manifest digests, tag lists), a semver candidate engine (prefix/part-count/
suffix preservation, signature-tag exclusion, calendar-version awareness, stale-
tag build-date guard) built **test-corpus-first**
(`internal/registry/testdata/candidates.yaml`), a digest path for mutable tags,
and comment-preserving targeted tag rewrites (env-interpolated tags surfaced,
never rewritten). A background scheduler re-reads the interval each minute so
Settings changes apply without a restart. Apply paths: semver → rewrite the
`image:` line + redeploy; digest → `up -d --pull always` (no tag to rewrite).
Per-image ignore pins a version (mutable even when up to date). No webhooks:
update state surfaces on the Updates page and the sidebar badge.

## 5a. Self-update (verified, digest-pinned — HARDENING.md §3)

Hivedock version-checks *itself* and verifies what it finds before offering it.

**Check** — `GET /api/app/update` compares the running version (build-time
`-ldflags`) against the newest `ghcr.io/rodzalendo/hivedock` tag (the semver
engine enforces strictly-greater plus the stale-tag guard — that *is* the
downgrade guard), then resolves the candidate's manifest digest and
cosign-verifies it against Hivedock's own release-workflow identity
(`…/release.yml@refs/tags/v*`, issuer `token.actions.githubusercontent.com`,
both baked in). Verification execs the **bundled cosign binary** (no new Go
deps, no shell — invariant 9) offline: the Rekor proof travels with the
signature and the sigstore trust root is seeded into the image, so the only
outbound call is fetching the signature from ghcr. A newer-but-unverifiable tag
is **not** offered — it surfaces a `verifyFailed` alert, because that alarm is
the point. The verified digest is cached (15 min) for the apply step. Skipped
for non-semver `dev`/`edge` builds and for update mode `off`.

**Apply** — `POST /api/app/update` (only in mode `full`) deploys that exact
verified digest. A container can't `compose up` itself, so Hivedock discovers
its own compose project from the `com.docker.compose.*` labels and launches a
**detached helper** — this same binary invoked as `hivedock apply-update`, from
the currently-running image pinned by *its own* repo digest (already-trusted
bytes). The helper pulls `…/hivedock@<digest>` by digest, retags it locally to
the compose `image:` ref, and runs `compose up -d --pull never` so the recreate
uses those precise bytes even if the remote tag moved since the check; a
post-check confirms the deployed image carries the approved digest. Every step
is an argument-array exec — no shell anywhere. The SPA polls `/api/health` and
reloads when the version changes. Failure modes (not compose-managed, pull
fails, verify fails) leave the old container running and report cleanly; SSH is
the universal fallback.

**Modes** (`app_update_mode` setting): `full` (verify + one-click apply,
default), `check-only` (verify + alert, no apply), `off` (no check at all).

**Release pipeline** — `.github/workflows/release.yml` builds multi-arch
(amd64+arm64), attaches SLSA provenance + an SBOM, and cosign-signs the pushed
digest keyless via GitHub Actions OIDC (no long-lived key; `id-token: write`,
gated on a `release` environment).

## 5b. Routing

The SPA uses the URL **hash** as its router (`#/stacks`, `#/updates`,
`#/settings`, `#/stacks/<name>` for an open stack) via a dependency-free hook,
so refresh and back/forward preserve the view. No server-side route config
needed; the `NotFound` SPA fallback serves `index.html`.

## 6. Security hardening (Phases A–E)

The full design + rationale is in `docs/HARDENING.md`; this is the architectural
map of what shipped and where. Trust model and residual risks: `THREAT_MODEL.md`.

- **Auth (A).** Single admin (bcrypt), HttpOnly session cookie stored as a SHA-256
  hash, double-submit CSRF on unsafe methods, per-(user,ip) login backoff. First
  run mints a one-time setup token. Optional trusted-header SSO decides trust from
  the real TCP peer captured by `capturePeer` *before* chi's RealIP rewrites
  `X-Forwarded-For`. `AUTH_DISABLED` is gone (boot-refuses). `server/auth.go`,
  `server/ratelimit.go`.
- **Verified self-update (B).** See §5a. `server/selfupdate.go` + `release.yml`.
- **Input/output (C).** `stacks.Contained` symlink-resolves every file target and
  confines it to `STACKS_DIR`; stack names are a lowercase allowlist. The CSP has
  zero external origins — custom icon URLs are fetched server-side by
  `discovery.RemoteIcon` behind a dial-time SSRF guard (`dialGuard`/`isPublicIP`)
  and served from `/api/icons/remote`. Log lines are escape-sanitized; a CI grep
  gate bans `sh -c` in non-test Go and `dangerouslySetInnerHTML`.
- **File trust (D).** Editor load returns the file's SHA-256; save must present it
  or gets a 409 with the on-disk version (`checkOptimisticLock`). The update
  rewrite is two-phase (unified-diff preview → confirm) and byte-exact: after the
  in-memory edit, `compose.verifyExactRewrite` reconstructs-and-compares + re-parses
  to prove only the image scalar changed, or aborts (fuzzed by `FuzzSetImageTag`).
  Opt-in `stacks` git helpers commit a snapshot-before + commit-after around each
  write (local only, no remotes).
- **Ops (E).** The shared `registry.Client` takes a `ConfigResolver` reading
  per-host credentials (Basic-auth token exchange) and TLS trust (CA bundle /
  insecure → cached per-host client) from `registry_config`; scheduled sweeps
  jitter their start. On boot, `server/systemcheck.go` inspects HiveDock's own
  container: a `STACKS_DIR` bind-parity mismatch drops it into read-only mode
  (`enforceReadOnly` refuses unsafe methods, banner via `/api/health`), and
  podman/rootless are flagged. A hashed, revocable read-only API token
  (`server/apitoken.go`) authorizes exactly three GET routes for monitoring tools.

## 7. Layout

```
cmd/hivedock/         entrypoint
internal/
  config/             env config
  docker/             Docker SDK wrapper (list, events, logs, self-inspect +
                      runtime detection for the boot self-check)
  stacks/             scan + merge + classify + drift (truth model); path
                      containment + opt-in git audit trail
  compose/            `docker compose` runner + validate + the byte-exact
                      image-tag rewriter
  events/             pub/sub hub (debounced)
  watch/              fsnotify + docker events + rescan → hub
  hoststats/          /proc sampler
  server/             chi router, REST handlers, multiplexed WS, auth,
                      self-update, deploy streaming, settings
  store/              SQLite + migrations (app state only)
  discovery/          resolver + icon matcher (zero-config homepage)
  updates/            update checker orchestration (registry ⋈ local images)
  registry/           registry client + semver candidate engine
  auth/               password hashing + token generation
web/                  React SPA (embedded via go:embed); hash router,
                      TanStack Query, views/ + components/
deploy/, scripts/, docs/
```
