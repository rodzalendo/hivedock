# Hivedock

Single-host Docker compose manager: **Dockge-style stack management** +
**auto-discovered homepage** + **WUD-style update checking**, in one container.

> Status: **Phase 0 — skeleton.** `docker compose up` serves the UI talking to a
> live backend. Stacks/Home/Updates/Settings land in later phases — see
> [docs/PLAN.md](docs/PLAN.md).

## Quick start (dev)

Prereqs: Go 1.23+, Node 20+, Docker, and [Task](https://taskfile.dev).

```sh
cp .env.local.example .env.local
task fixture         # create sample stacks in ./dev-stacks
task dev             # vite (:5173) + Go server (:5001); open http://localhost:5173
```

Or run the whole thing in a container:

```sh
task build           # multi-stage image build
task up              # docker compose up against ./dev-stacks
# open http://localhost:5001
```

## Layout

```
cmd/hivedock/        entrypoint (main)
internal/
  config/            env-based configuration
  server/            chi router: /api/health, /api/ws, SPA fallback
  store/             SQLite (app state only — compose files are source of truth)
web/                 Vite + React + TS + Tailwind; built output embedded via go:embed
deploy/              PCT 102 test-deploy compose + seed script
scripts/             dev fixtures
docs/                PRD / ARCHITECTURE / PLAN / DEPLOYMENT / CLAUDE
```

## Configuration (env)

| Var | Default | Meaning |
|---|---|---|
| `PORT` | `5001` | HTTP listen port |
| `STACKS_DIR` | `./dev-stacks` | Directory scanned for compose stacks (must match inside/outside the container) |
| `DATA_DIR` | `./data` | SQLite + app state |
| `AUTH_DISABLED` | `false` | Bypass auth (LAN test envs only) |
| `LOG_LEVEL` | `info` | `debug`\|`info`\|`warn`\|`error` |

## Tasks

`task --list` for all of them. Key ones: `dev`, `build`, `test`, `lint`,
`fixture`, `up`/`down`, `deploy`, `seed`.

See [docs/CLAUDE.md](docs/CLAUDE.md) for the non-negotiable invariants and
[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the PCT 102 test loop.
