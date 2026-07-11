# 🐝 Hivedock

**One container to run your homelab’s Docker: manage compose stacks, get a
zero-config dashboard, and check for image updates.** Dockge-style stack
management + a Homepage-style auto-discovered dashboard + WUD-style update
checking — in a single Go binary, no database server, no agents.

- **Stacks** — see every compose stack (and stray `docker run` containers),
  edit the compose file and `.env` in-browser, and deploy/stop/restart/pull with
  live streamed output. External stacks are shown read-only; drift is surfaced.
- **Home** — a zero-config dashboard. Cards, icons, and links are auto-derived
  from your compose files; legacy `homepage.*` labels are honored for migration.
- **Updates** — checks Docker Hub / ghcr / lscr / quay for newer image tags
  (semver-aware) and digest changes for mutable tags, then rewrites the tag in
  your compose file (comments preserved) and redeploys — one click.

> Compose files are always the source of truth. Hivedock’s SQLite holds only app
> state (settings, UI prefs, cached update results) — never your stack
> definitions. Point it at an existing `/opt/stacks` and it just works.

## 60-second install

Create a `compose.yaml` and `docker compose up -d`:

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
      # container — mount it 1:1 (see the volume below).
      - STACKS_DIR=/opt/stacks
      - DATA_DIR=/app/data
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /opt/stacks:/opt/stacks
      - hivedock-data:/app/data

volumes:
  hivedock-data:
```

Then open **http://<your-host>:5001** and create the admin account on the
first-run screen. That’s it — your existing stacks under `/opt/stacks` show up
immediately.

Multi-arch images (`amd64`, `arm64`) are published to
`ghcr.io/rodzalendo/hivedock` — works on a Pi or an x86 box alike.

> First pull failing with `unauthorized`/`denied`? A GHCR package starts
> **private**. After the first successful build, make it public
> (GitHub → the repo’s **Packages** → `hivedock` → **Package settings** →
> *Change visibility → Public*), or `docker login ghcr.io` on the host with a
> read-scoped token.

## Configuration (env)

| Var | Default | Meaning |
|---|---|---|
| `PORT` | `5001` | HTTP listen port |
| `STACKS_DIR` | `./dev-stacks` | Directory scanned for compose stacks. **Must resolve to the same path inside and outside the container.** |
| `DATA_DIR` | `./data` | SQLite + app state (cached icons, update results) |
| `AUTH_DISABLED` | `false` | Bypass login — trusted LAN only |
| `PUBLIC_HOST` | _(request host)_ | Host used to build dashboard links (set to a static IP/hostname so links don’t rot) |
| `CHECK_INTERVAL` | `6h` | How often to auto-check for image updates (`0`/`off` disables; use “Check now” anytime) |
| `WEBHOOK_URL` | _(none)_ | POSTed a JSON payload when **new** updates are found (also editable in Settings) |
| `LOG_LEVEL` | `info` | `debug`\|`info`\|`warn`\|`error` |

### Auth & CSRF

A single admin account (bcrypt) is created on first run; sessions are a
HttpOnly cookie and survive restarts. Every mutating request is CSRF-protected.
`AUTH_DISABLED=true` skips all of this for trusted LAN test environments.

### Reverse proxy / HTTPS

Front Hivedock with your proxy of choice and forward `X-Forwarded-Proto` so the
session cookie is marked `Secure` over HTTPS. The WebSocket at `/api/ws` needs
upgrade headers passed through.

## Migrating

- **From Dockge** — point `STACKS_DIR` at the same stacks directory Dockge used
  (e.g. `/opt/stacks`). Hivedock reads the same compose files in place; nothing
  is moved or rewritten on import. Your stacks appear as *managed* and are fully
  editable/deployable.
- **From Homepage** — no `services.yaml` needed. Cards are auto-discovered, but
  your existing `homepage.*` labels (`homepage.name`, `.group`, `.icon`,
  `.href`) are read as-is, so a labeled stack keeps its identity. Override any
  attribute with the primary `hivedock.*` labels.
- **From What’s Up Docker (WUD)** — the Updates tab replaces it: semver
  candidate selection (prefix/suffix/part-count preservation, signature-tag
  exclusion) plus a digest path for `latest`-style tags, with a webhook on new
  updates. Env-interpolated tags (`image: app:${TAG}`) are surfaced as
  “managed via .env”, never silently rewritten.

## How it works

Single Go binary (chi HTTP + one multiplexed WebSocket), the Docker SDK for
reads, and subprocess `docker compose` for mutations. The React SPA is embedded
via `go:embed`. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design
of record and [docs/PRD.md](docs/PRD.md) for what/why.

## Development

Prereqs: Go 1.23+, Node 20+, Docker, and [Task](https://taskfile.dev).

```sh
task fixture   # create sample stacks in ./dev-stacks
task dev       # vite (:5173) proxying to the Go server (:5001)
task test      # go test + vitest
task build     # multi-stage image (vite build → go build → alpine)
```

No local Go? Build/test in a container:

```sh
docker run --rm -v "$PWD:/src" -w /src -e CGO_ENABLED=0 golang:1.23-alpine \
  sh -c "go test ./internal/... ./cmd/..."
```

See [docs/CLAUDE.md](docs/CLAUDE.md) for the non-negotiable invariants and
[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the Proxmox/LXC deployment notes.

## License

See [LICENSE](LICENSE).
