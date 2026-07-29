package hostops

import (
	"context"

	"github.com/rogalinski/hivedock/internal/stacks"
)

// Backend is the portable stack-management surface. LocalBackend runs it in
// process against a STACKS_DIR + docker socket; the manager's remoteBackend runs
// it over the agent RPC. The two are interchangeable, so a handler picks a backend
// by host and calls it — local and remote go through identical code.
//
// Locking is the CALLER's responsibility for the streaming ops (mirrors
// compose.Runner.Exec, which documents "the caller must already hold the stack
// lock") so the synchronous 409-on-busy stays where the HTTP handler is.
type Backend interface {
	ListStacks(ctx context.Context) ([]stacks.Stack, error)
	GetStack(ctx context.Context, name string) (stacks.Stack, error)
	GetCompose(ctx context.Context, name string) (ComposeFile, error)
	ValidateCompose(ctx context.Context, name string, content []byte) (Validation, error)
	GetEnv(ctx context.Context, name string) (EnvFile, error)

	PutCompose(ctx context.Context, name string, content []byte, baseSha string) (ComposeFile, error)
	PutEnv(ctx context.Context, name string, content []byte, baseSha string) (EnvFile, error)
	CreateStack(ctx context.Context, name, composeYAML string) (StackRef, error)
	DeleteStack(ctx context.Context, name string, volumes bool) error
	RenameStack(ctx context.Context, name, newName string) (StackRef, error)
	UpdateService(ctx context.Context, req UpdateServiceReq) (UpdateServiceResult, error)

	// Streaming. The caller already holds the per-stack lock.
	RunAction(ctx context.Context, name, action, service string, onLine func(string)) error
	Logs(ctx context.Context, name string, tail int, onLine func(LogLine)) error
}

// Result types mirror the JSON the browser already receives, so the wire contract
// is preserved whether a stack is local or remote.

type ComposeFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Sha256  string `json:"sha256"`
}

type EnvFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Sha256  string `json:"sha256"`
	Exists  bool   `json:"exists"`
}

type StackRef struct {
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	ComposeFile string `json:"composeFile"`
}

type Validation struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// LogLine is one container log line. The server layer adds Host/Stack when it
// re-publishes to the browser.
type LogLine struct {
	Service string `json:"service"`
	Stream  string `json:"stream"`
	Line    string `json:"line"`
}

type UpdateServiceReq struct {
	Name       string
	Service    string
	Tag        string
	BaseSha256 string
	Confirm    bool
}

type UpdateServiceResult struct {
	Stack   string `json:"stack"`
	Service string `json:"service"`
	Tag     string `json:"tag"`
	Sha256  string `json:"sha256,omitempty"`
	Diff    string `json:"diff,omitempty"`
	Changed bool   `json:"changed"`
	Preview bool   `json:"preview"`
}
