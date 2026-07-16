<p align="center">
  <img src="web/public/favicon.svg" width="72" alt="HiveDock logo" />
</p>

<h1 align="center">HiveDock</h1>

<p align="center"><b>One small app to manage your Docker Compose stacks, show a dashboard of your apps, and check for image updates — for a single home server.</b></p>

<p align="center">
  <code>docker compose up -d</code> · point it at your stacks folder · done.
</p>

<!-- screenshot: add docs/screenshots/home.png then uncomment
![Home dashboard](docs/screenshots/home.png)
-->

---

## What it does

If you run Docker Compose on one box at home, you probably use three separate tools:

- One to manage stacks (like Dockge or Portainer).
- One to show a start page of your apps (like Homepage or Heimdall).
- One to check for image updates (like Watchtower or WUD).

That's three configs to keep in sync. The dashboard goes stale the moment you add a container and forget to update its YAML. And the update tool can't tell a real new version from an old leftover tag.

**HiveDock does all three in one place.** Because it already reads your compose files and talks to Docker, it *already knows* your stacks, services, ports, images, and health. So:

- The **dashboard builds itself** from what's actually running. You write no dashboard config, and it can't drift.
- **Update checks have real context** — HiveDock knows which version you're on and what a sensible next version is.

You keep editing your compose files like you always did. HiveDock just gives you a UI that reflects and respects them, instead of hiding them in a database.

Delete the HiveDock container and every stack keeps running, untouched.

## Who it's for

- **The homelabber.** You run maybe 5–40 compose stacks on a NAS, mini-PC, or Proxmox LXC. You like SSH and YAML and want to keep editing files directly — you just want a UI that shows the truth.
- **The self-hoster tidying up.** You're running Dockge + Homepage + Watchtower and you're tired of the moving parts. HiveDock reads your existing `homepage.*` labels as-is, so your dashboard keeps its look with no migration.

**Not for you if** you need multiple hosts, Kubernetes, a big admin console with teams and roles, or fully hands-off auto-updates. Those are on-purpose non-goals — see [docs/PRD.md](docs/PRD.md). HiveDock goes deep on one host, not wide across many.

## Features

