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
- [x] Log streaming: `docker logs --follow` piped over multiplexed WS, per-service filter, follow toggle
- [x] Host stats endpoint + strip component (/proc sampler; cgroup-limited view)

Exit (verified on PCT 102 against `/opt/stacks-test`): edit a compose file over SSH → drift badge appears without refresh (fsnotify, or ≤30s via rescan if inotify doesn't propagate through the LXC bind mounts — see DEPLOYMENT.md); `docker compose up` a new stack via SSH → it appears in UI within seconds; a plain `docker run` container shows as external/read-only.

> Progress note: all Phase 1 features (read-only truth, real-time WS push, drift
> detection, host stats, log streaming) are code-complete and verified locally
> against a live daemon (incl. correctly showing the user's other `eduplan`
> project as external/read-only, and live-following a chatty container's logs).
> Docker Desktop / Windows bind mounts don't deliver inotify (same class as the
> LXC double-bind caveat) — the 30s rescan covers it; the fs-watch code is proven
> by test on a native fs. **Remaining before phase exit: verification on PCT 102**
> against `/opt/stacks-test` (needs the SSH deploy host configured).

## Phase 2 — Homepage (2–3 sessions)

Goal: the differentiator. Zero-config dashboard.

- [x] Discovery resolver: name/url/group/hidden per priority chains in ARCHITECTURE.md §3, including `homepage.*` label fallback
- [x] Icon matcher: dashboard-icons CDN fetch + alias table, runtime local cache (DATA_DIR/icons), letter-avatar fallback (UI), negative cache — **note:** runtime CDN fetch instead of build-time manifest embed; selfh.st fallback deferred (post-v1 refinement)
- [x] URL heuristic incl. multi-port dropdown, https detection
- [x] Sidecar auto-hide heuristic + unhide toggle persisted in SQLite
- [x] Home view: grouped card grid, status dots, search/filter, host stats strip
- [x] Empty state ("No stacks found in /opt/stacks...")

Exit: fixture stacks render with correct icons and clickable URLs, zero labels required; the `homepage.*`-labeled fixture uses its labels.

> Progress note: Phase 2 code-complete and verified locally against a live
> daemon — `homepage.*` labels honored (name/group/icon), postgres/redis
> sidecars auto-hidden, mailpit multi-port dropdown, URLs from `PUBLIC_HOST`,
> icons fetched+cached from the dashboard-icons CDN (incl. `whoami`→`traefik`
> alias) with 404→letter-avatar. Two deliberate deviations from the bullet: icon
> resolution is runtime-CDN + cache rather than a build-time embedded manifest,
> and the selfh.st secondary source is deferred (letter avatar already covers
> misses). Remaining before phase exit: verification on PCT 102.

## Phase 3 — Mutations (2–3 sessions)

Goal: full Dockge-equivalent lifecycle. Auth lands here, before anything can mutate.

- [x] Auth: single admin (bcrypt, session cookie), first-run setup screen, `AUTH_DISABLED` path, CSRF
- [x] Compose runner: subprocess wrapper for up/down/restart/pull/stop with WS-streamed output; concurrent-op lock per stack
- [x] Compose tab: CodeMirror editor (yaml mode), validate via `docker compose config` before save, save ≠ deploy
- [x] Create stack flow (name → directory + template compose.yaml)
- [x] .env editor
- [x] Terminal-style output pane for deploy operations

Exit (on PCT 102): create/edit/deploy/stop/delete a stack entirely from the UI against `/opt/stacks-test`; kill the Hivedock container mid-everything → all stacks unaffected. Passing this is the gate for pointing `STACKS_DIR` at the real `/opt/stacks` (deliberate switch, per DEPLOYMENT.md).

> Progress note: **auth is done** and verified end-to-end against a running
> container. Single admin (bcrypt), DB-backed sessions (survive restart),
> HttpOnly session cookie + readable CSRF cookie with double-submit `X-CSRF-Token`
> enforcement on unsafe methods, first-run setup screen, login/logout, and the
> `AUTH_DISABLED` passthrough. Whole `/api` is gated except health + the auth
> bootstrap (status/setup/login). New: `internal/auth` (bcrypt+tokens),
> `internal/store/auth.go` + migration `0002_auth.sql`, `internal/server/auth.go`,
> and the SPA `AuthGate`/`AuthScreen`. Image still 152 MB.
>
> Progress note: **compose runner + deploy output pane done** and verified live
> against the daemon (socket-mounted container operating on a real whoami stack).
> `internal/compose/runner.go` shells out to `docker compose --ansi never …`
> for up/down/restart/pull/stop with a per-stack in-flight lock (409 on
> concurrent op). Mutations are triggered over an **authenticated, CSRF-protected**
> `POST /api/stacks/{name}/actions/{action}` (external stacks 409'd as read-only),
> which runs on a background context (a browser refresh can't abort a deploy) and
> streams combined stdout+stderr back over the WS as `deploy:{start,line,end}`
> broadcasts (`events.Hub.Publish`). SPA: `DeployConsole` (action buttons +
> terminal pane) on a new managed-only "Deploy" tab; `useLiveUpdates` fans
> `deploy:*` out via a `hivedock:deploy` DOM event and refetches on end. Verified:
> up creates+serves the container, restart streams live lines, down removes it,
> external→409.
>
> Progress note: **Compose editor tab done** and verified live. Backend:
> `compose.Validate` runs `docker compose -f - config -q` (candidate piped on
> stdin — never written until valid, evaluated in the stack's project dir so
> `.env`/relative paths resolve); `GET/PUT/POST /api/stacks/{name}/compose[/validate]`
> (managed-only, auth+CSRF, 1 MiB cap, atomic temp-file+rename write preserving
> mode). Save validates server-side (422 with compose's own stderr on failure)
> and **only writes the file — never redeploys** (drift surfaces until the user
> deploys). SPA: `ComposeEditor` (CodeMirror `@uiw/react-codemirror` +
> `@codemirror/lang-yaml`, Save/Validate/Revert, unsaved badge) on a managed-only
> "Compose" tab. Image 153 MB (CodeMirror +1 MB). Verified: get, validate
> valid/invalid, save-valid (file updated on disk), save-invalid→422 (file intact,
> no temp leftovers).
>
> Progress note: **create-stack done** and verified with a full end-to-end loop.
> `POST /api/stacks` (`internal/server/create.go`, auth+CSRF) validates the name
> against `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$` (+ a child-of-root containment
> check → no traversal), makes `<STACKS_DIR>/<name>/` and writes a template
> `compose.yaml` (nginx starter), 409 if it exists, rolls back the dir on write
> failure. SPA: an inline `NewStack` form in the Stacks list header; on success it
> selects the new stack and opens it on the **Compose** tab (StackDetail now keyed
> per stack with an `initialTab`). Verified live: bad name→400, create→201 +
> template on disk, appears as managed/stopped, dup→409, and the whole
> **create → edit/save compose → deploy → running** loop through the REST API
> (whoami serving on a host port).
>
> Progress note: **.env editor done — Phase 3 is now code-complete.**
> `GET/PUT /api/stacks/{name}/env` (`internal/server/env.go`, managed-only,
> auth+CSRF, atomic write reusing `atomicWrite`; a missing `.env` is a normal 200
> `exists:false`, PUT creates it — no `docker compose config` validation, it's
> plain KEY=VALUE). SPA: `EnvEditor` (CodeMirror, Save/Revert, new/unsaved badges)
> on a managed-only "Env" tab. Verified end-to-end that the written `.env` is what
> compose reads: a `${TAG}`-interpolated compose + `TAG=v1.10.1` in `.env` deployed
> the exact `traefik/whoami:v1.10.1` image.
>
> **All Phase 3 bullets are checked.** The only thing between here and the Phase 3
> exit gate is the same standing blocker as Phases 1–2: **verification on PCT 102**
> (needs `HIVEDOCK_DEPLOY_HOST` / SSH). Everything above is verified locally
> against Docker Desktop. Passing the PCT 102 exit gate is what unlocks pointing
> `STACKS_DIR` at the real `/opt/stacks`.

## Phase 4 — Updates (3–4 sessions)

Goal: the loop-closer. This is the hardest phase; the semver rules have sharp edges — port WUD's behavior, don't reinvent.

- [x] Registry v2 client: anonymous token auth (Hub, ghcr, lscr→ghcr, quay), HEAD manifest digest, tags list with pagination, per-registry concurrency limit + retry/backoff
- [x] Semver candidate engine: prefix preservation, part-count preservation, suffix matching, signature-tag exclusion, diff classification. **Table-driven tests first** — collect 30+ real-world image:tag cases (jellyfin, postgres `16` vs `16.4`, linuxserver `arm64v8-` prefixes, `-alpine` suffixes, calendar versioning like `2026.1`)
- [x] Digest path for mutable tags; store results in SQLite
- [x] Cron scheduler + "check now"
- [x] Updates view: table, semver diff color-coding, row expand (digest, used-by stacks), bulk check
- [x] Comment-preserving tag rewrite in compose.yaml (targeted scalar edit; test with commented, anchored, and env-interpolated files — `image: jellyfin:${TAG}` must be detected and surfaced as "managed via .env" not silently rewritten)
- [x] "Update and redeploy" button + inline banner in stack view; changelog link via `org.opencontainers.image.source`
- [x] Webhook trigger on new updates found (single URL, JSON payload)

Exit (on PCT 102): the seeded old-tag jellyfin stack shows the correct candidate, one click updates the file (comments intact) and redeploys; the `latest`-tag stack shows a digest-based update; the env-interpolated stack is flagged env-managed and untouched.

> Progress note: **Phase 4 started with the semver candidate engine, test-corpus
> first** (as the risk register demands). `internal/registry/semver.go` —
> `Candidate(current, available) → (tag, DiffType, ok)`: strips+preserves arch
> prefixes (`arm64v8-`, `amd64-`, …) and a leading `v`, splits a dotted-numeric
> core from a preserved suffix (`-alpine`/`-ubuntu`), enforces same-track matching
> (identical prefix + suffix + component count → the `16` vs `16.4` and single vs
> multi-part rules), numeric (not lexical) comparison, signature/attestation
> exclusion (`sha256-…`, `*.sig`), and first-differing-component diff
> classification (major/minor/patch). The contract is `testdata/candidates.yaml`
> (34 real-world cases; test fails if <30) — all pass. **Next Phase 4:** registry
> v2 client (anon token auth, HEAD manifest digest, paginated tags list,
> per-registry concurrency + backoff), then digest path for mutable tags + SQLite
> cache, cron/"check now", Updates view, comment-preserving tag rewrite, webhook.
>
> Progress note: **registry v2 client done + live-verified.**
> `internal/registry/ref.go` (`ParseImageRef` — docker.io/library defaulting,
> `lscr.io`→`ghcr.io` rewrite, host:port/localhost, tag/digest split) +
> `client.go`: generic WWW-Authenticate **Bearer token challenge** flow (works for
> Hub/ghcr/quay without per-registry code — only the ref normalization is
> registry-specific), `Digest` (HEAD manifest → `Docker-Content-Digest`, OCI+Docker
> Accept types), `Tags` (Link-header pagination), per-host concurrency semaphore
> (limit 2), and exponential backoff honoring `Retry-After` on 429/5xx. Unit-tested
> offline via a mock RoundTripper (token flow, pagination, 429 retry, concurrency
> cap); a REGISTRY_LIVE=1 test hit **real Docker Hub**: resolved whoami's digest,
> listed 95 tags, and the semver engine picked `v1.11.0` (minor) for `v1.10.0`.
> ghcr/quay use the same generic flow (not yet live-tested).
>
> Progress note: **update checker + digest path + SQLite cache done + live-verified.**
> `internal/updates/checker.go` — `Checker.CheckImage`: version-like tag → semver
> path (`Tags`+`Candidate`), mutable/absent tag → digest path (registry `Digest` vs
> local `docker.ImageRepoDigest`); `CheckAll` runs a bounded worker pool. Results
> (`updates.Result`, kinds semver|digest|uptodate|error|unsupported) cached in
> `image_checks` (migration `0003_updates.sql`, `store.SaveImageChecks`/
> `ImageChecks`). REST (in the auth+CSRF group): `GET /api/updates` (managed-stack
> images joined with cache + `usedBy`, kind "unchecked" until first check) and
> `POST /api/updates/check` (background run, `atomic.Bool` guard → 409 on
> concurrent, `2m` timeout, broadcasts `updates:changed`). Live-verified against
> real registries: whoami `v1.10.0`→`v1.11.0` (minor) semver, `alpine:latest`
> digest path uptodate (local RepoDigest == remote), concurrent-check 409. Image
> 153 MB.
>
> Progress note: **Updates view done.** `web/src/views/Updates.tsx` — grouped
> table (update available / up to date / not-checked), semver diff color-coding
> (major=red, minor=amber, patch=sky), `current → candidate` display, per-row
> expand showing used-by (stack/service) + current/latest digests + checked-at, and
> a **Check now** button (bulk check → `POST /api/updates/check`, 409-tolerant,
> clears on `updates:changed`). `useLiveUpdates` invalidates `["updates"]` and
> fires a `hivedock:updates` DOM event on `updates:changed`; App has an **Updates**
> nav item with an available-count badge (shared query cache). api.ts:
> `fetchUpdates`/`checkUpdates` + `UpdateEntry`. Vitest covers render + check-now.
> Live-verified: served SPA + real semver update surfaced.
>
> Progress note: **comment-preserving tag rewrite done** (the risk-register trust
> item). `internal/compose/rewrite.go` — `SetImageTag(content, service, newTag)`:
> locates `services.<svc>.image` via a read-only yaml.v3 parse (robust structural
> addressing) but edits the **raw bytes on that line only**, never re-serializing —
> comments, quoting (single/double), anchors/aliases, indentation, and CRLF are
> byte-preserved. Refuses env-interpolated (`${TAG}`→`ErrEnvManaged`, surfaced not
> rewritten) and digest-pinned (`@sha256`→`ErrDigestPinned`); handles untagged
> images and registry host:port; no-op when the tag already matches. Golden
> byte-exact table tests (7 rewrite cases + env/digest/not-found/no-change), all
> pass.
>
> Progress note: **Phase 4 is COMPLETE** — update&redeploy, changelog link,
> webhook, and cron scheduler all done + live-verified. `POST /api/stacks/{name}/
> services/{service}/update` (auth+CSRF) does `SetImageTag`+`atomicWrite` (409 on
> env-managed/digest-pinned); the Updates view's **Update & redeploy** button
> rewrites each `usedBy` usage then `compose up`s each affected stack, and Stacks
> rows show an "update" badge. Changelog: `docker.ImageSource` reads
> `org.opencontainers.image.source` (best-effort, needs image local), threaded
> through the checker/`image_checks.source` (migration `0004`)/DTO to a row link.
> Webhook: `WEBHOOK_URL` env → JSON POST of **newly-found** updates only (diffed vs
> prior cache, so no re-fire). Cron: `CHECK_INTERVAL` env (default 6h, 0 disables)
> → `New(ctx,…)` starts a background scheduler (initial check ~30s after boot).
> Live-verified end-to-end: check → webhook payload delivered → update&redeploy
> rewrote the tag (comment preserved) and the container came up on `v1.11.0` →
> re-check shows uptodate and the webhook did NOT re-fire → scheduler's initial
> check ran. Image 153 MB; all Go+vitest suites green. **Phase 4 exit still gated
> on PCT 102 verification** (the standing blocker); next is Phase 5 (polish).

## Phase 5 — Polish & release (2 sessions)

- [x] Mobile: Home as single column, Stacks read-mostly
- [~] Dark/light theme, loading/error states pass — **loading/error/empty states done** (all views); **light theme deferred** (dark-only for v1; see note)
- [x] Settings page (check interval, webhook, stacks dir display)
- [x] README with 60-second install (Dockge-style copy-paste compose.yaml), screenshots, "migrating from Dockge/Homepage/WUD" section — screenshots TODO (need a hosted instance)
- [x] GitHub Actions: test, lint, multi-arch image build (amd64/arm64) to ghcr
- [ ] Dogfood on NanoHive (PCT 102, real `/opt/stacks` after the Phase 3 gate) for two weeks before announcing; fold `docs/TESTING-NOTES.md` LXC findings into the README's Proxmox troubleshooting section

Exit: a stranger can go from README to working dashboard in under 5 minutes.

> Progress note: **Phase 5 mostly done.** Settings page (`GET/PUT /api/settings`,
> `internal/server/settings.go` + `store.{Get,Set,Delete}Setting`): webhook URL
> editable in-app (settings-table override wins over `WEBHOOK_URL` env; empty
> clears it), stacks/data dir + check interval + auth + version shown read-only;
> SPA `views/Settings.tsx` + nav item. Mobile: `App.tsx` layout is now
> `flex-col md:flex-row` — the sidebar becomes a horizontal scrollable top bar on
> small screens (sign-out moves into the header); Home/Stacks/Updates grids were
> already responsive. Loading/error/empty states verified present across all
> views. README rewritten as a release doc (60-sec copy-paste compose.yaml on
> `ghcr.io/rogalinski/hivedock`, env table incl. CHECK_INTERVAL/WEBHOOK_URL/
> PUBLIC_HOST, Dockge/Homepage/WUD migration section, reverse-proxy notes).
> CI: `.github/workflows/ci.yml` (go vet+test, web tsc+vitest+eslint, amd64 image
> build) and `release.yml` (buildx multi-arch amd64+arm64 → GHCR: `:edge` on main,
> `:X.Y.Z`/`:X.Y`/`:latest` on tags). Fixed 4 eslint issues to satisfy CI's
> `--max-warnings 0`. Image 153 MB; all suites green; settings GET/PUT live-verified.
>
> **Deferred (deliberate):** a full **light theme** — the app is dark-by-default
> and a proper light mode is a broad, regression-prone reclassification of every
> component's zinc palette; better as a dedicated pass than rushed here. Homelab
> dashboards are conventionally dark, so dark-only is a defensible v1. **Screenshots**
> in the README await a hosted instance. **Dogfood on PCT 102** is the remaining
> real-world gate (blocked on `HIVEDOCK_DEPLOY_HOST`).

## Post-v1 status (2026-07-11)

> **Shipped and deployed.** The repo is public at `github.com/rodzalendo/hivedock`;
> CI + Release publish `ghcr.io/rodzalendo/hivedock` (`:edge` on main,
> `:X.Y.Z`/`:latest` on tags). Release train so far: **v0.1.0 → v0.1.4**.
> Running in production side-by-side with Dockge on the user's Proxmox LXC
> (PCT 101, HiveDock on `:5002`, same real `/opt/stacks`) — this replaced the
> planned PCT 102 gate. Several feedback rounds landed (detail in git history):
> stale-tag build-date guard in the update engine (the qbittorrent `20.04.1`
> false positive, regression-tested), per-image update ignore, live-editable
> check interval (default 30m), user-defined home groups with drag & drop +
> tile size slider + card rename/icon overrides, hidden-sidecar reveal, stack
> rename/delete, force-recreate for cross-tool drift, prune + disk usage,
> security headers + login damping, branding as **HiveDock**. Remaining:
> README screenshots (drop into docs/screenshots/, then uncomment the image
> lines) and the parking lot below.

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
