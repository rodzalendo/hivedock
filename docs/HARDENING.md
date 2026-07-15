# Hivedock — Security Hardening: Design & Implementation Plan

What changes before the public launch post, and in what order. `docs/ARCHITECTURE.md`
stays the design of record for shipped systems; the sections below amend it as they
merge (update both together, per house rules). `docs/PRD.md` non-goals still bind.

> **Status:** approved design, implementation in progress. Two items fix real
> vulnerabilities (unverified self-update, `AUTH_DISABLED`). The rest exist so
> every predictable audit question has a written answer and a test behind it
> before strangers read the repo.
>
> **Landed (v0.3.2):** §4.4 WS same-origin, §4.6 log-escape sanitization + the
> `dangerouslySetInnerHTML` CI gate, `SECURITY.md` + README security section.
>
> **Landed (v0.4.0, Phase A core):** §2.1 `AUTH_DISABLED` removed with boot
> refusal; §2.2 trusted-header forward-auth (peer-CIDR gated, real TCP peer via
> `capturePeer` before `X-Forwarded-For`, CSRF skipped on the header path); test
> harness migrated onto it. Spoof-rejection tested.
>
> **Landed (v0.4.1, Phase A complete):** §2.3 first-run setup token (logged, gates
> setup, cleared on completion) + `ADMIN_USER`/`ADMIN_PASSWORD_FILE` bootstrap;
> §2.4 login rate limiting per (user, ip) with exponential backoff, SHA-256
> session-token storage, 7d idle / 30d absolute expiry, rotation on login, and a
> fail2ban filter in `SECURITY.md`. Migration 0008 clears old sessions (re-login
> once). Session-invalidate-on-password-change is wired (`DeleteAllSessions`) but
> there is no password-change endpoint yet. Remaining Phase-A polish: copy-paste
> proxy snippets and a "disable password login" toggle (§2.2/§2.4 nice-to-haves).
>
> **Landed (Phase B — supply chain):** §3 complete. Releases are cosign-signed
> keyless via GitHub Actions OIDC with SLSA provenance + SBOM, multi-arch
> (amd64+arm64), gated on a `release` environment. The in-app check resolves the
> candidate's digest and cosign-verifies it (bundled cosign binary, offline
> against a seeded sigstore trust root — outbound stays ghcr-only) before
> offering it; a failed verify surfaces an alert, never an offer. The apply path
> pulls the exact verified digest and recreates via `up -d --pull never` from a
> `hivedock apply-update` helper subcommand — which retired the last `sh -c`, so
> the §4.3 no-shell CI gate landed too. Update modes `full`/`check-only`/`off`.
>
> **Not yet done:** §4.1, §4.2, §4.5, §4.7, §4.8, §5, §6, and `THREAT_MODEL.md`.

## 1. Scope

In: auth model rework, supply-chain verification for self-update, an input/output
hardening pass, file-trust guarantees, registry resilience, repo hygiene, and the
disclosure documents (`SECURITY.md`, `THREAT_MODEL.md`, README security section).

Out, unchanged from PRD non-goals: multi-user/RBAC, built-in TOTP (forward auth is
the answer), webhook notifications (stays removed), secret encryption at rest beyond
file permissions, `.env` masking in the editor (parking lot). Multi-file stack
support (`compose.override.yaml`, `include:`) is a functional roadmap item and not
part of this plan.

## 2. Auth

### 2.1 `AUTH_DISABLED` is removed — ✅ shipped (v0.4.0)

The env var turned a socket-holding mutator into an unauthenticated proxy for
anyone who could reach the port. No upside proportional to that. Deleted, not
deprecated.

Boot behavior when the var is present: log
`AUTH_DISABLED was removed in vX.Y. Use trusted-header auth for SSO (docs/auth.md) or complete first-run setup.`
and exit nonzero. Rationale for failing instead of ignoring: users who set this
never created credentials, so silently enabling auth strands them at a login they
don't have; and a security switch that stops working should be impossible to miss.
An explicit failure gets read. A silent behavior change gets a confused issue.

### 2.2 Trusted-header forward auth (the SSO replacement) — ✅ shipped (v0.4.0)

