# CLAUDE.md — Hivedock

Single-host Docker compose manager: Dockge-style stack management + auto-discovered homepage + WUD-style update checking, in one container. Read `docs/PRD.md` (what and why), `docs/ARCHITECTURE.md` (how), `docs/PLAN.md` (order of work) before implementing anything non-trivial. Test target is a real box: `docs/DEPLOYMENT.md` (Proxmox NanoHive, LXC PCT 102).

## Stack

- Backend: Go 1.23+, net/http + chi, `github.com/docker/docker/client`, SQLite via `modernc.org/sqlite` (no CGO)
- Frontend: React 18 + TypeScript + Vite + Tailwind, in `web/`, embedded via `go:embed`
- Compose operations: subprocess `docker compose ...`, never a reimplementation
- Real-time: single multiplexed WebSocket at `/api/ws`

## Commands

```
task dev      # vite dev server (:5173) proxying /api to go server (:5001)
task build    # full production build incl. embedded frontend
task test     # go test ./... && cd web && npm test
task lint     # golangci-lint + eslint
task fixture  # (re)create dev stacks in ./dev-stacks/
task deploy   # build amd64 image, docker save | ssh load to PCT 102, compose up (see docs/DEPLOYMENT.md)
task seed     # run deploy/seed-test-stacks.sh against PCT 102's /opt/stacks-test
```

Always run `task test` and `task lint` before considering a task done. Phase exit criteria are verified on PCT 102 (`task deploy`), not just against local fixtures.

## Non-negotiable invariants

1. **Compose files are the source of truth.** Never store stack definitions in SQLite. Never write to a compose file except: (a) explicit user save in the editor, (b) the targeted image-tag rewrite in the update flow.
2. **File edits preserve formatting.** Tag rewrites are targeted scalar edits on the `image:` line. Parse-and-dump YAML rewriting is forbidden — it destroys comments and ordering. If a tag is env-interpolated (`image: x:${TAG}`), do NOT rewrite; surface it as .env-managed.
3. **The UI never lies.** Containers we didn't create are shown (read-only), not hidden. Drift is always surfaced. Failed operations show real stderr, not generic errors.
4. **Stacks dir path must match inside/outside the container** (compose relative-path resolution). Don't break this assumption in code or docs.
5. **No mutations without auth.** Every mutating endpoint requires a valid session (+ CSRF for unsafe methods) or a trusted-proxy header from a configured CIDR. `AUTH_DISABLED` was removed — there is no auth-bypass switch, and one must never be reintroduced (see docs/HARDENING.md §2). Trusted-header trust is decided by the real TCP peer (captured before `X-Forwarded-For` rewriting), never a forwarded header.
6. **Scope:** check `docs/PRD.md` non-goals before adding features. Multi-host, k8s, widgets, auto-update are out.
7. **Test deployments target `/opt/stacks-test` on PCT 102.** Never point a dev/test build at the real `/opt/stacks`, and never run destructive operations against stacks Hivedock didn't create there. Switching to the real stacks dir happens once, deliberately, after Phase 3 exit criteria pass (see docs/DEPLOYMENT.md).

## Conventions

- Go: standard library first; justify each new dependency in the PR/commit message. Errors wrapped with `fmt.Errorf("context: %w", err)`. Table-driven tests.
- Registry/semver logic (`internal/registry/`): every behavior change needs a test case in the real-world corpus (`internal/registry/testdata/candidates.yaml`). This code has sharp edges; tests first.
- API: REST for request/response, WebSocket only for streams (logs, deploy output, events, stats). JSON errors: `{"error": "..."}` with correct status codes.
- Frontend: views in `web/src/views/{Home,Stacks,Updates,Settings}`, shared components in `web/src/components`. Server state via TanStack Query; no global state library. Dark mode default.
- Labels namespace: `hivedock.*` primary, `homepage.*` read as fallback (migration path — keep working).
- Commits: conventional (`feat:`, `fix:`, `refactor:`), small and focused.

## Testing against real Docker

Dev environment assumes a local Docker daemon. `task fixture` creates sample stacks in `./dev-stacks/` (set as `STACKS_DIR` in dev). Integration tests that need the daemon are tagged `//go:build integration`, run with `task test-integration`. Never run integration tests against `/opt/stacks` or any real stacks directory.

## Gotchas already discovered (don't relearn)

- `docker compose config --hash="*"` gives canonical per-service hashes; compare against container label `com.docker.compose.config-hash` for drift. Compute against resolved config, not the raw file (env interpolation changes the hash).
- Docker Hub digest HEAD: needs anonymous bearer token from `auth.docker.io` first; send OCI + Docker manifest-list Accept headers or you get the wrong digest for multi-arch images. HEAD doesn't count against pull limits; GET does.
- lscr.io is a redirect front for ghcr/other backends — follow redirects, auth against the terminal registry.
- dashboard-icons naming is kebab-case and not always the obvious name (`ubuntu` → `ubuntu-linux`); match against the manifest, don't construct URLs blind, always have the letter-avatar fallback.
- Semver candidates: preserve prefix (`v`), part count (`16` ≠ `16.4` family), and suffix (`-alpine`). linuxserver images may prefix arch (`arm64v8-`). Some projects use calendar versioning (`2026.1.0`) — plain semver compare still works, but never label those diffs major/minor/patch confidently; show tag names.
