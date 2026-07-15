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

1. **`ghcr.io`** — to check whether a newer *HiveDock* release exists (the sidebar version check and self-update), and to fetch that release's cosign signature so it can be verified. Skipped entirely for `dev`/`edge` builds, and for the `off` / `check-only` update modes as documented. Signature verification uses a sigstore trust root baked into the image and verifies the transparency-log proof offline, so it needs no call to Rekor; cosign may occasionally refresh an expired trust root from the sigstore TUF CDN (`tuf-repo-cdn.sigstore.dev`).
2. **Container registries you already use** — Docker Hub, GHCR, LinuxServer (lscr.io), Quay — to check your images for updates. Only the registries your stacks reference are contacted, and only for tag/digest metadata (HEAD requests where possible).
3. **Icon CDNs** (dashboard-icons, selfh.st) — once per newly seen image to fetch its icon, which is then cached under `DATA_DIR` and served locally. Existing installs re-fetch nothing.

Nothing else leaves the box. If you find a connection not on this list, that's a bug — please report it.

## Trust model, stated plainly

- **The Docker socket is root-equivalent.** HiveDock mounts `/var/run/docker.sock` to read state and run `docker compose`. Anyone who can drive HiveDock (or compromises it) can do anything Docker can do on that host — which is effectively root. Treat access to HiveDock as access to the host.
- **Deploy it behind your network boundary.** HiveDock is designed for a LAN or VPN. Do **not** expose it directly to the internet. If you need remote access, put it behind a reverse proxy that terminates TLS and enforces its own authentication (Authelia/authentik/Caddy forward-auth, a VPN, or your SSO).
- **`.env` files are readable and editable** in the stack editor, as plaintext. That is a deliberate, documented decision — HiveDock manages your compose stacks, and their `.env` is part of that. Secrets in those files are protected by filesystem permissions on `DATA_DIR`/`STACKS_DIR`, not by encryption at rest.

## Current protections

