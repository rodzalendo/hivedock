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

## Deploy loop (dev machine → PCT 102)

No registry round-trip during development. `task deploy` does:

```
task build                                    # local multi-arch not needed; build for PCT arch (amd64)
docker save hivedock:dev | ssh root@<pct-ip> docker load
ssh root@<pct-ip> "cd /opt/hivedock && docker compose up -d"
```

Set `HIVEDOCK_DEPLOY_HOST` in `.env.local` for the Taskfile. First deploy: copy `deploy/pct102.compose.yaml` to `/opt/hivedock/compose.yaml` on the container:

```yaml
services:
  hivedock:
    image: hivedock:dev
    restart: unless-stopped
    ports:
      - 5001:5001
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data
      - /opt/stacks-test:/opt/stacks-test   # same path inside and out — required
    environment:
      - STACKS_DIR=/opt/stacks-test
      - AUTH_DISABLED=true                  # test env only; behind LAN
```

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
