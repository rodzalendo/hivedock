// Package stacks builds the read-only truth model: it merges compose files on
// disk (the source of truth) with running container state from the daemon and
// classifies each stack as managed or external. Nothing here mutates anything.
package stacks

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// composeFilenames are the canonical compose file names, in precedence order
// (compose itself prefers compose.yaml over docker-compose.yml).
var composeFilenames = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

// ScannedStack is a stack as defined on disk (before merging with runtime state).
type ScannedStack struct {
	Name        string                // directory base name
	Dir         string                // absolute directory path
	ComposeFile string                // absolute path to the compose file
	Project     string                // compose project name (explicit `name:` or normalized dir)
	Services    map[string]ScannedSvc // service name -> definition
}

// ScannedSvc is the subset of a compose service Hivedock reads. Read-only: we
// never round-trip this back to disk.
type ScannedSvc struct {
	Image  string
	Labels map[string]string
}

// composeDoc mirrors just the fields we parse. Unknown fields are ignored.
type composeDoc struct {
	Name     string                    `yaml:"name"`
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image  string        `yaml:"image"`
	Labels composeLabels `yaml:"labels"`
}

// Scan walks stacksDir one level deep and parses every directory that contains
// a compose file. Parse errors for one stack don't fail the whole scan; the
// stack is returned with a ParseErr so the UI can surface it (the UI never lies).
func Scan(stacksDir string) ([]ScannedStack, error) {
	absRoot, err := filepath.Abs(stacksDir)
	if err != nil {
		return nil, fmt.Errorf("resolve stacks dir %q: %w", stacksDir, err)
	}
	entries, err := os.ReadDir(absRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // empty state; caller renders "no stacks found"
		}
		return nil, fmt.Errorf("read stacks dir %q: %w", absRoot, err)
	}

	var stacks []ScannedStack
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(absRoot, e.Name())
		file := findComposeFile(dir)
		if file == "" {
			continue
		}
		s := ScannedStack{
			Name:        e.Name(),
			Dir:         dir,
			ComposeFile: file,
			Project:     NormalizeProject(e.Name()),
			Services:    map[string]ScannedSvc{},
		}
		// Keep the stack visible even if unparsable (the UI never lies); a
		// parse error just leaves Services empty.
		if doc, err := parseCompose(file); err == nil {
			if doc.Name != "" {
				s.Project = doc.Name
			}
			for name, svc := range doc.Services {
				s.Services[name] = ScannedSvc{Image: svc.Image, Labels: svc.Labels}
			}
		}
		stacks = append(stacks, s)
	}
	return stacks, nil
}

func findComposeFile(dir string) string {
	for _, name := range composeFilenames {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func parseCompose(path string) (composeDoc, error) {
	var doc composeDoc
	data, err := os.ReadFile(path)
	if err != nil {
		return doc, fmt.Errorf("read %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return doc, fmt.Errorf("parse %q: %w", path, err)
	}
	return doc, nil
}
