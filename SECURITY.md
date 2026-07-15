# Security Policy

HiveDock is a single-host Docker management tool that holds the Docker socket. That makes it a high-value target and honesty about its posture matters more than a long feature list. This document says what it does over the network, how to report a problem, and what is and isn't guaranteed today.

## Reporting a vulnerability

Please report privately, not in a public issue:

- **GitHub Security Advisories** — [open a private report](https://github.com/rodzalendo/hivedock/security/advisories/new) on the repository. This is the preferred channel.

Include a description, affected version (see the sidebar/`/api/health`), and a reproduction if you have one. We aim to acknowledge within a few days and to fix confirmed issues in a timely release, credited unless you prefer otherwise. There is no bug-bounty program.

## Supported versions

HiveDock is pre-1.0 and ships from a single line. Security fixes land on the latest release (`:latest` / newest `vX.Y.Z` tag). Run a release build, not `:edge`, on anything you care about, and keep it current — the sidebar tells you when a newer release exists.

## What HiveDock connects to (the "does it phone home" answer)

HiveDock makes **no** analytics, telemetry, or crash-reporting calls — ever. The only outbound connections are:

1. **`ghcr.io`** — to check whether a newer *HiveDock* release exists (the sidebar version check and self-update). Skipped entirely for `dev`/`edge` builds; a future setting will let you turn it off.
2. **Container registries you already use** — Docker Hub, GHCR, LinuxServer (lscr.io), Quay — to check your images for updates. Only the registries your stacks reference are contacted, and only for tag/digest metadata (HEAD requests where possible).
3. **Icon CDNs** (dashboard-icons, selfh.st) — once per newly seen image to fetch its icon, which is then cached under `DATA_DIR` and served locally. Existing installs re-fetch nothing.

Nothing else leaves the box. If you find a connection not on this list, that's a bug — please report it.

## Trust model, stated plainly

- **The Docker socket is root-equivalent.** HiveDock mounts `/var/run/docker.sock` to read state and run `docker compose`. Anyone who can drive HiveDock (or compromises it) can do anything Docker can do on that host — which is effectively root. Treat access to HiveDock as access to the host.
- **Deploy it behind your network boundary.** HiveDock is designed for a LAN or VPN. Do **not** expose it directly to the internet. If you need remote access, put it behind a reverse proxy that terminates TLS and enforces its own authentication (Authelia/authentik/Caddy forward-auth, a VPN, or your SSO).
- **`.env` files are readable and editable** in the stack editor, as plaintext. That is a deliberate, documented decision — HiveDock manages your compose stacks, and their `.env` is part of that. Secrets in those files are protected by filesystem permissions on `DATA_DIR`/`STACKS_DIR`, not by encryption at rest.

## Current protections

- **Single-admin authentication** (bcrypt), HttpOnly session cookie, double-submit CSRF token on every mutating request, and a flat delay on failed logins to damp brute force. First-run creates the admin; there are no default credentials.
- **Reads via the Docker SDK; mutations shell out to `docker compose`** with argument arrays (no shell string interpolation), each under a per-stack lock.
- **Compose files are only ever written** on an explicit editor save or the single-line image-tag rewrite in the update flow (a targeted scalar edit — never a parse-and-dump). Env-interpolated tags are surfaced, never rewritten.
- **Same-origin WebSocket** — the `/api/ws` upgrade is rejected unless the `Origin` matches the request host or the configured `PUBLIC_HOST`, closing cross-site WebSocket hijacking.
- **Log output is sanitized server-side** — terminal escape sequences and stray control bytes are stripped from container log lines before they reach the browser, and all dynamic strings render as text nodes (no `innerHTML`, enforced in CI).
- **Baseline response headers** — a Content-Security-Policy, `nosniff`, `frame-ancestors 'none'`, and a restrictive `Referrer-Policy` on every response.

## Known limitations being addressed

Being explicit beats being discovered. On the current roadmap (see `docs/HARDENING.md`):

- **Self-update is not yet signature-verified.** It pulls the newest release image and recreates HiveDock's own container; cosign verification and digest-pinning of that flow are planned.
- **No at-rest encryption** for stored settings beyond filesystem permissions.
- **First-run setup is not yet token-gated** — on a fresh install, whoever reaches the setup screen first becomes admin. Complete setup promptly on a trusted network; a one-time log-printed setup token is planned.

> **`AUTH_DISABLED` was removed.** It disabled authentication entirely and turned a socket-holding mutator into an open proxy. The container now refuses to boot if the variable is still set. Use trusted-header (forward-auth) SSO instead: set `AUTH_TRUSTED_HEADER` + `AUTH_TRUSTED_PROXY_CIDRS` behind Authelia/authentik/Caddy. The header is honored only when the request's real TCP peer is inside a configured CIDR (evaluated before `X-Forwarded-For` rewriting), so it cannot be spoofed from outside your proxy network.

If any of these blocks your use case, run behind a proxy that adds the missing control, and watch the releases.
