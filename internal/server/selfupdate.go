package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rogalinski/hivedock/internal/registry"
)

// selfImage is the repository HiveDock is distributed from; the self-update
// check compares the running version against its published tags and cosign-
// verifies a candidate's signature before offering it.
const selfImage = "ghcr.io/rodzalendo/hivedock"

// selfRepoURL is HiveDock's GitHub repo. Note this is deliberately NOT the Go
// module path (github.com/rogalinski/hivedock): the GHCR org and GitHub owner
// are "rodzalendo", the module path is "rogalinski". Used for release-notes
// links and, below, the cosign certificate identity.
const selfRepoURL = "https://github.com/rodzalendo/hivedock"

// Keyless cosign verification parameters (HARDENING.md §3.2), baked into the
// binary. A candidate image is offered only when it carries a valid signature
// whose certificate identity is HiveDock's own release workflow (at any version
// tag) and whose OIDC issuer is GitHub Actions. Owner/repo/issuer are fixed;
// only the tag varies, hence the identity regexp.
const (
	cosignIdentityRegexp = `^https://github\.com/rodzalendo/hivedock/\.github/workflows/release\.yml@refs/tags/v.*$`
	cosignIssuer         = "https://token.actions.githubusercontent.com"
)

// Update modes (§3.4), stored under settingAppUpdateMode:
//   - full:       check + verify + one-click apply (default)
//   - check-only: check + verify, but apply is refused
//   - off:        no version check at all (air-gapped installs)
const settingAppUpdateMode = "app_update_mode"

const (
	updateModeFull      = "full"
	updateModeCheckOnly = "check-only"
	updateModeOff       = "off"
)

func validUpdateMode(m string) bool {
	switch m {
	case updateModeFull, updateModeCheckOnly, updateModeOff:
		return true
	}
	return false
}

// appUpdateMode returns the configured update mode, defaulting to "full".
func (a *api) appUpdateMode() string {
	if a.db != nil {
		if v, ok, err := a.db.GetSetting(settingAppUpdateMode); err == nil && ok && validUpdateMode(v) {
			return v
		}
	}
	return updateModeFull
}

// selfRegistry resolves the manifest digest of a HiveDock release tag — the
// slice of the registry client the self-update check needs. An interface keeps
// the check unit-testable without real HTTP.
type selfRegistry interface {
	Digest(ctx context.Context, ref registry.Ref) (string, error)
}

// imageVerifier verifies that an image (repo@sha256:…) carries a valid HiveDock
// release signature. A nil error means verified. The default execs the bundled
// cosign binary; tests inject a fake.
type imageVerifier interface {
	Verify(ctx context.Context, digestRef string) error
}

// cosignVerifier verifies keyless cosign signatures by execing the bundled
// cosign binary with an argument array — no shell, satisfying invariant 9.
// Verification is offline for the transparency-log proof (the Rekor bundle
// travels with the signature in the registry), so the only outbound call is
// fetching the signature from ghcr; the outbound inventory (§3.5) stays honest.
type cosignVerifier struct{}