### Stacks — manage what's running
- See **every compose stack and every stray container** in one list. Each is marked **managed** (has a compose file here, fully editable) or **external** (read-only — HiveDock won't pretend to own it).
- Edit **compose and `.env` files in the browser**. HiveDock checks them with `docker compose config` before it saves. Saving and deploying are separate steps.
- Run **lifecycle actions** — deploy, pull, restart, stop, and force-recreate — and watch the output **stream live** with a **progress bar**, not a frozen spinner.
- **Restart a single service** without touching the rest of the stack.
- **View logs** per service: follow live, filter, highlight errors and warnings, copy, and open a fullscreen reading view.
- **Drift detection.** If a running container no longer matches its compose file (you edited it over SSH, or another tool deployed it), HiveDock shows a *drift* badge, explains it in plain words, and offers a force-recreate to fix it.
- **Rename and delete** with guards so you can't break a running stack by accident.
- A live **resource strip** at the top shows CPU, memory, and disk that the container can actually see.

### Home — a dashboard you never set up
- The app grid is **built from your stacks automatically**. Names, icons, links, and grouping come from your compose files and containers. No dashboard YAML, ever.
- **Correct icons with no config**, matched from the image name against the [dashboard-icons](https://github.com/homarr-labs/dashboard-icons) set and cached locally. A clean letter avatar is used when there's no match.
- **Clickable links** built from published ports, with a per-app override for tricky cases (host-network apps, or a service sharing another's network).
- **Smart grouping.** Databases, caches, and helper containers don't clutter the grid — they tuck under their app's card and expand when you want them.
- **Customize anything, and it sticks.** Rename cards, set custom icons or links, hide or show things, make groups, drag cards between columns, pick the column count and tile size, sort by name or status. Saved on the server, per install.
- Everything is an override on top of a sensible default. Labels (`hivedock.*`, or old `homepage.*`) win if you set them, but nothing requires them.

### Updates — suggestions you can trust
- Checks for newer images on **Docker Hub, GHCR, LinuxServer (lscr.io), Quay, and any v2 registry**. Works anonymously by default, or with **per-registry logins and custom CA / TLS** for private and self-signed registries.
- **Version-aware.** It only suggests tags on *your* track — same prefix, same shape, same suffix (so `16` isn't confused with `16.4`, and `-alpine` stays `-alpine`) — and labels each jump major, minor, or patch.
- **Won't be fooled by old tags.** It checks the candidate image's build date against yours, so a stale legacy tag can never look like an upgrade.
- **Digest checks for `latest`-style tags.** When the tag name hasn't changed but the image behind it has, you get a "new digest" with a one-click **pull & redeploy**.
- **One-click apply.** For version updates, HiveDock rewrites just the `image:` line in your compose file (keeping comments and formatting) and redeploys — one image or all at once, with a progress bar.
- **Ignore per image** to stay on a version on purpose, even one that's currently up to date, so it never nags you. Tags set by variables (`image: x:${TAG}`) are flagged and never rewritten.
- Set a **background check interval** (or turn it off), or check now by hand. Updates show up on the Updates page and as a badge in the sidebar. No webhooks.

### Themes
- Six built-in looks: **Hive Dark** (default), **Modern Glossy**, **Minimalist Paper**, **Fallout**, **Cyberpunk**, and **Nord**. Pick one in Settings. The choice is saved in your browser.

### Runs itself, updates itself
- **Small image, multi-arch** (amd64 + arm64), works in Docker-in-LXC. SQLite (pure Go, no CGO) stores *only* app state — settings, UI preferences, cached results — never your stack files.
- **Login built in.** Single admin (bcrypt), CSRF protection on every change, and rate-limited logins. Optional SSO via a trusted-header proxy for a no-second-login setup.
- **Live updates** over one WebSocket — stack changes, deploy output, and logs all push to the browser. No polling, no manual refresh.
- **Real URLs.** Stacks, Updates, Settings, and each open stack have their own address, so refresh and bookmarks keep your place.
- **One-click self-update from the sidebar.** HiveDock checks for its own new release on load. Every release is cosign-signed, and HiveDock **verifies the signature and pins the exact image** before updating — a small helper swaps the container and the page reconnects on its own, no SSH. If a new tag fails the check, you get a warning, not an update. Choose `full`, `check-only`, or `off` in Settings.
- **Read-only API token** for monitoring tools (Uptime Kuma, Gatus, scripts). It only works on `GET /api/health`, `/api/stacks`, and `/api/updates` — never on changes.
- **Cleanup** with one click: prune dangling images and build cache to reclaim the disk that updates leave behind. It never touches tagged images, volumes, or networks.
- **Safety checks on boot.** If your stacks folder isn't mounted the same way inside and outside the container, HiveDock switches to read-only mode and tells you. It also warns on Podman / rootless Docker setups it can't fully drive.

## HiveDock vs. the three tools it replaces

| | Dockge / Portainer | Homepage / Heimdall | Watchtower / WUD | **HiveDock** |
|---|:--:|:--:|:--:|:--:|
| Manage compose stacks | ✅ | — | — | ✅ |
| Dashboard of apps | — | ✅ | — | ✅ **(zero-config, can't drift)** |
| Image-update checking | — | — | ✅ | ✅ **(version + build-date verified)** |
| Config you write for the dashboard | — | hand-written YAML | — | **none** |
| Files stay the source of truth | Dockge: ✅ | n/a | n/a | ✅ |
| Auto-update danger | — | — | opt-in, unattended | **never — you decide** |
| Self-updates from the UI | — | — | — | ✅ |
| Scope | one host | any | any | **one host, on purpose** |

## Install

Make a `compose.yaml` and run `docker compose up -d`:

```yaml
services:
  hivedock:
    image: ghcr.io/rodzalendo/hivedock:latest
    container_name: hivedock
    restart: unless-stopped
    ports:
      - "5001:5001"
    environment:
      # STACKS_DIR must be the SAME path inside and outside the container.
      # Mount it 1:1 (see volumes below).
      - STACKS_DIR=/opt/stacks
      - DATA_DIR=/app/data
      - PUBLIC_HOST=192.168.1.50:5001   # how you reach the box (keeps links stable)
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data
      - /opt/stacks:/opt/stacks
```

Open `http://<your-host>:5001`, create the admin account on the first screen, and your existing stacks show up right away. After that, you can update HiveDock itself with one click from the sidebar.

> **Image tags:** `:latest` is the newest stable release, `:X.Y.Z` pins one version, `:edge` follows every push to `main`. Point HiveDock's own compose at **`:latest`** (like the example) so one-click self-update stays clean. A pinned `:X.Y.Z` still updates, but you'd re-pin the tag by hand afterward.

## Configuration

Everything is set with environment variables:

| Variable | Default | What it does |
|---|---|---|
| `PORT` | `5001` | HTTP port to listen on. |
| `STACKS_DIR` | `/opt/stacks` | Folder scanned for `<stack>/compose.yaml`. **Must be the same path inside and outside the container** (compose resolves relative paths against it). |
| `DATA_DIR` | `/app/data` | SQLite, icon cache, and app state. |
| `PUBLIC_HOST` | request host | Host used to build dashboard links, e.g. `192.168.1.50:5001`. Set a static IP or hostname so links don't break. |
| `AUTH_TRUSTED_HEADER` | unset | SSO header carrying the logged-in user, e.g. `Remote-User`. Turns on trusted-header auth when set with the CIDRs below. |
| `AUTH_TRUSTED_PROXY_CIDRS` | unset | Comma-separated networks of your auth proxy. The header is trusted only when the direct connection comes from one of them. |
| `ADMIN_USER` | unset | First-run admin username (with `ADMIN_PASSWORD_FILE`). Used only when no admin exists yet. |
| `ADMIN_PASSWORD_FILE` | unset | Path to a file with the first admin password (for scripted installs). |
| `CHECK_INTERVAL` | `30m` | How often to check for updates in the background. `off` disables it; also editable live in Settings. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |

> `AUTH_DISABLED` was **removed** — it turned a tool that holds the Docker socket into an open door. The container won't start if it's still set. For no-second-login SSO, use trusted-header auth (above) behind Authelia, authentik, or Caddy.

### Login

A single admin account (bcrypt) is created on first run. So a stranger can't grab a fresh install first, setup needs a **one-time token printed to the container log** — get it with `docker logs hivedock` (look for `setup_token`). For scripted installs, set `ADMIN_USER` + `ADMIN_PASSWORD_FILE` instead.

Sessions are an HttpOnly cookie (stored as a SHA-256 hash, 7-day idle and 30-day hard limit) and survive restarts. Every change is CSRF-protected, and failed logins are rate-limited per user and IP.

For SSO without a second login, put HiveDock behind a forward-auth proxy (Authelia, authentik, Caddy `forward_auth`) and set `AUTH_TRUSTED_HEADER` plus `AUTH_TRUSTED_PROXY_CIDRS`. The header is trusted **only** when the real connection comes from one of those networks, so it can't be faked from outside.

### Reverse proxy / HTTPS

Put HiveDock behind your proxy and forward `X-Forwarded-Proto` so the session cookie is marked `Secure` over HTTPS. The WebSocket at `/api/ws` needs upgrade headers passed through.

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

Old `homepage.*` labels (`homepage.name`, `.group`, `.icon`, `.href`, `.description`) are read as-is, so a labeled stack keeps its look with no migration.

## How it works

**One picture of reality.** On every change, HiveDock scans `STACKS_DIR` one level deep and reads each `compose.yaml` (the *desired* state), then adds the running containers grouped by their compose project (the *actual* state). Joining the two gives per-service status and marks each stack **managed** (compose file here → editable) or **external** (running, no file here → read-only). If Docker is down, managed stacks still show with an `unknown` status. Nothing about your stacks is stored — the picture is rebuilt from files and Docker each time.

**Reads use the SDK; changes shell out.** HiveDock reads Docker through the official Go SDK, but every change runs the real `docker compose` command as a subprocess (up, pull, restart, stop, recreate), streamed to your browser and guarded by a per-stack lock so two actions never collide. It's never a reimplementation of compose — what you'd run by hand is what it runs. Compose files are only written in two places: your save in the editor, and the single `image:` line rewrite during an update (a careful edit that keeps comments and order).

**The update engine.** A registry client fetches tag lists and image digests (per-registry logins — anonymous by default, or authenticated when you add credentials). The version engine suggests only same-track tags and confirms each with a build-date check. For `latest`-style tags it compares digests instead.

**Self-update, safely.** A container can't `compose up` itself without killing itself mid-swap. So HiveDock starts a small detached helper container from the already-present image, which runs `compose pull && up -d` on the HiveDock project and outlives the swap. The browser waits for the new version to answer, then reloads. If anything's off, the old container is left alone and you get a clear message.

## Security & privacy

HiveDock holds the Docker socket, so it's honest about what that means: **socket access is root-equivalent.** Run HiveDock inside your network (LAN/VPN), and only expose it to the internet behind a reverse proxy that does its own auth.

- **Login with no bypass.** Single admin (bcrypt), CSRF-protected changes, rate-limited logins, hashed sessions. No default password — first run is gated by a token in the log. Optional forward-auth SSO (trust decided by the real connection, not a header anyone can fake).
- **Verified, pinned self-update.** Releases are cosign-signed (keyless, via GitHub Actions). HiveDock checks the signature against its own release workflow and deploys the exact verified image, refusing downgrades. A failed check shows a warning, never a silent update.
- **Files stay the source of truth.** The tag rewrite is exact or it aborts (fuzz-tested). Editor saves are lock-checked so they can't clobber an SSH edit. Machine edits show a diff first. Every file operation is kept inside `STACKS_DIR`. Optional local git history of changes.
- **Small browser surface.** Same-origin WebSocket, sanitized log output (no `innerHTML`), and a content-security policy with **zero external origins** (icons are proxied and SSRF-guarded).

**It doesn't phone home.** No analytics, no telemetry. The only outbound calls are: `ghcr.io` (its own version check + signature), the registries your stacks use (update checks), and icon sources once per new image (then cached). That's the whole list.

See **[THREAT_MODEL.md](THREAT_MODEL.md)** for trust boundaries and risks, **[SECURITY.md](SECURITY.md)** for how to report a vulnerability, and **[`deploy/compose.hardened.yaml`](deploy/compose.hardened.yaml)** for a locked-down setup (cap-drop, read-only rootfs, optional socket proxy). Licensed [MIT](LICENSE).

## How it's built

A Go backend (chi router, Docker SDK for reads, `docker compose` subprocess for changes) with a React + Tailwind frontend, built into one static Go binary. The runtime image runs on a small Alpine base and adds the Docker CLI + Compose plugin (which HiveDock drives), cosign (for verified self-update), and git (for the optional change history). SQLite (pure Go, no CGO) stores only app state — settings, UI preferences, cached update results. Your stacks live only in your compose files. Multi-arch images are published to GHCR on every tagged release.

The whole codebase was written with [Claude](https://claude.com): architecture, backend, frontend, tests, and CI.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design, [docs/PRD.md](docs/PRD.md) for the product spec and scope, and [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the reference deployment.
