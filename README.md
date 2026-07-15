<p align="center">
  <img src="web/public/favicon.svg" width="72" alt="HiveDock logo" />
</p>

<h1 align="center">HiveDock</h1>

<p align="center"><b>One container for your homelab's Docker: compose stack management, a zero-config app dashboard, and trustworthy image-update checking — in a single small binary.</b></p>

<p align="center">
  <code>docker compose up -d</code> · point it at your stacks directory · done.
</p>

<!-- screenshot: add docs/screenshots/home.png then uncomment
![Home dashboard](docs/screenshots/home.png)
-->

---

## The problem

If you self-host with Docker Compose on a single box, you probably run **three tools that don't know about each other**:

- **Dockge / Portainer** to manage stacks — edit compose, deploy, read logs.
- **Homepage / Heimdall** as a start page of app tiles — hand-maintained YAML that slowly drifts from what's actually running.
- **Watchtower / WUD** to find image updates — which either auto-updates and occasionally breaks things, or just notifies with no context.

Three configs, three label schemes, three mental models. The dashboard lies the moment you add a container and forget to edit its YAML; the update tool can't tell a real new release from a stale legacy tag.

## What HiveDock is

**One container that does all three, coherently, for one host.** The insight that ties them together: because HiveDock already reads your compose files and talks to Docker, it *already knows* your stacks, services, ports, images, and health. So the dashboard is free and can never drift — it **is** derived from reality — and update checks have real version context. You write no dashboard config and adopt no new label scheme.

It's a single Go binary with an embedded UI, ~30 MB image, no database server, no agents, no cloud. It reads the compose files you already have and never copies them into a database. Delete the HiveDock container and every stack keeps running, untouched.

## Who it's for

**The homelabber.** You run somewhere between 5 and 40 compose stacks on a NAS, a mini-PC, or a Proxmox LXC. You're comfortable with SSH and YAML and you *want* to keep editing files directly — you just want a UI that reflects and respects those files instead of hiding them behind a database. You value trust and predictability over a wall of features. This is the person HiveDock is built for.

**The tidy self-hoster consolidating.** You're currently running Dockge + Homepage + Watchtower and you're tired of the moving parts. HiveDock reads your existing `homepage.*` labels as-is, so your dashboard keeps its identity with zero migration work, and you get one thing to update and back up instead of three.

**Not the right fit if** you need multiple hosts, Kubernetes, a Portainer-style everything-console (registries, RBAC, teams), or hands-off automatic updates. Those are deliberate non-goals — see [docs/PRD.md](docs/PRD.md). HiveDock does depth on one host, not breadth across many.

## Feature tour