func (cosignVerifier) Verify(ctx context.Context, digestRef string) error {
	cmd := exec.CommandContext(ctx, "cosign", "verify",
		"--certificate-identity-regexp", cosignIdentityRegexp,
		"--certificate-oidc-issuer", cosignIssuer,
		"--offline", // verify the tlog proof from the bundle, don't call Rekor
		digestRef,
	)
	// No key material is used; COSIGN_YES avoids any interactive confirmation.
	cmd.Env = append(os.Environ(), "COSIGN_YES=true")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cosign verify: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// appUpdateResponse tells the sidebar whether a newer, verified HiveDock release
// exists.
type appUpdateResponse struct {
	Current      string `json:"current"`
	Candidate    string `json:"candidate,omitempty"`
	HasUpdate    bool   `json:"hasUpdate"`
	Checkable    bool   `json:"checkable"` // false for dev/edge builds or mode=off
	NotesURL     string `json:"notesUrl,omitempty"`
	Mode         string `json:"mode"`                   // full | check-only | off (§3.4)
	VerifyFailed bool   `json:"verifyFailed,omitempty"` // a newer tag exists but its signature failed to verify (§3.2 alert)
}

// selfCheckCache memoizes the registry lookup so page reloads (the UI checks on
// every load) don't hammer ghcr, and records the verified digest the apply path
// will deploy.
type selfCheckCache struct {
	mu   sync.Mutex
	at   time.Time
	resp appUpdateResponse
	// approvedDigest is the cosign-verified manifest digest (sha256:…) of the
	// candidate from the last successful check; the apply path (§3.3) pulls
	// exactly these bytes. approvedTag is the tag it corresponds to. Both empty
	// when the last check found nothing verifiable.
	approvedDigest string
	approvedTag    string
}

// appUpdate reports whether a newer, signature-verified release than the running
// one is published.
func (a *api) appUpdate(w http.ResponseWriter, r *http.Request) {
	mode := a.appUpdateMode()
	if mode == updateModeOff || !registry.IsVersion(version) {
		// off, or dev/edge builds: no semver to compare, nothing to check.
		writeJSON(w, http.StatusOK, appUpdateResponse{Current: version, Mode: mode})
		return
	}

	a.selfCheck.mu.Lock()
	if !a.selfCheck.at.IsZero() && time.Since(a.selfCheck.at) < 15*time.Minute {
		cached := a.selfCheck.resp
		cached.Mode = mode // mode can change without forcing a re-check
		a.selfCheck.mu.Unlock()
		writeJSON(w, http.StatusOK, cached)
		return
	}
	a.selfCheck.mu.Unlock()

	resp, digest := a.runSelfCheck(r.Context(), mode)
	a.storeSelfCheck(resp, digest)
	writeJSON(w, http.StatusOK, resp)
}

// storeSelfCheck memoizes a completed check together with its verified digest.
func (a *api) storeSelfCheck(resp appUpdateResponse, digest string) {
	a.selfCheck.mu.Lock()
	a.selfCheck.at = time.Now()
	a.selfCheck.resp = resp
	a.selfCheck.approvedDigest = digest
	a.selfCheck.approvedTag = resp.Candidate
	if digest == "" {
		a.selfCheck.approvedTag = ""
	}
	a.selfCheck.mu.Unlock()
}

// runSelfCheck performs the uncached self-update check: find the freshest newer
// release (the semver engine already enforces strictly-greater plus the stale-
// tag guard — that IS the downgrade guard), then resolve and cosign-verify its
// digest. Returns the response and, on success, the verified digest for the
// apply path. A newer-but-unverifiable tag yields VerifyFailed and no offer —
// surfacing the alarm is the entire point of the scheme (§3.2).
func (a *api) runSelfCheck(ctx context.Context, mode string) (appUpdateResponse, string) {
	resp := appUpdateResponse{Current: version, Checkable: true, Mode: mode}

	res := a.checker.CheckImage(ctx, selfImage+":"+version)
	if !res.HasUpdate || res.Candidate == "" {
		return resp, ""
	}

	offer, digest, verifyFailed := a.evaluateCandidate(ctx, res.Candidate)
	if !offer {
		resp.VerifyFailed = verifyFailed
		return resp, ""
	}
	resp.HasUpdate = true
	resp.Candidate = res.Candidate
	resp.NotesURL = selfRepoURL + "/releases/tag/v" + res.Candidate
	return resp, digest
}

// evaluateCandidate resolves a candidate tag's manifest digest and cosign-
// verifies it. offer is true only when the signature is valid; verifyFailed is
// true whenever a resolvable candidate could not be verified (the alert state).
func (a *api) evaluateCandidate(ctx context.Context, candidate string) (offer bool, digest string, verifyFailed bool) {
	ref, err := registry.ParseImageRef(selfImage + ":" + candidate)
	if err != nil {
		a.logger.Error("self-update check: parse candidate", "candidate", candidate, "err", err)
		return false, "", true
	}
	digest, err = a.selfReg.Digest(ctx, ref)
	if err != nil {
		a.logger.Error("self-update check: resolve candidate digest", "candidate", candidate, "err", err)
		return false, "", true
	}
	if err := a.verify.Verify(ctx, selfImage+"@"+digest); err != nil {
		a.logger.Warn("self-update check: signature verification FAILED — refusing to offer update",
			"candidate", candidate, "digest", digest, "err", err)
		return false, "", true
	}
	a.logger.Info("self-update check: verified newer release", "candidate", candidate, "digest", digest)
	return true, digest, false
}

// selfUpdate replaces the running HiveDock container with the newest verified
// image. A container cannot `compose up` itself — it would be killed halfway
// through its own recreate — so this launches a small DETACHED helper container
// (this same binary, invoked as `hivedock apply-update`) that pulls the approved
// digest, retags it locally, and recreates HiveDock's compose project from those
// exact bytes with `--pull never`. Every step is an argument-array exec inside
// Go: no shell anywhere (invariant 9). The UI polls /api/health until the new
// version answers.
func (a *api) selfUpdate(w http.ResponseWriter, r *http.Request) {
	mode := a.appUpdateMode()
	if mode != updateModeFull {
		writeError(w, http.StatusForbidden, "self-update is disabled — update mode is "+mode)
		return
	}
	if !registry.IsVersion(version) {
		writeError(w, http.StatusConflict, "dev/edge builds cannot self-update (no release version to verify)")
		return
	}

	// The image is applied only by its cosign-verified digest. Prefer the last
	// check's result; refresh it if stale or empty so an apply never runs on an
	// unverified image.
	digest, tag, ok := a.approvedDigest(r.Context(), mode)
	if !ok {
		writeError(w, http.StatusConflict, "no verified update available")
		return
	}

	if !a.selfUpdating.CompareAndSwap(false, true) {
		writeError(w, http.StatusConflict, "a self-update is already in progress")
		return
	}
	// If the update succeeds this process is replaced and the flag dies with it;
	// if the helper fails out-of-band, let the user retry after a while.
	time.AfterFunc(5*time.Minute, func() { a.selfUpdating.Store(false) })

	workdir, cfgFile, imageRef, err := selfComposeProject()
	if err != nil {
		a.selfUpdating.Store(false)
		writeError(w, http.StatusConflict, "cannot self-update: "+err.Error())
		return
	}

	// Pin the HELPER image to the exact bytes we're already running (its own
	// repo digest), not a moving tag — already-trusted bytes (§3.3 step 2).
	helperImage := imageRef
	if a.docker != nil {
		if selfDigest, derr := a.docker.ImageRepoDigest(r.Context(), imageRef); derr == nil && selfDigest != "" {
			helperImage = selfImage + "@" + selfDigest
		}
	}

	cmd := exec.CommandContext(r.Context(), "docker", "run", "-d", "--rm",
		"--name", "hivedock-self-update",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", workdir+":"+workdir,
		helperImage, "apply-update",
		"--digest", digest,
		"--project-dir", workdir,
		"--compose-file", cfgFile,
		"--image-ref", imageRef,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		a.selfUpdating.Store(false)
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		a.logger.Error("self-update: launch helper", "err", err, "out", msg)
		writeError(w, http.StatusInternalServerError, "failed to launch updater: "+msg)
		return
	}
	a.logger.Info("self-update helper launched",
		"container", strings.TrimSpace(string(out)), "candidate", tag, "digest", digest)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "updating"})
}

