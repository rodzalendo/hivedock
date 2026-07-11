package stacks

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStack(t *testing.T, root, name, filename, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	writeStack(t, root, "whoami", "compose.yaml", `
services:
  whoami:
    image: traefik/whoami:v1.10.1
    ports:
      - "8081:80"
`)
	writeStack(t, root, "labeled", "docker-compose.yml", `
services:
  web:
    image: nginx:1.27-alpine
    labels:
      - homepage.name=Web
      - homepage.group=Tools
`)
	writeStack(t, root, "explicit-name", "compose.yaml", `
name: custom-project
services:
  app:
    image: alpine:3.20
`)
	// A directory without a compose file must be ignored.
	if err := os.MkdirAll(filepath.Join(root, "not-a-stack"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 stacks, got %d: %+v", len(got), got)
	}

	byName := map[string]ScannedStack{}
	for _, s := range got {
		byName[s.Name] = s
	}

	if s := byName["whoami"]; s.Services["whoami"].Image != "traefik/whoami:v1.10.1" {
		t.Errorf("whoami image = %q", s.Services["whoami"].Image)
	}
	if s := byName["labeled"]; s.Services["web"].Labels["homepage.name"] != "Web" {
		t.Errorf("labeled homepage.name = %q (labels=%+v)", s.Services["web"].Labels["homepage.name"], s.Services["web"].Labels)
	}
	if s := byName["explicit-name"]; s.Project != "custom-project" {
		t.Errorf("explicit-name project = %q, want custom-project", s.Project)
	}
}

func TestScanMissingDir(t *testing.T) {
	got, err := Scan(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil stacks, got %+v", got)
	}
}

func TestNormalizeProject(t *testing.T) {
	cases := map[string]string{
		"whoami":    "whoami",
		"My App":    "myapp",
		"redis-app": "redis-app",
		"App_1":     "app_1",
		"Foo.Bar":   "foobar",
		"UPPER":     "upper",
	}
	for in, want := range cases {
		if got := NormalizeProject(in); got != want {
			t.Errorf("NormalizeProject(%q) = %q, want %q", in, got, want)
		}
	}
}