- **Single-admin authentication** (bcrypt), HttpOnly session cookie, double-submit CSRF token on every mutating request. First-run creates the admin — there are no default credentials, and setup is gated by a one-time token printed to the container log so an unclaimed instance can't be seized by whoever reaches it first.
- **Login brute-force damping** — failed logins are rate-limited per (username, IP) with exponential backoff (5 failures → 30 s, doubling to a 15 min cap), on top of a flat per-attempt delay and bcrypt's own cost. The unknown-username path still runs a bcrypt comparison, so timing doesn't reveal whether the account exists.
- **Session hardening** — tokens are stored only as a SHA-256 hash (a DB read can't recover a usable cookie), rotate on each login, and expire after 7 days idle / 30 days absolute.
- **Trusted-header (forward-auth) SSO** — optional; trust is decided by the real TCP peer against configured CIDRs, evaluated before any `X-Forwarded-For` rewriting, so the header can't be spoofed from outside the proxy network.
- **Verified, digest-pinned self-update.** Release images are cosign-signed keyless via GitHub Actions OIDC (no long-lived signing key). Before HiveDock offers an update to itself it verifies the candidate's signature against its own release-workflow identity; if verification fails it shows an alert instead of an offer. Applying pulls the *exact verified digest* (never a tag that may have moved) and recreates the container from those bytes. A stricter-than-running downgrade guard means a stale-but-signed release can't walk an install backward. Three modes: `full` (verify + one-click apply, default), `check-only`, `off`.
- **Reads via the Docker SDK; mutations shell out to `docker compose`** with argument arrays (no shell string interpolation — enforced by a CI grep gate over non-test Go), each under a per-stack lock.
- **File trust.** Compose files are the source of truth and are only ever written on an explicit editor save or the image-tag rewrite in the update flow. That rewrite is a targeted scalar edit whose byte-exactness is *verified* — the result must be the original with only the tag span changed, or the write aborts (fuzz-tested). Editor saves are optimistic-locked: if a file changed on disk since you opened it, the save is refused with a 409 and the two versions to reconcile, so an out-of-band edit (e.g. over SSH) is never silently clobbered. Machine edits show a unified diff for confirmation before anything is written. Env-interpolated tags are surfaced, never rewritten.
- **Path containment.** Every file operation resolves symlinks and must land inside `STACKS_DIR`; a compose/`.env` that symlinks out of the tree is refused. Stack names are a strict lowercase allowlist (`^[a-z0-9][a-z0-9_-]{0,63}$`), so a name can't spell a path escape.
- **Optional local audit trail** — a per-change git history of `STACKS_DIR` (HiveDock's writes and out-of-band ones alike), off by default, local only (no remotes).
- **Registry hardening** — per-registry credentials (private registries) and TLS trust (custom CA bundle or an insecure toggle for self-signed homelab registries), scoped per host; registries you don't configure stay anonymous with strict TLS. Scheduled update sweeps jitter their start and back off on `429`/`Retry-After`.
- **Startup self-check** — HiveDock verifies its own `STACKS_DIR` bind resolves to the same path inside and outside the container; a mismatch (which would misdirect compose's relative paths) drops it into **read-only mode** with a banner instead of corrupting stacks. Podman and rootless Docker are detected and flagged as unsupported.
- **Read-only API token** — an optional, revocable bearer token for monitoring tools, stored hashed and valid only for `GET /api/health`, `/api/stacks`, `/api/updates` — never mutations or settings.
- **Same-origin WebSocket** — the `/api/ws` upgrade is rejected unless the `Origin` matches the request host or the configured `PUBLIC_HOST`, closing cross-site WebSocket hijacking.
- **Log output is sanitized server-side** — terminal escape sequences and stray control bytes are stripped from container log lines before they reach the browser, and all dynamic strings render as text nodes (no `innerHTML`, enforced in CI).
- **Strict response headers** — a Content-Security-Policy with **zero external origins** (`default-src 'self'`, `img-src 'self' data:` — even custom icon URLs are proxied server-side, SSRF-guarded), plus `nosniff`, `frame-ancestors 'none'`, and a restrictive `Referrer-Policy` on every response.

## Known limitations, stated plainly

Being explicit beats being discovered. The full picture — assets, trust boundaries, and residual risks — is in [`THREAT_MODEL.md`](THREAT_MODEL.md). In short:

- **Socket access is root-equivalent.** No in-app control changes this while the socket is reachable; `deploy/compose.hardened.yaml` shows how to reduce surface (cap-drop, read-only rootfs, socket proxy).
- **No at-rest encryption** for stored settings or `.env` beyond filesystem permissions.
- **Release-pipeline trust.** Verified self-update moves the trust root from a registry password to HiveDock's GitHub Actions release workflow (keyless OIDC). A compromise of that workflow could produce a validly-signed malicious image; the tag protection, required-reviewer release environment, and SLSA provenance on each build exist to reduce that risk. SSH remains the universal fallback.

> **`AUTH_DISABLED` was removed.** It disabled authentication entirely and turned a socket-holding mutator into an open proxy. The container now refuses to boot if the variable is still set. Use trusted-header (forward-auth) SSO instead: set `AUTH_TRUSTED_HEADER` + `AUTH_TRUSTED_PROXY_CIDRS` behind Authelia/authentik/Caddy. The header is honored only when the request's real TCP peer is inside a configured CIDR (evaluated before `X-Forwarded-For` rewriting), so it cannot be spoofed from outside your proxy network.

If any of these blocks your use case, run behind a proxy that adds the missing control, and watch the releases.

## fail2ban

Failed logins log a fixed line you can ban on. HiveDock logs JSON; a filter (`/etc/fail2ban/filter.d/hivedock.conf`):

```ini
[Definition]
failregex = "msg":"auth: failed login".*"ip":"<HOST>"
datepattern = "time":"%%Y-%%m-%%dT%%H:%%M:%%S
```

Jail (`/etc/fail2ban/jail.d/hivedock.conf`), pointing at the container log (e.g. via `docker logs` shipped to a file, or journald):

```ini
[hivedock]
enabled  = true
filter   = hivedock
maxretry = 5
findtime = 15m
bantime  = 1h
```

HiveDock's own per-(user, IP) backoff already damps brute force; fail2ban adds a network-level ban when you want it.