// approvedDigest returns the cosign-verified digest (and its tag) to deploy,
// refreshing the check when the cached result is stale or absent so an apply is
// never launched on an unverified image.
func (a *api) approvedDigest(ctx context.Context, mode string) (digest, tag string, ok bool) {
	a.selfCheck.mu.Lock()
	digest, tag = a.selfCheck.approvedDigest, a.selfCheck.approvedTag
	fresh := !a.selfCheck.at.IsZero() && time.Since(a.selfCheck.at) < 15*time.Minute
	a.selfCheck.mu.Unlock()
	if digest != "" && fresh {
		return digest, tag, true
	}
	resp, d := a.runSelfCheck(ctx, mode)
	a.storeSelfCheck(resp, d)
	return d, resp.Candidate, d != ""
}

// selfComposeProject discovers the running HiveDock container's own compose
// project — host-side working dir, compose file, and image ref — from the labels
// docker compose stamps on every container it creates. Errors when HiveDock
// isn't running as a compose-managed container (dev builds, plain docker run).
func selfComposeProject() (workdir, cfgFile, imageRef string, err error) {
	hostname, _ := os.Hostname() // inside a container this is the container id
	for _, target := range []string{hostname, "hivedock"} {
		if target == "" {
			continue
		}
		out, ierr := exec.Command("docker", "inspect", "--format",
			`{{index .Config.Labels "com.docker.compose.project.working_dir"}}|{{index .Config.Labels "com.docker.compose.project.config_files"}}|{{.Config.Image}}`,
			target).Output()
		if ierr != nil {
			err = fmt.Errorf("inspect %s: %w", target, ierr)
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 3)
		if len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
			// config_files may list several, comma-separated; the first is the
			// project's main compose file.
			return parts[0], strings.Split(parts[1], ",")[0], parts[2], nil
		}
		err = fmt.Errorf("container %q has no compose labels — was HiveDock started with docker compose?", target)
	}
	return "", "", "", err
}

