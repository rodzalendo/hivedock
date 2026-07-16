# Hivedock — Test Deployment (NanoHive, PCT 102)

Primary test environment: Proxmox host **NanoHive**, LXC container **PCT 102** running Docker. Every phase's exit criteria get verified here, not just against local fixtures.

## LXC prerequisites (one-time)

Docker-in-LXC works but has known sharp edges; check these before blaming Hivedock for anything.

Container config (`/etc/pve/lxc/102.conf` on the Proxmox host):

```
features: nesting=1,keyctl=1
```

- `nesting=1` is required for Docker at all; `keyctl=1` is required for unprivileged containers
- Unprivileged is preferred; only fall back to privileged if something specific breaks, and note what
- **Storage driver check:** run `docker info | grep "Storage Driver"` inside the container. Want `overlay2`. If the LXC rootfs is on ZFS, overlay2 needs ZFS 2.2+ (Proxmox 8 default); older setups silently fall back to `vfs`, which is brutally slow and disk-hungry — fix that before testing, or Hivedock will look slower than it is
- Give the container a static IP or DHCP reservation; the homepage URL heuristic builds links from the host IP, and a changing IP makes every card link rot

Record here once verified:

| | value |
|---|---|
| PCT 102 IP | `<fill in>` |
| Privileged/unprivileged | `<fill in>` |
| Storage driver | `<fill in>` |
| Docker / compose version | `<fill in>` (compose must be v2.20+) |

## Directory layout on PCT 102

```
/opt/hivedock/            hivedock's own compose.yaml + ./data
/opt/stacks-test/         test stacks dir — Hivedock points HERE initially
/opt/stacks/              real stacks (if any) — only after Phase 3 exit criteria pass
```

Rule: Hivedock runs against `/opt/stacks-test` until it has proven, on this box, that (a) read-only views never mutate anything and (b) every mutation is scoped to the stack it targets. Flipping `STACKS_DIR` to the real directory is a deliberate, logged decision at the end of Phase 3, not a default.

## Two tracks: staging (local) vs. deploy (server pulls from GHCR)

Development and deployment are decoupled through GitHub + GHCR — no `docker save`/`ssh load` round-trip.

- **Staging = this machine (Docker Desktop).** Iterate with `task up` (or `docker compose up --build`), which builds `hivedock:dev` and serves `:5001` against `./dev-stacks`. This is the throwaway "working version" — try changes here before committing.
- **Deploy = server pulls a published image.** Push to `main` → GitHub Actions builds a multi-arch image and publishes `ghcr.io/rodzalendo/hivedock:edge`. Tagging `vX.Y.Z` additionally publishes `:X.Y.Z`, `:X.Y`, and `:latest`. The server never builds — it only pulls.

### One-time: make the GHCR package pullable

GHCR packages start **private**. After the first Release workflow succeeds, either:

- make it public — GitHub → repo → *Packages* → `hivedock` → *Package settings* → *Change visibility* → Public; **or**
- keep it private and `docker login ghcr.io -u rodzalendo` on the server with a read-scoped PAT.

### First deploy on PCT 102

```bash
ssh root@<pct-ip>
mkdir -p /opt/hivedock && cd /opt/hivedock
# copy deploy/pct102.compose.yaml from this repo to ./compose.yaml (scp, or paste)
docker compose pull
docker compose up -d
```

`deploy/pct102.compose.yaml` already references `ghcr.io/rodzalendo/hivedock:edge`, mounts the socket, `./data`, and `/opt/stacks-test`. Auth is on — complete first-run setup at the login screen (the one-time token is printed to the container log).

### Subsequent updates

```bash
ssh root@<pct-ip> "cd /opt/hivedock && docker compose pull && docker compose up -d"
```

Or set `HIVEDOCK_DEPLOY_HOST` in `.env.local` and run `task deploy` (which does exactly the above over SSH). For pinned releases, deploy a tag instead of `:edge` by editing the `image:` line to `:X.Y.Z`.

## Test stacks to seed on PCT 102

Seed `/opt/stacks-test` with stacks chosen to exercise each subsystem (a `deploy/seed-test-stacks.sh` script should create these):

1. `whoami/` — trivial, one published port → URL heuristic baseline
2. `jellyfin/` — pinned old tag (e.g. `10.10.3`) → semver update path, icon match
3. `uptime-kuma/` — `:1` major-pin tag → part-count preservation test
4. `nginx-latest/` — `latest` tag → digest update path
5. `app-with-db/` — web app + postgres → sidecar auto-hide heuristic
6. `homepage-labeled/` — any service with `homepage.name/icon/group` labels → migration fallback
7. `env-tag/` — `image: redis:${REDIS_TAG}` with `.env` → must be surfaced as env-managed, never rewritten

Plus, at Phase 1: one stack deployed manually via SSH (`docker compose up -d`) that Hivedock never created → must appear correctly. And one container started with plain `docker run` → must show as external, read-only.

## What to log while testing

Keep a running `docs/TESTING-NOTES.md` (gitignored or not, your call) with: date, image build, what broke, whether it was Hivedock, Docker-in-LXC, or the network. LXC quirks especially — they'll become the README's troubleshooting section, because every future user on Proxmox hits the same ones.

## Known Docker-in-LXC caveats (so they don't get misattributed)

- Proxmox officially recommends Docker in a VM; LXC works and is common in homelabs, but kernel updates on the host can occasionally break nesting until container restart
- `docker logs` streaming and events API work normally under nesting — if they don't, check AppArmor profile on the host
- Host stats inside the container report the LXC's cgroup limits, not the Proxmox host's totals — expected, not a bug; the stats strip shows what the container can see
- If bind-mounting from Proxmox host storage into the LXC and then into Docker (double bind), inotify events may not propagate — the periodic rescan (not just fsnotify) exists for exactly this case; verify drift detection works with a 30s worst-case delay
