// Package hostops holds the portable stack-management core that BOTH the manager
// (in-process, for the local host) and a remote `hivedock agent` link and run —
// so remote and local go through identical code with identical security
// invariants (docs/MULTIHOST.md). It has no HTTP: handlers map its typed results
// and errors to responses; the wire (agentrpc) carries a machine code so a remote
// failure maps to the same HTTP status as a local one.
package hostops

import (
	"encoding/json"
	"errors"

	"github.com/rogalinski/hivedock/internal/compose"
)

// Domain errors. These cross the wire as codes (see CodeFor / ErrorForCode) and
// map to HTTP status on the manager (see the server's httpError).
var (
	ErrNotFound    = errors.New("stack not found")
	ErrExternal    = errors.New("stack is external (read-only); no compose file to edit")
	ErrBusy        = errors.New("an operation is already running for this stack")
	ErrReadOnly    = errors.New("host is read-only")
	ErrInvalidName = errors.New("invalid stack name: use lowercase letters, digits, dash or underscore, starting with a letter or digit (max 64)")
	ErrExists      = errors.New("a stack with that name already exists")
	ErrRunning     = errors.New("stop the stack first (it still has containers)")
	ErrEscape      = errors.New("refusing to access a path outside the stacks directory")
	ErrNoDocker    = errors.New("docker is unavailable on this host")
	ErrOffline     = errors.New("host is offline")
)

// ConflictError is the optimistic-lock failure: the file changed on disk since it
// was loaded. It carries the current bytes so the UI can reconcile (matching the
// local 409 body).
type ConflictError struct {
	Content string `json:"content"`
	Sha256  string `json:"sha256"`
}

func (e *ConflictError) Error() string { return "this file changed on disk since you opened it" }

// ValidationError is a `docker compose config` failure on a compose save (→ 422).
// It carries the compose error message.
type ValidationError struct {
	Msg string `json:"error"`
}

func (e *ValidationError) Error() string { return e.Msg }

// Machine error codes carried in agentrpc.Response.Code.
const (
	CodeNotFound        = "not_found"
	CodeExternal        = "external"
	CodeBusy            = "busy"
	CodeReadOnly        = "read_only"
	CodeInvalidName     = "invalid_name"
	CodeExists          = "exists"
	CodeRunning         = "running"
	CodeConflict        = "conflict"
	CodeValidation      = "validation"
	CodeEnvManaged      = "env_managed"
	CodeDigestPinned    = "digest_pinned"
	CodeServiceNotFound = "service_not_found"
	CodeNoImage         = "no_image"
	CodeEscape          = "escape"
	CodeNoDocker        = "no_docker"
	CodeCanceled        = "canceled"
	CodeOffline         = "offline"
)

// CodeFor maps a domain error to its wire code, or "" for a generic (500) error.
func CodeFor(err error) string {
	var conflict *ConflictError
	var validation *ValidationError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &conflict):
		return CodeConflict
	case errors.As(err, &validation):
		return CodeValidation
	case errors.Is(err, ErrNotFound):
		return CodeNotFound
	case errors.Is(err, ErrExternal):
		return CodeExternal
	case errors.Is(err, ErrBusy):
		return CodeBusy
	case errors.Is(err, ErrReadOnly):
		return CodeReadOnly
	case errors.Is(err, ErrInvalidName):
		return CodeInvalidName
	case errors.Is(err, ErrExists):
		return CodeExists
	case errors.Is(err, ErrRunning):
		return CodeRunning
	case errors.Is(err, ErrEscape):
		return CodeEscape
	case errors.Is(err, ErrNoDocker):
		return CodeNoDocker
	case errors.Is(err, ErrOffline):
		return CodeOffline
	case errors.Is(err, compose.ErrEnvManaged):
		return CodeEnvManaged
	case errors.Is(err, compose.ErrDigestPinned):
		return CodeDigestPinned
	case errors.Is(err, compose.ErrServiceNotFound):
		return CodeServiceNotFound
	case errors.Is(err, compose.ErrNoImage):
		return CodeNoImage
	default:
		return ""
	}
}

// ErrorForCode reconstructs a typed error from a wire code (+ optional data, e.g.
// the conflict bytes), so the manager returns the same HTTP status for a remote
// failure as for a local one. text is the human message (used when the code is
// empty/unknown).
func ErrorForCode(code, text string, data json.RawMessage) error {
	switch code {
	case "":
		if text == "" {
			return nil
		}
		return errors.New(text)
	case CodeConflict:
		var c ConflictError
		if len(data) > 0 {
			_ = json.Unmarshal(data, &c)
		}
		return &c
	case CodeValidation:
		return &ValidationError{Msg: text}
	case CodeNotFound:
		return ErrNotFound
	case CodeExternal:
		return ErrExternal
	case CodeBusy:
		return ErrBusy
	case CodeReadOnly:
		return ErrReadOnly
	case CodeInvalidName:
		return ErrInvalidName
	case CodeExists:
		return ErrExists
	case CodeRunning:
		return ErrRunning
	case CodeEscape:
		return ErrEscape
	case CodeNoDocker:
		return ErrNoDocker
	case CodeOffline:
		return ErrOffline
	case CodeEnvManaged:
		return compose.ErrEnvManaged
	case CodeDigestPinned:
		return compose.ErrDigestPinned
	case CodeServiceNotFound:
		return compose.ErrServiceNotFound
	case CodeNoImage:
		return compose.ErrNoImage
	default:
		if text != "" {
			return errors.New(text)
		}
		return errors.New(code)
	}
}
