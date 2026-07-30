package hostops

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/rogalinski/hivedock/internal/docker"
)

// SystemCheck runs the boot-time environment checks (§6.3/§6.4) for a host: it
// warns on unsupported runtimes (podman/rootless) and verifies STACKS_DIR bind
// parity, returning a non-empty readOnlyReason when the bind is mismatched so the
// caller refuses writes rather than corrupting stacks. Shared by the manager (its
// local host) and a remote agent, so a bind-mismatched agent refuses writes
// independently (docs/MULTIHOST.md). A nil docker client (dev, no daemon) is a
// no-op — nothing to inspect.
func SystemCheck(ctx context.Context, dc *docker.Client, stacksDir string, logger *slog.Logger) (warnings []string, readOnlyReason string) {
	if dc == nil {
		return nil, ""
	}

	// §6.4 — Podman / rootless Docker are unsupported; say so explicitly rather
	// than break in confusing ways later.
	if rootless, podman := dc.DaemonRuntime(ctx); podman || rootless {
		if podman {
			warnings = append(warnings,
				"Podman detected. HiveDock targets Docker Engine and is unsupported on Podman — the socket API and compose behavior differ.")
		}
		if rootless {
			warnings = append(warnings,
				"Rootless Docker detected. HiveDock is unsupported on rootless Docker — socket path, permissions, and bind semantics differ.")
		}
	}

	// §6.3 — Invariant 4: STACKS_DIR must resolve to the same path inside and
	// outside the container, or compose relative-path resolution silently points
	// at the wrong files. Verify the bind and refuse to mutate on a mismatch.
	hostname, _ := os.Hostname() // inside a container this is the container id
	if hostname == "" {
		return warnings, ""
	}
	src, found, err := dc.SelfBindSource(ctx, hostname, stacksDir)
	switch {
	case err != nil:
		// Not running as an inspectable container (dev, plain binary): can't verify,
		// don't assume broken.
		logger.Debug("startup self-check: could not inspect own container", "err", err)
	case found && src != stacksDir:
		readOnlyReason = fmt.Sprintf(
			"STACKS_DIR bind mismatch: the host path %q is mounted at %q. Docker Compose resolves relative paths against the host path, so a mismatch points them at the wrong files. Fix the bind to %q:%q and restart. HiveDock is running read-only until then.",
			src, stacksDir, stacksDir, stacksDir)
		warnings = append(warnings, readOnlyReason)
		logger.Error("startup self-check FAILED: STACKS_DIR bind mismatch — entering read-only mode",
			"host_source", src, "container_dest", stacksDir)
	case !found:
		logger.Debug("startup self-check: STACKS_DIR is not a bind mount; parity not verified")
	default:
		logger.Info("startup self-check: STACKS_DIR bind parity OK", "path", stacksDir)
	}
	return warnings, readOnlyReason
}
