package compose

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Validate reports whether content is a valid compose file, evaluated in the
// context of projectDir so relative paths and a sibling `.env` resolve the way
// they would on deploy. It runs `docker compose -f - config -q`, feeding the
// candidate content on stdin — nothing is written to disk, so an invalid draft
// can never clobber the live file.
//
// On failure the returned error carries compose's own stderr (the UI never
// lies: it shows the real reason the file was rejected).
func Validate(ctx context.Context, projectDir string, content []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"--project-directory", projectDir,
		"-f", "-",
		"config", "-q",
	)
	cmd.Dir = projectDir
	cmd.Stdin = bytes.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
