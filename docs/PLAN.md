# Hivedock — Implementation Plan

Sequenced for Claude Code sessions. Each phase ends with something runnable and demoable; each task is sized to fit one focused session. Don't start a phase until the previous one's exit criteria pass.

## Phase 0 — Skeleton (1–2 sessions)

Goal: `docker compose up` on the repo yields a served (empty) UI talking to a live backend.

- [x] Repo scaffold per ARCHITECTURE.md layout; Go module; Vite+React+TS+Tailwind in `web/`
- [x] Go server: serve embedded SPA, `/api/health`, structured logging (slog), config via env (`STACKS_DIR`, `PORT`, `AUTH_DISABLED`, `DATA_DIR`)
- [x] Multi-stage Dockerfile (node build → go build → alpine), dev compose.yaml with socket + stacks mounts
- [x] Taskfile: `task dev` (vite dev server proxying to go), `task build`, `task test`
- [x] SQLite init + migrations scaffold
- [x] Dev fixture: script that creates 4–5 sample stacks in a local `./dev-stacks/` (whoami, nginx, redis+app, one with `homepage.*` labels) — every later feature gets tested against this
- [~] Test-env deploy pipeline: `task deploy` (build amd64 → `docker save | ssh docker load` → compose up on PCT 102) and `deploy/pct102.compose.yaml`; verify PCT 102 prerequisites and fill in the table in `docs/DEPLOYMENT.md` (storage driver must be overlay2, compose ≥ 2.20) — **scripts written; PCT 102 verification + table pending (needs SSH host)**
- [x] `deploy/seed-test-stacks.sh`: seeds the 7 test stacks from `docs/DEPLOYMENT.md` into `/opt/stacks-test` on PCT 102

Exit: container builds < 160 MB, UI loads, health check green — locally **and** served from PCT 102 via `task deploy`.

> Image size note (revised from the original < 100 MB): the runtime bundles the
> full `docker` CLI + compose plugin (~93 MB) so mutations shell out to
> `docker compose` and the CLI is available for in-container debugging. Our own
> footprint (Go binary + embedded SPA) is only ~10 MB; alpine base ~8 MB → ~149 MB
> total. Still far leaner than Dockge (~250 MB+). If we ever need < 100 MB, drop
> `docker-cli` and ship only the standalone compose plugin binary (~80 MB) —
> aligned with "Go SDK for reads, subprocess compose for mutations."

## Phase 1 — Read-only truth (2–3 sessions)

Goal: the UI accurately shows everything that exists. No mutations yet. This phase is where trust is built; do not rush it.

- [x] Docker client wrapper: list containers with compose labels, group by `com.docker.compose.project`
- [x] Stack scanner: walk `STACKS_DIR`, parse compose files (yaml lib, read-only), merge with running state → managed/external classification (+ standalone `docker run` containers)
- [x] fsnotify watcher + Docker events subscription → push state changes over WebSocket (debounced hub; 30s rescan fallback; event noise filtered)
- [x] Stacks view: list with status summaries; detail with Containers tab (name, image:tag, state, ports)
- [x] Drift detection: `docker compose config --hash` vs container `config-hash` labels; drift badge in UI (mtime-cached)
- [ ] Log streaming: `docker logs --follow` piped over WS, per-service filter, follow toggle — **only remaining Phase 1 item**
- [x] Host stats endpoint + strip component (/proc sampler; cgroup-limited view)

