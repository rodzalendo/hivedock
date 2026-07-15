# HiveDock — Threat Model

This document states, plainly, what HiveDock protects, what it does not, and what
an attacker gets if they succeed. It is deliberately blunt: a tool that holds the
Docker socket owes its users honesty about what that means. Mitigations referenced
here are implemented (see `SECURITY.md` and `docs/HARDENING.md`); residual risks
are named, not hidden.

## Assets

1. **The Docker socket.** The crown jewel. Read/write access to
   `/var/run/docker.sock` is root-equivalent on the host (see below).
2. **Compose files and `.env` under `STACKS_DIR`.** The source of truth for every
   stack. `.env` files routinely hold secrets (DB passwords, API keys).
3. **The admin session and credentials.** A valid session can drive every
   mutating operation, i.e. anything the socket can do.
4. **HiveDock's own release pipeline.** Signed releases are the trust root for the
   self-update feature; compromising the pipeline compromises every updater.

## Trust boundaries

### Browser ↔ app

The SPA talks to the API over same-origin HTTP/WebSocket.

- **Authentication:** single admin, bcrypt password, HttpOnly session cookie.
  No default credentials; first-run is gated by a one-time token printed to the
  container log. Optional trusted-header (forward-auth) SSO, honored only when the
  request's real TCP peer is inside a configured proxy CIDR (evaluated *before*
  `X-Forwarded-For` rewriting, so it cannot be spoofed from outside the proxy
  network). There is no auth-bypass switch.
- **CSRF:** double-submit cookie token on every mutating request.
- **Cross-site WebSocket hijacking:** the `/api/ws` upgrade requires the `Origin`
  to match the request host (or `PUBLIC_HOST`).
- **Injection into the UI:** container log lines and label values are attacker-
  influenced; they are sanitized server-side (control/escape sequences stripped)
  and rendered only as React text nodes — never `innerHTML` (CI-enforced). The CSP
  allows zero external origins (`default-src 'self'`, `img-src 'self' data:`); even
  custom icon URLs are proxied server-side, so a stored string can't exfiltrate via
  an image request.
- **Brute force:** login/setup are rate-limited per (username, IP) with
  exponential backoff; the unknown-user path still runs bcrypt so timing doesn't
  leak account existence. Session tokens are stored only as SHA-256 hashes.

### App ↔ Docker socket

**This boundary is the whole risk.** HiveDock mounts the socket to read state and
run `docker compose`. **Socket access is root-equivalent:** anyone who can drive
HiveDock — or who compromises the HiveDock process — can start a container that
bind-mounts the host filesystem and thereby do anything root can do on the host.
No in-app permission model changes this. Mitigations reduce *reachability* of the
socket, not its power: reads go through the Docker SDK; mutations shell out to
`docker compose` with argument arrays only (no shell interpolation, CI-enforced);
file paths are symlink-resolved and confined to `STACKS_DIR`; stack names are a
strict allowlist. For read-mostly deployments, a socket-proxy allowlist
(`deploy/compose.hardened.yaml`) narrows the surface — with the honest caveat that
mutations require POST, so a proxy reduces surface without removing
root-equivalence.

### App ↔ registries

HiveDock queries registries for image metadata (tags, digests) and pulls images
during updates. It does **not** send credentials it wasn't given, and the only
outbound connections are enumerated in `SECURITY.md` (ghcr for HiveDock's own
version check + signature, the registries your stacks use, icon CDNs once per new
image). Registry TLS is verified. There is no telemetry.

### Release pipeline (self-update trust root)

Release images are cosign-signed keyless via GitHub Actions OIDC — no long-lived
signing key exists to steal. Before offering an update to itself, HiveDock verifies
the candidate's signature against its own release-workflow identity (baked into the
binary) and applies only the exact verified digest, refusing downgrades. A failed
verification surfaces an alert, never a silent update.

## Deployment assumptions

- **HiveDock runs on a LAN or VPN**, behind the user's network boundary. It is not
  designed to face the public internet directly.
- **WAN exposure only behind a reverse proxy** that terminates TLS and enforces its
  own authentication (Authelia/authentik/Caddy forward-auth, or a VPN).
- The host and the `DATA_DIR`/`STACKS_DIR` filesystem permissions are trusted; the
  admin is trusted (they already have socket-equivalent power by design).
- The Docker daemon and the images the user chooses to run are outside HiveDock's
  trust boundary — HiveDock manages them, it does not vet them.

## What an attacker gets

- **Compromising HiveDock (RCE, or a stolen admin session on an exposed instance):**
  full control of the Docker host — root-equivalent. Treat access to HiveDock as
  access to the host. This is inherent to the category (Portainer, Dockge, and
  every socket-mounting manager share it).
- **A malicious/compromised container on the host:** can feed attacker-controlled
  strings into log views and labels — contained by output sanitization and
  text-node rendering, so the blast radius is display, not code execution in the
  browser.
- **A man-in-the-middle on a registry:** cannot substitute a HiveDock self-update
  (signature + digest verification); can affect *your* image update checks the same
  way it could affect a plain `docker pull` (registry TLS is the mitigation).

## Residual risks (named honestly)

- **Release-pipeline compromise.** Verified self-update relocates the trust root
  from a registry password to HiveDock's GitHub Actions release workflow. An
  attacker who fully compromised that workflow (protected tags, required-reviewer
  release environment, SHA-pinned actions are the defenses) could produce a
  validly-signed malicious image. SSH remains the universal fallback to recover.
- **Secrets on disk.** `.env` files hold secrets as plaintext, protected by
  filesystem permissions on `DATA_DIR`/`STACKS_DIR`, not encryption at rest. This
  is a deliberate, documented decision.
- **`.env` in the editor.** The stack editor reads and writes `.env` as plaintext;
  masking is intentionally out of scope. Being explicit beats being discovered.
- **Root by default.** The container runs as root because the socket is typically
  root-owned. `deploy/compose.hardened.yaml` shows how to drop capabilities, set
  `no-new-privileges`, run read-only rootfs, and front the socket with a proxy —
  but none of that removes root-equivalence while the socket is reachable.
- **A trusted admin turning malicious** is out of scope: the admin already holds
  socket-equivalent power by design.

## Reporting

Security issues: please report privately via GitHub security advisories rather than
a public issue. See `SECURITY.md`.