// ApplyUpdateOpts configures the detached, digest-pinned recreate performed by
// the `hivedock apply-update` helper (§3.3).
type ApplyUpdateOpts struct {
	Digest      string // approved manifest digest (sha256:…) to deploy
	ProjectDir  string // compose --project-directory
	ComposeFile string // compose -f
	ImageRef    string // the compose file's image: ref (retag target)
}

// ApplyUpdate pulls the approved image by digest, retags it to the compose image
// ref, and recreates the project from those exact local bytes with --pull never
// — so the new container runs precisely the verified digest even if the remote
// tag moved since the check. Every step is an argument-array exec (no shell).
// Intended to run inside the detached helper container.
func ApplyUpdate(ctx context.Context, logger *slog.Logger, o ApplyUpdateOpts) error {
	if o.Digest == "" || o.ProjectDir == "" || o.ComposeFile == "" || o.ImageRef == "" {
		return fmt.Errorf("apply-update: --digest, --project-dir, --compose-file and --image-ref are all required")
	}
	if !strings.HasPrefix(o.Digest, "sha256:") {
		return fmt.Errorf("apply-update: %q is not a sha256 manifest digest", o.Digest)
	}
	pinned := selfImage + "@" + o.Digest

	run := func(name string, args ...string) error {
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		logger.Info("apply-update step", "cmd", name+" "+strings.Join(args, " "), "out", strings.TrimSpace(string(out)))
		if err != nil {
			return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	// 1. Pull the exact verified bytes by digest (never by tag).
	if err := run("docker", "pull", pinned); err != nil {
		return err
	}
	// 2. Retag locally to the compose image ref so `up -d --pull never` resolves
	//    to these bytes.
	if err := run("docker", "tag", pinned, o.ImageRef); err != nil {
		return err
	}
	// 3. Recreate the project from the local image only — no network, so a moved
	//    remote tag cannot substitute different bytes mid-update.
	if err := run("docker", "compose", "--ansi", "never",
		"-f", o.ComposeFile, "--project-directory", o.ProjectDir,
		"up", "-d", "--pull", "never"); err != nil {
		return err
	}
	// 4. Post-check (§3.3 step 5): the retagged local image must actually carry
	//    the approved digest. Belt-and-suspenders against a wrong image ref.
	ok, err := imageCarriesDigest(ctx, o.ImageRef, o.Digest)
	switch {
	case err != nil:
		logger.Warn("apply-update: post-check inconclusive", "err", err)
	case !ok:
		return fmt.Errorf("apply-update: post-check failed — %s does not resolve to %s", o.ImageRef, o.Digest)
	}
	logger.Info("apply-update: recreated from verified digest", "digest", o.Digest, "imageRef", o.ImageRef)
	return nil
}

// imageCarriesDigest reports whether the local image ref's RepoDigests include
// the given manifest digest.
func imageCarriesDigest(ctx context.Context, imageRef, digest string) (bool, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .RepoDigests}}", imageRef).Output()
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", imageRef, err)
	}
	return strings.Contains(string(out), digest), nil
}
