// Package compose runs read-only `docker compose` subcommands. Phase 1 uses
// only `config --hash` for drift detection; the mutating runner (up/down/etc.)
// arrives in Phase 3. Compose operations always shell out — never a
// reimplementation (see docs/CLAUDE.md).
package compose

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ConfigHashes returns the canonical per-service config hash for a compose
// file, computed against the RESOLVED config (env interpolation included) —
// this is what Docker Compose stamps onto containers as
// com.docker.compose.config-hash, so it's the correct thing to diff for drift.
//
// It runs: docker compose -f <file> --project-directory <dir> config --hash '*'
// which prints "service<TAB>hash" lines and does not touch the daemon.
func ConfigHashes(ctx context.Context, composeFile, projectDir string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", composeFile,
		"--project-directory", projectDir,
		"config", "--hash", "*",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose config --hash: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	hashes := map[string]string{}
	sc := bufio.NewScanner(&stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Output is "service<whitespace>hash".
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			hashes[fields[0]] = fields[len(fields)-1]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan config --hash output: %w", err)
	}
	return hashes, nil
}
