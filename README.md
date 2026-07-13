<p align="center">
  <img src="web/public/favicon.svg" width="72" alt="HiveDock logo" />
</p>

<h1 align="center">HiveDock</h1>

<p align="center"><b>One container for your homelab's Docker: compose stack management, a zero-config app dashboard, and trustworthy image update checking. A single small binary.</b></p>

<!-- screenshot: add docs/screenshots/home.png then uncomment
![Home dashboard](docs/screenshots/home.png)
-->

## Why HiveDock

Most homelabs end up running three separate tools: one to manage compose stacks, one as a start page with app tiles, and one to watch for image updates. HiveDock does all three in one container, and does a few things you will not find elsewhere:

- **Zero-config dashboard.** No YAML to write for your start page. Cards, icons, links, and groups are derived from the compose files and containers you already have. Databases and sidecars auto-hide, and you can override anything per card (icon, visibility, group layout).
- **Update suggestions you can trust.** The version engine only proposes tags on the same track as yours (same prefix, suffix, and shape), and cross-checks the candidate's build date against your current image, so a stale legacy tag can never masquerade as an update. Deliberately pinned versions can be ignored per image, so a bulk update never overwrites your choice.
- **Your files stay the source of truth.** Stack definitions are never copied into a database. HiveDock reads the compose files in your stacks directory, and when it updates a tag it rewrites just that line, preserving comments and formatting. Point it at an existing stacks directory and it works immediately; nothing is ever locked in.
- **Honest about ownership.** Stacks it did not create (external compose projects, plain `docker run` containers) are shown read-only and clearly labeled. Config drift between a running container and its file is detected and explained in plain language.

## Features

- **Stacks**: every compose stack and stray container in one list, in-browser compose and `.env` editing with server-side validation, deploy / pull / restart / stop with live streamed output, rename and delete with running-state guards, per-service logs, drift detection.
- **Home**: auto-discovered app grid with icons (dashboard-icons plus custom icon URLs), clickable port links, hide/show per card, groups with custom titles, drag-and-drop arrangement, column count, and name/status sorting.
- **Updates**: semver-aware tag checking across Docker Hub, ghcr, lscr, and quay, digest checking for `latest`-style tags, one-click update and redeploy (single or bulk), per-image ignore for pinned versions.
- **Ops**: single binary with embedded UI, SQLite for app state only, WebSocket live updates, session auth with CSRF protection, multi-arch images (amd64 and arm64), works in Docker-in-LXC. HiveDock checks for its own updates on load and can update itself from the sidebar (one click — a detached helper container swaps the image and the page reconnects).

<!-- screenshots: add docs/screenshots/stacks.png and updates.png then uncomment
![Stacks view](docs/screenshots/stacks.png)
![Updates view](docs/screenshots/updates.png)
-->

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
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data
      - /opt/stacks:/opt/stacks
```

Open `http://<your-host>:5001` and create the admin account on the first-run screen. Your existing stacks show up immediately.

## Configuration

Everything is configured with environment variables:

| Variable | Default | What it does |
|---|---|---|
| `PORT` | `5001` | HTTP listen port. |
| `STACKS_DIR` | `/opt/stacks` | Directory scanned for `<stack>/compose.yaml`. Must be the same path inside and outside the container. |
| `DATA_DIR` | `/app/data` | SQLite, icon cache, app state. |
| `AUTH_DISABLED` | `false` | Skip login entirely. Only for trusted LANs. |
| `PUBLIC_HOST` | request host | Host used to build the dashboard's app links, e.g. `192.168.1.50:5001`. Set it to a static IP or hostname so links do not rot. |
| `CHECK_INTERVAL` | `30m` | Periodic update check cadence. `off` disables it; also editable live in Settings. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |

### Auth

A single admin account (bcrypt) is created on first run. Sessions are an HttpOnly cookie and survive restarts; every mutating request is CSRF-protected and login failures are rate-damped. `AUTH_DISABLED=true` skips all of it for trusted LAN setups.

### Reverse proxy / HTTPS

Front HiveDock with your proxy of choice and forward `X-Forwarded-Proto`, so the session cookie is marked `Secure` over HTTPS. The WebSocket at `/api/ws` needs upgrade headers passed through.

### Labels (optional)

The dashboard needs no labels, but you can override any card:

```yaml
services:
  app:
    image: ghcr.io/example/app:1.2.3
    labels:
      hivedock.name: My App
      hivedock.group: Media
      hivedock.icon: jellyfin        # dashboard-icons slug or a full image URL
      hivedock.url: https://app.lan
      hivedock.hidden: "true"
```

Existing `homepage.*` labels (`homepage.name`, `.group`, `.icon`, `.href`, `.description`) are also read as-is, so a labeled stack keeps its identity with zero migration work.

## How it is built

A Go backend (chi router, Docker SDK for reads, `docker compose` subprocess for mutations) with an embedded React + Tailwind frontend, compiled into one static binary in a multi-stage Docker build. SQLite (pure Go driver, no CGO) stores only app state: settings, UI preferences, cached update results. Stack definitions live exclusively in your compose files.

The entire codebase was written with [Claude](https://claude.com): architecture, backend, frontend, tests, and CI, end to end.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design of record and [docs/PRD.md](docs/PRD.md) for the product spec.