Exit (verified on PCT 102 against `/opt/stacks-test`): edit a compose file over SSH → drift badge appears without refresh (fsnotify, or ≤30s via rescan if inotify doesn't propagate through the LXC bind mounts — see DEPLOYMENT.md); `docker compose up` a new stack via SSH → it appears in UI within seconds; a plain `docker run` container shows as external/read-only.

> Progress note: read-only truth, real-time WS push, drift detection, and host
> stats are done and verified locally against a live daemon (incl. correctly
> showing the user's other `eduplan` project as external/read-only). Docker
> Desktop / Windows bind mounts don't deliver inotify (same class as the LXC
> double-bind caveat) — the 30s rescan covers it; the fs-watch code is proven by
> test on a native fs. Only **log streaming** remains before the PCT 102 exit
> verification.

## Phase 2 — Homepage (2–3 sessions)

Goal: the differentiator. Zero-config dashboard.

- [ ] Discovery resolver: name/url/group/hidden per priority chains in ARCHITECTURE.md §3, including `homepage.*` label fallback
- [ ] Icon matcher: build-time fetch of dashboard-icons manifest, normalization (strip registry/org/tag, kebab-case), runtime local cache, selfh.st fallback, letter avatar
- [ ] URL heuristic incl. multi-port dropdown, https detection
- [ ] Sidecar auto-hide heuristic + unhide toggle persisted in SQLite
- [ ] Home view: grouped card grid, status dots, search/filter, host stats strip
- [ ] Empty state ("No stacks found in /opt/stacks...")

Exit: fixture stacks render with correct icons and clickable URLs, zero labels required; the `homepage.*`-labeled fixture uses its labels.

## Phase 3 — Mutations (2–3 sessions)

Goal: full Dockge-equivalent lifecycle. Auth lands here, before anything can mutate.

- [ ] Auth: single admin (bcrypt, session cookie), first-run setup screen, `AUTH_DISABLED` path, CSRF
- [ ] Compose runner: subprocess wrapper for up/down/restart/pull/stop with WS-streamed output; concurrent-op lock per stack
- [ ] Compose tab: CodeMirror editor (yaml mode), validate via `docker compose config` before save, save ≠ deploy
- [ ] Create stack flow (name → directory + template compose.yaml)
- [ ] .env editor
- [ ] Terminal-style output pane for deploy operations

Exit (on PCT 102): create/edit/deploy/stop/delete a stack entirely from the UI against `/opt/stacks-test`; kill the Hivedock container mid-everything → all stacks unaffected. Passing this is the gate for pointing `STACKS_DIR` at the real `/opt/stacks` (deliberate switch, per DEPLOYMENT.md).

## Phase 4 — Updates (3–4 sessions)

Goal: the loop-closer. This is the hardest phase; the semver rules have sharp edges — port WUD's behavior, don't reinvent.

- [ ] Registry v2 client: anonymous token auth (Hub, ghcr, lscr→ghcr, quay), HEAD manifest digest, tags list with pagination, per-registry concurrency limit + retry/backoff
- [ ] Semver candidate engine: prefix preservation, part-count preservation, suffix matching, signature-tag exclusion, diff classification. **Table-driven tests first** — collect 30+ real-world image:tag cases (jellyfin, postgres `16` vs `16.4`, linuxserver `arm64v8-` prefixes, `-alpine` suffixes, calendar versioning like `2026.1`)
- [ ] Digest path for mutable tags; store results in SQLite
- [ ] Cron scheduler + "check now"
- [ ] Updates view: table, semver diff color-coding, row expand (digest, used-by stacks), bulk check
- [ ] Comment-preserving tag rewrite in compose.yaml (targeted scalar edit; test with commented, anchored, and env-interpolated files — `image: jellyfin:${TAG}` must be detected and surfaced as "managed via .env" not silently rewritten)
- [ ] "Update and redeploy" button + inline banner in stack view; changelog link via `org.opencontainers.image.source`
- [ ] Webhook trigger on new updates found (single URL, JSON payload)

Exit (on PCT 102): the seeded old-tag jellyfin stack shows the correct candidate, one click updates the file (comments intact) and redeploys; the `latest`-tag stack shows a digest-based update; the env-interpolated stack is flagged env-managed and untouched.

## Phase 5 — Polish & release (2 sessions)

- [ ] Mobile: Home as single column, Stacks read-mostly
- [ ] Dark/light theme, loading/error states pass
- [ ] Settings page (check interval, webhook, stacks dir display)
- [ ] README with 60-second install (Dockge-style copy-paste compose.yaml), screenshots, "migrating from Dockge/Homepage/WUD" section
- [ ] GitHub Actions: test, lint, multi-arch image build (amd64/arm64) to ghcr
- [ ] Dogfood on NanoHive (PCT 102, real `/opt/stacks` after the Phase 3 gate) for two weeks before announcing; fold `docs/TESTING-NOTES.md` LXC findings into the README's Proxmox troubleshooting section

Exit: a stranger can go from README to working dashboard in under 5 minutes.

## Risk register

| Risk | Mitigation |
|---|---|
| Semver edge cases produce wrong update suggestions (trust-killer) | Table-driven test corpus from real images before UI work; digest path as safe fallback; "dismiss this candidate" escape hatch |
| YAML rewrite mangles user files | CST-level targeted edit, never parse-and-dump; golden-file tests; env-interpolated tags surfaced, not rewritten |
| Docker Hub rate limiting | HEAD (not GET) manifests, 6h default interval, cache, per-registry concurrency 2, back off on 429 |
| Scope creep toward Portainer | PRD non-goals list is the contract; new features must displace something |
| compose CLI behavior differences across versions | Require compose v2.20+, check at startup, surface version in settings |

## Post-v1 candidates (parking lot, not commitments)

docker run → compose converter · multiple webhook targets · update batching/schedules ("apply all patch updates Sunday 3am" — this is the feature that could eventually beat Watchtower safely) · read-only public dashboard mode · Gotify/ntfy native · multi-host agents
