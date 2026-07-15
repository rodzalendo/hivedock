// Command hivedock is the single-binary server: it serves the embedded SPA and
// the JSON/WebSocket API for managing Docker compose stacks on one host.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rogalinski/hivedock/internal/config"
	"github.com/rogalinski/hivedock/internal/discovery"
	"github.com/rogalinski/hivedock/internal/docker"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/hoststats"
	"github.com/rogalinski/hivedock/internal/server"
	"github.com/rogalinski/hivedock/internal/stacks"
	"github.com/rogalinski/hivedock/internal/store"
	"github.com/rogalinski/hivedock/internal/watch"
	webui "github.com/rogalinski/hivedock/web"
)

func main() {
	// The detached self-update helper re-invokes this same binary as
	// `hivedock apply-update …` to run the digest-pinned recreate (HARDENING.md
	// §3.3). It needs no config, store, or daemon of its own — only the docker
	// CLI — so handle it before the normal server bootstrap.
	if len(os.Args) > 1 && os.Args[1] == "apply-update" {
		os.Exit(runApplyUpdate(os.Args[2:]))
	}

	cfg := config.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	// AUTH_DISABLED was removed: it turned a socket-holding mutator into an
	// unauthenticated proxy. Fail loudly on a truthy value rather than silently
	// enabling auth (which would strand a user at a login they never created) —
	// a security switch that stops working must be impossible to miss.
	if present, truthy := config.AuthDisabledRemoved(); present {
		if truthy {
			logger.Error("AUTH_DISABLED was removed — it disabled authentication entirely. " +
				"Use trusted-header (forward-auth) SSO via AUTH_TRUSTED_HEADER + AUTH_TRUSTED_PROXY_CIDRS " +
				"(see SECURITY.md and docs/HARDENING.md §2.2), or remove the variable and complete " +
				"first-run setup at the login screen.")
			os.Exit(1)
		}
		logger.Warn("AUTH_DISABLED is set but has been removed and no longer has any effect; delete it from your config")
	}

	if err := run(cfg, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir %q: %w", cfg.DataDir, err)
	}

	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	// The Docker client is best-effort at startup: if the daemon is unreachable
	// (dev without Docker), the server still serves on-disk stacks with unknown
	// status rather than failing to boot.
	var dockerClient *docker.Client
	if dc, err := docker.New(); err != nil {
		logger.Warn("docker client init failed; running without live state", "err", err)
	} else {
		dockerClient = dc
		defer dockerClient.Close()
		if err := dockerClient.Ping(context.Background()); err != nil {
			logger.Warn("docker daemon unreachable at startup", "err", err)
		}
	}

	var lister stacks.ContainerLister
	if dockerClient != nil {
		lister = dockerClient
	}
	stacksSvc := stacks.NewManager(cfg.StacksDir, lister, logger)

	// Event hub + watcher push change notifications to the UI (no polling).
	hub := events.NewHub(300 * time.Millisecond)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	watcher := watch.New(cfg.StacksDir, hub, dockerClient, logger)
	go watcher.Run(ctx)

	host := hoststats.NewSampler(2 * time.Second)
	host.SetDiskPath(cfg.StacksDir) // bind-mounted → reflects the host disk
	go host.Run(ctx)

	icons := discovery.NewIconResolver(cfg.DataDir, nil)

	handler := server.New(ctx, cfg, logger, db, stacksSvc, hub, host, dockerClient, icons, webui.Dist())

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("hivedock listening",
			"addr", httpServer.Addr,
			"stacks_dir", cfg.StacksDir,
			"data_dir", cfg.DataDir,
			"trusted_header_auth", cfg.TrustedHeader != "",
		)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

// runApplyUpdate is the entry point for the detached `hivedock apply-update`
// helper: it pulls the cosign-verified digest and recreates HiveDock's compose
// project from those exact bytes (server.ApplyUpdate). Returns a process exit
// code.
func runApplyUpdate(args []string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	fs := flag.NewFlagSet("apply-update", flag.ContinueOnError)
	var o server.ApplyUpdateOpts
	fs.StringVar(&o.Digest, "digest", "", "approved manifest digest (sha256:…) to deploy")
	fs.StringVar(&o.ProjectDir, "project-dir", "", "compose project directory")
	fs.StringVar(&o.ComposeFile, "compose-file", "", "compose file path")
	fs.StringVar(&o.ImageRef, "image-ref", "", "compose image ref to retag onto the pulled digest")
	if err := fs.Parse(args); err != nil {
		logger.Error("apply-update: parse args", "err", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := server.ApplyUpdate(ctx, logger, o); err != nil {
		logger.Error("apply-update failed", "err", err)
		return 1
	}
	return 0
}