For the legitimate `AUTH_DISABLED` use case: Authelia/authentik/Caddy in front,
no second login. Implemented as `AUTH_TRUSTED_HEADER` + `AUTH_TRUSTED_PROXY_CIDRS`;
the CIDR test uses the real TCP peer captured by `capturePeer` (before RealIP's
`X-Forwarded-For` rewrite). Copy-paste proxy snippets and the "disable password
login" toggle are still to write.

| Env | Meaning |
|---|---|
| `AUTH_TRUSTED_HEADER` | Header name carrying the authenticated user, e.g. `Remote-User`. Empty (default) disables the feature entirely. |
| `AUTH_TRUSTED_PROXY_CIDRS` | Comma-separated CIDRs. The header is honored only when the direct TCP peer is inside one of these. |

Semantics: both set, peer inside a listed CIDR, header present → request is
authenticated as admin. The header value is logged for audit but not used for
lookup (single-admin model). Peer outside the CIDRs → the header is stripped and
ignored and normal login applies. The CIDR test uses the TCP peer address only,
never `X-Forwarded-For`. A setting allows disabling password login once header
auth is confirmed working. Docs ship copy-paste snippets for Authelia, authentik,
and Caddy `forward_auth`.

### 2.3 First-run claim protection — ✅ shipped (v0.4.1)

No default credentials, ever. On first boot with no admin in `settings`, the app
serves a setup flow gated by a one-time token printed to the container log
(`docker logs hivedock` → token). This closes both the default-password headline
and the unclaimed-instance race where whoever finds a fresh install first becomes
its admin. The token regenerates on every boot until setup completes, and the
setup endpoint is rate limited like login.

Automation path for CI and scripted installs: `ADMIN_USER` +
`ADMIN_PASSWORD_FILE`, consumed only when no admin exists yet and ignored ever
after. This also replaces the dev-workflow use of `AUTH_DISABLED`.

### 2.4 Sessions and login — ✅ shipped (v0.4.1)

- Rate limiting keyed on (username, IP): five failures start a backoff at 30 s,
  doubling to a 15 min cap. The failure log line is fixed-format,
  `hivedock auth: failed login user=%q ip=%s`, with a ready fail2ban filter in
  the docs.
- The unknown-username path burns a bcrypt comparison too, so timing doesn't
  reveal whether the account exists.
- Session tokens are stored as SHA-256 hashes, rotated on login, and all sessions
  are invalidated on password change. Idle expiry 7 days, absolute 30.
- WebSocket upgrades authenticate the cookie the same as REST (see §4.4 for
  Origin).

## 3. Self-update, verified end to end — ✅ shipped (Phase B)

The feature stays (trust-root argument lives in `THREAT_MODEL.md`); what changes
is that every step from tag to running container is now verified, and the gap
between "checked" and "applied" is closed. Implementation notes below record how
it shipped; the design as described held, with two concrete choices: in-app
verification execs a **bundled cosign binary** (chosen over vendoring the
sigstore-go library, which would have forced a Go-version bump and a large
dependency tree — the repo stays stdlib-first on Go 1.23), and verification runs
**offline** against a trust root seeded into the image so the outbound inventory
(§3.5) stays ghcr-only in the common case.

### 3.1 Release pipeline

Every release image is cosign-signed, keyless, via GitHub Actions OIDC. That
relocates the crown jewels from a registry password to the workflow itself, so
the workflow gets the protection: protected tags, a release environment with a
required reviewer, all actions pinned by SHA. SLSA provenance attestation is
attached (the official generator makes this nearly free). Builds are multi-arch
(amd64 + arm64) and the signature covers the manifest index.

### 3.2 Check path

`GET /api/app/update` now does, in order: resolve newest semver tag → fetch
manifest digest → cosign-verify the digest against the pinned identity
(`https://github.com/<owner>/hivedock/.github/workflows/release.yml@refs/tags/*`,
issuer `token.actions.githubusercontent.com`, both baked in at build time) →
require strictly greater than the running version → cache 15 min as today.

The downgrade guard applies even to validly signed images: a stale or malicious
tag must not be able to walk an install backward onto a signed-but-vulnerable
release. Failed verification is not a silent skip; it surfaces as an alert state
("newer tag exists, signature verification failed"), because that alarm is the
entire point of the scheme.

### 3.3 Apply path

The old flow verified nothing at pull time: the helper ran
`compose pull && up -d` and deployed whatever the tag pointed at *then*, which
can differ from what was checked. New flow:

