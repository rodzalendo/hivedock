# Hivedock — Product Requirements

*What* Hivedock is and *why*. For *how*, see `docs/ARCHITECTURE.md`; for order of
work, `docs/PLAN.md`. This consolidates decisions already encoded across the
repo — treat it as the scope contract, especially the non-goals.

## 1. Problem

Self-hosters running Docker Compose on a single box juggle three tools:

- **Dockge/Portainer** to manage stacks (edit compose, up/down, logs),
- **Homepage/Heimdall** to get a dashboard of what's running,
- **Watchtower/WUD** to know when images have updates.

Each needs its own config, its own labels, its own mental model. The dashboard
is hand-maintained YAML that drifts from reality; update tools either
auto-update dangerously or just notify without context.

## 2. What Hivedock is

One container that does all three, coherently, for **one host**:

1. **Stack management** (Dockge-equivalent): see every stack and container,
   edit compose files, deploy/stop/restart, stream logs.
2. **Zero-config homepage**: an auto-generated dashboard of your running apps —
   correct icons and clickable URLs with **no labels required** — that can never
   drift from reality because it *is* derived from reality.
3. **Update checking** (WUD-style): know which images have newer tags, with
   semver context, and update-and-redeploy in one click — never automatically.

The wedge is the homepage: because Hivedock already knows your stacks, ports,
and images, the dashboard is free and always accurate.

## 3. Users

- **Primary — the homelabber**: runs 5–30 compose stacks on a NAS/mini-PC/Proxmox
  LXC. Comfortable with SSH and YAML, wants a UI that respects the files, not one
  that hides or rewrites them. Values trust over features.
- **Secondary — the tidy self-hoster** migrating from a Dockge + Homepage + WUD
  stack who wants fewer moving parts and a migration path (honor existing
  `homepage.*` labels).

## 4. Principles

- **The files are the truth.** Hivedock reflects and edits compose files; it
  never becomes a second source of truth. You can stop using it and your stacks
  are untouched.
- **The UI never lies.** Things Hivedock didn't create are shown read-only, not
  hidden. Drift is surfaced. Failures show real stderr.
- **Zero-config first, labels to override.** Sensible automatic defaults for
  names/icons/URLs/grouping; labels only when you want to override.
- **Safe by default.** No auto-updates. No destructive action without intent.
  Auth before any mutation.
- **One host, done well.** Depth over breadth.

## 5. Success criteria

- From README to a working dashboard in **under 5 minutes** (Dockge-style
  copy-paste `compose.yaml`).
- Fixture/real stacks render on the homepage with **correct icons and clickable
  URLs and zero labels**.
- Editing a compose file over SSH shows as **drift** in the UI without a refresh;
  deploying from the UI is byte-faithful to the file (comments/order intact).
- Update suggestions are **trustworthy** — the semver engine is right on a real
  corpus, env-interpolated tags are flagged not rewritten, mutable tags use
  digests.
- Killing the Hivedock container leaves **every stack unaffected**.

## 6. Non-goals (the scope contract)

Adding any of these must displace something, not just accrete:

- **Multi-host / clustering / Kubernetes.** Single host only.
- **Automatic updates** (Watchtower-style unattended). Hivedock checks and
  offers; the human decides. (Safe batching is a *post-v1* research item.)
- **Homepage service widgets** (per-service API integrations, weather, etc.).
  Cards link out; they don't embed dashboards.
- **A compose reimplementation.** All lifecycle ops shell out to
  `docker compose`.
- **Becoming Portainer** — image/network/volume/registry management surfaces,
  RBAC, teams. Single admin only.
- **Storing stack definitions in a database.** Files are the truth.

## 7. Post-v1 parking lot (not commitments)

`docker run → compose` converter · multiple webhook targets · update
batching/schedules ("apply all patch updates Sunday 3am") · read-only public
dashboard mode · Gotify/ntfy native · `homepage.* → hivedock.*` migration helper.