### Stacks — manage what's running
- Every compose stack **and** every stray `docker run` container in one list, each labelled **managed** (has a compose file here, fully editable) or **external** (read-only — the UI never pretends to own something it doesn't).
- In-browser **compose and `.env` editor**, validated with `docker compose config` before it will save. Save and deploy are separate steps.
- **Lifecycle actions** — deploy (up), pull, restart, stop, and a force-recreate — with output **streamed live** to the browser as it happens, so you watch the pull and the recreate in real time, not a spinner.
- **Per-service restart** — restart a single misbehaving container without touching the rest of the stack.
- **Per-service logs** with follow, per-service filtering and color coding, severity highlighting (errors/warnings stand out), a copy button (all shown, or the last N lines), and an enlarge-to-fullscreen reading view.
- **Drift detection** — when a running container's config no longer matches its compose file (you edited the file over SSH, or another tool deployed it), the stack shows a *drift* badge with a plain-language explanation and a force-recreate to clear it.
- **Rename and delete** with running-state guards so you can't foot-gun a live stack.
- A live **host-resource strip** (CPU, memory, disk) at the top — the numbers the container can actually see.

### Home — a dashboard you never configure
- An app grid **auto-derived from your stacks** — no dashboard YAML, ever. Names, icons, links, and grouping come from the compose files and containers you already have.
- **Correct icons with zero config**, matched from the image against the [dashboard-icons](https://github.com/homarr-labs/dashboard-icons) set, cached locally, with a clean letter-avatar fallback so a tile is never a broken image.
- **Clickable links** derived from published ports, with a per-card **link override** for the tricky cases the port heuristic can't see — host-network containers (e.g. Jellyfin) and services sharing another's network stack (e.g. qBittorrent behind Gluetun).
- **Smart bundling** — databases, caches, and sidecars (Postgres, Redis, an Immich machine-learning worker) don't clutter the grid; they roll up under their app's card and expand on demand.
- **Full customization that persists**: rename a card, set a custom icon or link, hide/show anything, create groups with custom titles, drag cards and groups between columns, pick the column count and tile size, sort by name or status. All stored server-side, per install.
- Everything is an **override on top of a sensible default** — labels (`hivedock.*`, or legacy `homepage.*`) win if present, but nothing *requires* them.

### Updates — suggestions you can trust
- Checks for newer images across **Docker Hub, GHCR, LinuxServer (lscr.io), and Quay**, with anonymous auth and multi-arch-correct digest handling.
- **Semver-aware**: it only proposes tags on *your* track — same prefix (`v`), same part count (`16` isn't the `16.4` family), same suffix (`-alpine`) — and labels the jump major / minor / patch. Calendar-versioned images are handled without a bogus semver label.
- **Trustworthy by construction**: the candidate's build date is cross-checked against your current image, so a stale legacy tag can never masquerade as an upgrade. This engine is built test-corpus-first against real-world image tags.
- **Digest checking** for `latest`-style mutable tags — when the tag hasn't moved names but the image underneath has, you get a "new digest" with a one-click **pull & redeploy**.
- **One-click apply**: semver updates rewrite just the `image:` line in the compose file (comments and formatting preserved) and redeploy; single or bulk ("update all").
- **Per-image ignore** to pin a version deliberately — including muting an image that's currently up to date, so a future update never nags you. Env-interpolated tags (`image: x:${TAG}`) are flagged as env-managed, never rewritten.
- A configurable **background check interval** (or off), and a manual "check now" — with a webhook-free, in-app model: updates surface on the Updates page and as a sidebar badge.

### Ops — runs itself, updates itself
- **Single binary, embedded UI**, multi-arch images (amd64 + arm64), works in Docker-in-LXC. SQLite (pure-Go, no CGO) stores *only* app state — settings, UI prefs, cached check results — never stack definitions.
- **Session auth** (single admin, bcrypt) with CSRF protection on every mutation and login rate-damping; an `AUTH_DISABLED` escape hatch for trusted LANs.
- **Live everything** over a single multiplexed WebSocket — stack changes, deploy output, and log streams all push to the browser; no polling, no manual refresh.
- **Real routing** — Stacks, Updates, Settings, and an open stack each live at their own URL, so a refresh or a bookmark keeps your place.
- **Verified self-update from the sidebar**: HiveDock checks for its *own* new release on every load; when one exists the version turns into a one-click update. Every release is cosign-signed (keyless, via GitHub Actions OIDC), and HiveDock **verifies that signature before offering the update and pins the exact verified digest** when applying it — a detached helper pulls those precise bytes, recreates the container, and the page reconnects by itself, no SSH required. If a newer tag ever fails verification you get an alert, not an offer. Choose `full` / `check-only` / `off` in Settings. Every release ships with generated GitHub release notes linked right there.
- **Maintenance**: a one-click prune of dangling images and build cache (never touching tagged images, volumes, or networks) to reclaim the disk that image updates leave behind.

## How it works

**The truth model.** On every change, HiveDock builds one picture of reality: it scans `STACKS_DIR` one level deep and parses each `compose.yaml` read-only (the *desired* state), then merges in the running containers grouped by their `com.docker.compose.project` label (the *actual* state). The join produces per-service status — desired image vs. running state and ports — and classifies each stack as **managed** (a compose file exists here → editable) or **external** (running but no compose file here → read-only). If the Docker daemon is down, managed stacks still render with an `unknown` status. Nothing about your stacks is stored; the model is rebuilt from files + Docker each time.

**Zero-config discovery.** Every card attribute — name, group, URL, icon, description, visibility — resolves through a priority chain that *ends* in a sensible automatic default: a `hivedock.*` label wins, then a legacy `homepage.*` label, then the heuristic. The URL comes from published ports (with an override for host/shared-network apps); the icon comes from normalizing the image reference to a slug and matching it against an embedded dashboard-icons manifest, fetched once and cached under `DATA_DIR`. Infrastructure without a user destination (portless services, known datastores) auto-hides so the dashboard shows apps, not plumbing — and your explicit hide/show always wins.

**Mutations shell out; reads use the SDK.** HiveDock reads Docker through the official Go SDK, but every state change runs the real `docker compose` binary as a subprocess (up/pull/restart/stop/recreate), streamed line-by-line to your browser and guarded by a per-stack lock so two operations never race. It is never a compose reimplementation — what you'd run by hand is what it runs. Compose files are only ever written in two places: your explicit save in the editor, and the single-line `image:` tag rewrite in the update flow (a targeted scalar edit that preserves comments and ordering — never a parse-and-dump).

**The update engine.** A registry v2 client fetches tag lists and manifest digests (anonymous bearer tokens per registry; OCI + Docker Accept headers so multi-arch digests are correct). The semver engine proposes only same-track candidates and confirms each with a build-date check against your running image. For mutable `latest`-style tags it compares digests instead. Env-interpolated tags are surfaced but never touched.

**Self-update, safely.** A container can't `compose up` itself — it would kill itself mid-recreate. So HiveDock discovers its own compose project from the labels Docker stamped on its container, launches a small **detached helper** container from the already-present image that runs `compose pull && up -d` on the HiveDock project, and survives its own replacement. The browser polls health and reloads when the new version answers. If anything's off, the old container is left untouched and you get a clear message.

**Real-time.** One `events.Hub` fans debounced change signals to all WebSocket clients from three sources — fsnotify on `STACKS_DIR`, filtered Docker lifecycle events, and a periodic rescan as a fallback where inotify doesn't propagate (LXC binds, Docker Desktop). Clients refetch on a change signal; logs and deploy output stream inline on the same socket.

## HiveDock vs. the three tools it replaces

| | Dockge / Portainer | Homepage / Heimdall | Watchtower / WUD | **HiveDock** |
|---|:--:|:--:|:--:|:--:|
| Manage compose stacks | ✅ | — | — | ✅ |
| Dashboard of apps | — | ✅ | — | ✅ **(zero-config, can't drift)** |
| Image-update checking | — | — | ✅ | ✅ **(semver + build-date verified)** |
| Config you write for the dashboard | — | hand-maintained YAML | — | **none** |
| Files stay the source of truth | Dockge: ✅ | n/a | n/a | ✅ |
| Auto-update danger | — | — | opt-in, unattended | **never — human decides** |
| Self-updates from the UI | — | — | — | ✅ |
| Scope | one host | any | any | **one host, on purpose** |

## Install

Create a `compose.yaml` and run `docker compose up -d`:

```yaml
services:
  hivedock:
    image: ghcr.io/rodzalendo/hivedock:latest
    container_name: hivedock
    restart: unless-stopped
    ports:
      - "5001:5001"
    environment:
      # STACKS_DIR must resolve to the SAME path inside and outside the
      # container. Mount it 1:1 (see volumes below).
      - STACKS_DIR=/opt/stacks
      - DATA_DIR=/app/data
      - PUBLIC_HOST=192.168.1.50:5001   # how you reach the box (keeps links stable)
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data
      - /opt/stacks:/opt/stacks
```

Open `http://<your-host>:5001`, create the admin account on the first-run screen, and your existing stacks appear immediately. From then on you can update HiveDock itself with one click from the sidebar.

> **Image tags:** `:latest` tracks stable releases, `:X.Y.Z` pins one, `:edge` follows every push to `main`. Point HiveDock's *own* compose at **`:latest`** (as the example above does) so the one-click self-update stays clean — it deploys the verified new image in place, and `:latest` keeps the tag and the registry in sync. A pinned `:X.Y.Z` still updates, but you'd re-pin the tag by hand afterwards.

## Configuration

Everything is configured with environment variables:

| Variable | Default | What it does |
|---|---|---|
| `PORT` | `5001` | HTTP listen port. |
| `STACKS_DIR` | `/opt/stacks` | Directory scanned for `<stack>/compose.yaml`. **Must be the same path inside and outside the container** (compose resolves relative paths against it). |
| `DATA_DIR` | `/app/data` | SQLite, icon cache, app state. |
| `PUBLIC_HOST` | request host | Host used to build the dashboard's app links, e.g. `192.168.1.50:5001`. Set it to a static IP or hostname so links don't rot. |
| `AUTH_TRUSTED_HEADER` | unset | Forward-auth (SSO) header carrying the authenticated user, e.g. `Remote-User`. Enables trusted-header auth when set together with the CIDRs below. |
| `AUTH_TRUSTED_PROXY_CIDRS` | unset | Comma-separated CIDRs of your auth proxy. The header is honored only when the direct TCP peer is inside one of them. |
| `ADMIN_USER` | unset | Non-interactive first-run admin username (with `ADMIN_PASSWORD_FILE`). Consumed only when no admin exists; ignored after. |
| `ADMIN_PASSWORD_FILE` | unset | Path to a file holding the bootstrap admin password (for CI/scripted installs). |
| `CHECK_INTERVAL` | `30m` | Background update-check cadence. `off` disables it; also editable live in Settings. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |

> `AUTH_DISABLED` was **removed** — it turned a socket-holding mutator into an unauthenticated proxy. The container refuses to boot if it's still set. For no-second-login SSO, use trusted-header auth (above) behind Authelia/authentik/Caddy.

### Auth

A single admin account (bcrypt) is created on first run. To stop a stranger who reaches a fresh install first from claiming it, setup requires a **one-time token printed to the container log** — grab it with `docker logs hivedock` (look for `setup_token`). For CI/scripted installs, set `ADMIN_USER` + `ADMIN_PASSWORD_FILE` to create the admin non-interactively instead.

Sessions are an HttpOnly cookie (stored server-side as a SHA-256 hash, with a 7-day idle and 30-day absolute expiry) and survive restarts; every mutating request is CSRF-protected, and failed logins are rate-limited per (user, IP) with exponential backoff.

For SSO without a second login, put HiveDock behind a forward-auth proxy (Authelia, authentik, Caddy `forward_auth`) and set `AUTH_TRUSTED_HEADER` to the header it injects (e.g. `Remote-User`) plus `AUTH_TRUSTED_PROXY_CIDRS` to the proxy's network. The header is trusted **only** when the request's real TCP peer is inside one of those CIDRs (checked before any `X-Forwarded-For` rewriting), so it can't be spoofed from outside.

### Reverse proxy / HTTPS

Front HiveDock with your proxy of choice and forward `X-Forwarded-Proto`, so the session cookie is marked `Secure` over HTTPS. The WebSocket at `/api/ws` needs upgrade headers passed through.

### Labels (optional)

The dashboard needs no labels, but you can override any card from the compose file (or, for most of these, right in the UI):

```yaml
services:
  app:
    image: ghcr.io/example/app:1.2.3
    labels:
      hivedock.name: My App
      hivedock.group: Media
      hivedock.icon: jellyfin        # dashboard-icons slug or a full image URL
      hivedock.url: https://app.lan  # for host/shared-network apps
      hivedock.hidden: "true"
      hivedock.primary: "true"       # make this the stack's main card
```

Existing `homepage.*` labels (`homepage.name`, `.group`, `.icon`, `.href`, `.description`) are read as-is, so a labeled stack keeps its identity with zero migration work.

## Security & privacy

HiveDock holds the Docker socket, so it's honest about what that means: **socket access is root-equivalent** — run HiveDock behind your network boundary (LAN/VPN), and only expose it to the internet behind a reverse proxy that does its own auth. It ships with single-admin auth (bcrypt, CSRF-protected mutations, login rate-damping), same-origin WebSocket enforcement, server-side sanitization of container log output, a baseline CSP, and a **cosign-verified, digest-pinned self-update** (keyless-signed releases; a failed signature check surfaces an alert instead of an update).

**It does not phone home.** No analytics, no telemetry. The only outbound calls are: `ghcr.io` to check for a newer HiveDock release and fetch its signature to verify (transparency-log proof checked offline against a baked-in trust root), the container registries your own stacks use (for image-update checks), and icon CDNs once per new image (then cached and served locally). That's the complete list.

See [SECURITY.md](SECURITY.md) for the full posture, the trust model, how to report a vulnerability, and current limitations (including the `AUTH_DISABLED` escape hatch — don't use it on an untrusted network).

## How it's built

A Go backend (chi router, Docker SDK for reads, `docker compose` subprocess for mutations) with an embedded React + Tailwind frontend, compiled into one static binary in a multi-stage Docker build. SQLite (pure-Go driver, no CGO) stores only app state: settings, UI preferences, cached update results. Stack definitions live exclusively in your compose files. Multi-arch images are published to GHCR on every tagged release.

The entire codebase was written with [Claude](https://claude.com): architecture, backend, frontend, tests, and CI, end to end.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design of record, [docs/PRD.md](docs/PRD.md) for the product spec and scope contract, and [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the reference deployment.