1. The server records the approved, verified digest `D` from the last check.
2. The helper container is launched from the currently running image, pinned by
   its own RepoDigest (already-trusted bytes, not a tag).
3. The helper pulls `ghcr.io/<owner>/hivedock@D` by digest, never by tag.
4. It tags the pulled image locally as whatever ref the Hivedock stack's compose
   file names, then runs `docker compose up -d --pull never` on the project, so
   compose recreates from the exact verified bytes even if the remote tag moved
   in the meantime.
5. Post-check: the new container's image digest must equal `D`.

Failure modes: signature invalid → no offer, alert shown. Pull fails → old
container keeps running, error reported. Recreate fails → compose's own behavior
applies and the error streams to the UI; SSH remains the universal fallback,
stated in the threat model.

**Tag hygiene (as implemented).** Step 4 retags the verified bytes onto whatever
`image:` ref the Hivedock stack names, so self-update targets a *mutable* tag —
track `ghcr.io/<owner>/hivedock:latest`, which is what the shipped compose
(`deploy/pct101.compose.yaml`, README example) uses. If the stack instead pins an
exact version (`…:1.0.0`), the running container does get the verified new bytes,
but the local tag then diverges from the registry, so a later manual
`docker compose pull` could revert it. Rewriting a pinned `image:` line to the new
version through the comment-preserving editor (so the file matches what's running)
is a planned enhancement tracked with §5 file-trust; until then, use `:latest` for
Hivedock's own image.

### 3.4 Modes

A Settings radio: `full` (default), `check-only`, `off`. Default rationale: for a
socket-holding tool the dangerous steady state is a fleet of stale installs, not
a signed, human-triggered update channel. Verification is the gate; the click
stays the trigger. If launch-day optics end up mattering more, defaulting to
`check-only` is a one-line change and the announcement copy works either way.
`off` exists for the air-gapped crowd and disables the version check entirely.

### 3.5 Outbound connections inventory

Goes into the README verbatim, because "does it phone home" appears in every
thread:

1. `ghcr.io`, version check for Hivedock itself and its cosign signature fetch
   (this feature; `off` = no call). The transparency-log proof is verified
   offline against a trust root baked into the image, so Rekor is not contacted;
   cosign may occasionally refresh an expired root from the sigstore TUF CDN.
2. Registries you configure, for image update checking.
3. Icon CDNs (dashboard-icons / selfh.st), once per newly seen image, then cached
   on disk and served locally.

Nothing else. No analytics, no crash reporting. CI includes a periodic review
note in `SECURITY.md` to keep this list honest.

## 4. Hardening pass

Each item lands with a test; the README security section (§7) lists them once
they exist, not before.

### 4.1 Stack names

Allowlist `^[a-z0-9][a-z0-9_-]{0,63}$`, enforced on create and rename (this is
roughly compose's own project-name rule anyway). Kills path traversal via names
at the door.

### 4.2 Path containment

Every file operation (read, write, rename, delete, editor load/save) resolves
its target with `filepath.EvalSymlinks` and requires the result to sit under the
resolved `STACKS_DIR`. Symlinks inside the tree are followed but their targets
must still resolve inside the root; anything escaping is refused with a clear
error. Property tests feed traversal and symlink attempts.

### 4.3 Subprocess discipline — ✅ shipped (Phase B)

`exec.Command` with argument arrays only, no shell, anywhere. Enforced by a CI
grep gate (`.github/workflows/ci.yml`) that fails the build on `sh -c` /
`bash -c` — and a shell container entrypoint — in non-test Go code, so the claim
"no shell interpolation" is checkable, not aspirational. This was deferred until
Phase B because the old self-update helper shelled out a `compose pull && up -d`
pipeline; §3.3 replaced it with the `hivedock apply-update` subcommand, so the
gate could land clean.

### 4.4 WebSocket origin — ✅ shipped (v0.3.2)

`/api/ws` upgrades only when the `Origin` host matches the request `Host` (or
the configured `PUBLIC_HOST`); a missing Origin (non-browser clients) is allowed
as it carries no ambient cookies. Cross-site WebSocket hijacking is the classic
dashboard bug; this closes the browser vector now, and cookie auth at upgrade
(§2.4) completes it once §2 lands. `api.checkWSOrigin`, table-tested.

### 4.5 Response headers

Because the SPA is embedded and icons are proxied server-side, there are no
external asset origins, which makes a strict CSP actually achievable:
`default-src 'self'` as the target, `img-src 'self' data:`, plus
`frame-ancestors 'none'` (clickjacking a "stack down" button is a real
scenario), `X-Content-Type-Options: nosniff`, and a restrictive
`Referrer-Policy`. Exact policy gets pinned during implementation against what
Vite/Tailwind emit; the invariant is zero external origins.

### 4.6 Untrusted string rendering — ✅ shipped (v0.3.2)

Container log lines are attacker-controlled the moment any container on the host
is compromised, and `hivedock.description` labels are attacker-influenced by
design. `sanitizeLogLine` strips CSI/OSC/two-byte escape sequences and stray
control bytes server-side (keeping printable text + tabs); the UI does its own
severity coloring, so we drop SGR entirely rather than run an allowlist
converter. All dynamic strings already render as React text nodes; a CI grep
gate bans `dangerouslySetInnerHTML` outright. Table-tested.

### 4.7 Editor and secrets stance

`.env` files in stack directories are readable and editable through the editor,
as plaintext. That's a deliberate, documented decision (the threat model states
it); masking stays in the parking lot. Being explicit beats being discovered.

### 4.8 Fuzzing

Go fuzz targets for the YAML tag rewriter (mutations of the corpus must leave
every byte outside the target scalar untouched, see §5.3) and property tests for
the name validator and path containment.

## 5. File trust

### 5.1 Optimistic locking on save

Editor load returns the file's SHA-256 alongside content; save must present it.
Mismatch → 409 with the fresh content, and the UI offers reload-and-reapply or
overwrite. No more silently clobbering an edit someone made over SSH.

### 5.2 Diff preview for machine edits

Any write Hivedock generates itself (today: the update tag rewrite) returns a
unified diff first and applies only on confirmation, within the same
optimistic-lock window captured when the diff was produced.

### 5.3 Byte-exactness enforcement

The rewriter already targets scalars; this makes it a hard guarantee. After an
in-memory rewrite, the byte diff against the original must consist of exactly
the intended scalar span(s). Anything else → abort the write, surface the diff,
touch nothing. Turns "comment-preserving" from a goal into an invariant with a
fuzzer behind it.

### 5.4 Git auto-commit, opt-in

Setting off by default (zero-config stays true). When on: `STACKS_DIR` must be a
git worktree (one-click "init for me"); before any Hivedock-initiated write, a
dirty worktree gets a `hivedock: snapshot before <action>` commit; after the
write, a `hivedock: <action>` commit with a fixed author. No remotes, no push,
no branching. A failed commit aborts the mutation and shows stderr, because a
broken paper trail should stop the press, not get skipped.

## 6. Ops resilience

### 6.1 Registry credentials and backoff

Per-registry credentials in Settings (stored in SQLite under `DATA_DIR` file
permissions; full at-rest encryption is out of scope and the threat model says
so). Update sweeps get a per-registry rate limiter, jitter that spreads checks
across the interval window instead of a thundering herd at tick, exponential
backoff on 429, and `Retry-After` honored. Docs get a section on Docker Hub
anonymous limits with current numbers linked rather than hardcoded, since Hub
keeps changing them.

### 6.2 Per-registry TLS trust

Self-signed homelab registries are a fact of life: per-registry CA bundle path
and an insecure toggle, both scoped to a single registry host, default strict.

### 6.3 Invariant-4 startup self-check

On boot, Hivedock inspects its own container via the socket, finds the bind
whose destination is `STACKS_DIR`, and requires the source to be the identical
path. On mismatch it starts in read-only mode with a banner explaining exactly
what to fix, instead of letting relative binds in user stacks resolve against
wrong paths. Cheap, exact, and it pre-empts the entire Synology/WSL support
thread genre.

### 6.4 Runtime detection

Podman and rootless Docker get detected from daemon info at startup and produce
an explicit "unsupported, here's why" banner and doc page. Honest beats silent.

### 6.5 Read-only API token

Settings-generated bearer token, stored hashed, revocable, valid only for an
allowlist of GET routes (`/api/health`, `/api/stacks`, `/api/updates`). Exists
so uptime-kuma/gatus/scripts can watch update state without a notification
subsystem being rebuilt. `/api/settings` is explicitly excluded.

## 7. Disclosure deliverables

- `SECURITY.md`: supported versions, private reporting via GitHub security
  advisories, response-time targets, the outbound-connections list (§3.5).
- `THREAT_MODEL.md`: assets; trust boundaries (browser↔app, app↔socket,
  app↔registries, release pipeline); the plain statement that socket access is
  root-equivalent and what an attacker gets from compromising Hivedock
  (everything); deployment assumptions (LAN/VPN, WAN only behind proxy auth);
  residual risks named honestly: release-pipeline compromise, secrets on disk,
  `.env` in the editor.
- README security section summarizing §§2–5 mitigations, linking both docs.
- `deploy/compose.hardened.yaml`: `cap_drop: [ALL]`, `no-new-privileges`,
  read-only rootfs with tmpfs where needed, resource limits, plus a
  socket-proxy allowlist config for read-mostly deployments, with the honest
  caveat printed next to it that mutations require POST and the proxy therefore
  reduces surface without removing root-equivalence.
- Repo hygiene: LICENSE file, dependabot, `govulncheck` + `staticcheck` in CI,
  the two grep gates (§4.3, §4.6), and a README paragraph owning the Claude
  Code workflow with links to the corpora that do the disciplining.

## 8. Implementation plan

| Phase | Ships | Exit criteria | Size |
|---|---|---|---|
| **A — footgun & docs** | §2 complete (removal + boot refusal, trusted-header auth, first-run token, rate limiting, session hardening); `SECURITY.md`; `THREAT_MODEL.md`; repo hygiene | Boot with `AUTH_DISABLED` fails with the message; setup unreachable without log token; fail2ban filter tested against real log output; forward-auth verified behind Authelia on PCT 102 | 1 weekend |
| **B — supply chain** ✅ | §3 complete: cosign in CI, verify-in-app, digest-pinned helper flow, downgrade guard, modes, outbound inventory; multi-arch builds; §4.3 no-shell gate | A tampered/unsigned tag is refused and surfaces the alert state; end-to-end self-update on PCT 102 lands on the exact approved digest; arm64 image boots | 1 weekend |
| **C — hardening pass** | §4 complete with tests; README security section | Traversal/symlink property tests green; WS upgrade rejects foreign Origin; CSP has zero external origins; grep gates wired into CI and failing on planted violations | 1 weekend |
| **D — file trust** | §5 complete; golden-file + fuzz corpora linked from README with a "PR tags that fool it" invitation | Concurrent-edit save returns 409; rewriter fuzzer runs clean; git opt-in produces the two-commit pattern; a deliberately imperfect rewrite aborts | 1–2 weekends |
| **E — ops** | §6 complete; hardened compose + proxy doc | 30-stack sweep stays under per-registry limits with jitter visible in logs; parity mismatch boots read-only with banner; read-only token rejects non-allowlisted routes; podman banner fires | 1 weekend |

Order rationale: A kills the top thread comments and unblocks everything. B must
precede the announcement because the post claims it. C and D are what the CVE
hunters grep for. E can trail, except multi-arch, which moved into B (it's
release-workflow work anyway, and the Pi crowd files the issue within hours).

**Launch gate: the r/selfhosted post goes out after A–D are merged, because it
describes them in the present tense.**

## 9. Migration notes

- `AUTH_DISABLED` users: boot refusal message (§2.1) points at forward-auth docs
  and first-run setup. Unsetting the var drops them into setup, which is
  non-destructive.
- Session schema migration invalidates existing sessions once (everyone logs in
  again); release notes say so.
- New settings keys (`update mode`, registry credentials, git opt-in, API token)
  default to current behavior except update verification, which is not optional.
- An in-flight old-style helper from a previous version is unaffected; the new
  flow applies from the next check.

## 10. CLAUDE.md invariant additions

Append to the numbered list:

8. Any Hivedock-generated file write must be byte-identical outside the intended
   scalar span(s), or it aborts. Never best-effort a file edit.
9. No shell interpolation. `exec.Command` argument arrays only; CI enforces.
10. Auth headers are trusted only from configured proxy CIDRs, decided by the
    TCP peer address. There is no auth bypass switch and one must never be
    reintroduced.
11. Self-update applies only cosign-verified, digest-pinned images strictly
    newer than the running version.
