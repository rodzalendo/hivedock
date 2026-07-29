// Package agentrpc defines the small JSON protocol spoken between a HiveDock
// manager and a remote `hivedock agent` over one WebSocket (docs/MULTIHOST.md).
// It is types only — no transport — so both the server (manager) and the agent
// client can share the exact wire shapes.
package agentrpc

import "encoding/json"

// Request is a manager → agent call (or the agent's unsolicited register hello,
// which carries no ID and expects no reply). Correlation is by ID.
type Request struct {
	ID     string          `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is an agent → manager reply. A call is either UNARY (one terminal
// Response) or STREAMING (zero or more Kind=="stream" chunk frames, then one
// terminal frame). Terminal = Kind is empty; on success Result holds the value,
// on failure Error holds human text and Code holds a machine code (see errors in
// package hostops) so the manager can map it to the same HTTP status as a local
// failure. A conflict terminal carries the current bytes in Result while Code is
// set — that's why Result doubles as error data when Code != "".
type Response struct {
	ID     string          `json:"id"`
	Kind   string          `json:"kind,omitempty"` // "stream" = intermediate chunk; "" = terminal
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Code   string          `json:"code,omitempty"`
}

// KindStream marks an intermediate chunk of a streaming call (a deploy output
// line, a log line, terminal output). Anything else is terminal.
const KindStream = "stream"

// RegisterParams is the agent's opening hello, identifying the host.
type RegisterParams struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// RemoteContainer is the read-only view of a container on a remote host. A small
// DTO on purpose — the wire shape is decoupled from the manager's internal
// docker types so either side can evolve independently.
type RemoteContainer struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Health  string `json:"health,omitempty"`
	Stack   string `json:"stack,omitempty"`   // compose project
	Service string `json:"service,omitempty"` // compose service
}

// Methods. Phase 1 was read-only (register/ping/listContainers); Phase 2 adds the
// full stack-management surface (docs/MULTIHOST.md), gated identically.
const (
	MethodRegister       = "register"
	MethodPing           = "ping"
	MethodListContainers = "listContainers"

	// Unary stack ops.
	MethodListStacks      = "listStacks"
	MethodGetStack        = "getStack"
	MethodGetCompose      = "getCompose"
	MethodPutCompose      = "putCompose"
	MethodValidateCompose = "validateCompose"
	MethodGetEnv          = "getEnv"
	MethodPutEnv          = "putEnv"
	MethodCreateStack     = "createStack"
	MethodDeleteStack     = "deleteStack"
	MethodRenameStack     = "renameStack"
	MethodUpdateService   = "updateService"

	// Streaming ops.
	MethodRunAction = "runAction" // streams deploy output lines
	MethodLogs      = "logs"      // streams container log lines
	MethodExec      = "exec"      // bidirectional container shell

	// Control frames (manager → agent), correlated to a streaming call's ID.
	MethodCancel     = "cancel"     // abort the streaming call with this ID
	MethodExecInput  = "execInput"  // stdin bytes for an exec stream
	MethodExecResize = "execResize" // TTY resize for an exec stream
)

// Control-frame params.
type CancelParams struct {
	ID string `json:"id"`
}
type ExecInputParams struct {
	ID   string `json:"id"`
	Data []byte `json:"data"` // base64 in JSON
}
type ExecResizeParams struct {
	ID   string `json:"id"`
	Cols uint   `json:"cols"`
	Rows uint   `json:"rows"`
}

// Operation params. Results travel as json.RawMessage decoded against the domain
// types on the far side, so this package stays a leaf (no hostops import).
type NameParam struct {
	Name string `json:"name"`
}
type ComposeWriteParams struct {
	Name       string `json:"name"`
	Content    string `json:"content"`
	BaseSha256 string `json:"baseSha256"`
}
type CreateStackParams struct {
	Name    string `json:"name"`
	Compose string `json:"compose,omitempty"`
}
type RenameStackParams struct {
	Name    string `json:"name"`
	NewName string `json:"newName"`
}
type DeleteStackParams struct {
	Name    string `json:"name"`
	Volumes bool   `json:"volumes"`
}
type UpdateServiceParams struct {
	Name       string `json:"name"`
	Service    string `json:"service"`
	Tag        string `json:"tag"`
	BaseSha256 string `json:"baseSha256,omitempty"`
	Confirm    bool   `json:"confirm"`
}
type RunActionParams struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Service string `json:"service,omitempty"`
}
type LogsParams struct {
	Stack string `json:"stack"`
	Tail  int    `json:"tail"`
}
type ExecParams struct {
	ContainerID string `json:"containerId"`
	Shell       string `json:"shell,omitempty"`
}
