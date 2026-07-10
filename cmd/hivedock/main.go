// Command hivedock is the single-binary server: it serves the embedded SPA and
// the JSON/WebSocket API for managing Docker compose stacks on one host.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rogalinski/hivedock/internal/config"
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
	cfg := config.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

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
	go host.Run(ctx)

	handler := server.New(cfg, logger, db, stacksSvc, hub, host, webui.Dist())

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
			"auth_disabled", cfg.AuthDisabled,
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
