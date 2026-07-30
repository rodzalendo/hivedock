package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rogalinski/hivedock/internal/agentrpc"
	"github.com/rogalinski/hivedock/internal/hostops"
	"github.com/rogalinski/hivedock/internal/stacks"
)

// remoteCallTimeout bounds a unary RPC to an agent. Streaming ops (deploy/logs)
// are not bounded here — they live as long as the caller's context.
const remoteCallTimeout = 30 * time.Second

// remoteBackend implements hostops.Backend by translating each call into an RPC
// to a connected agent (docs/MULTIHOST.md). Results are decoded against the same
// hostops types the LocalBackend returns, and a failure's machine Code is
// re-classified into the same typed error a local failure would produce — so a
// remote stack behaves identically to a local one, including status codes and the
// 409-conflict reconcile payload.
type remoteBackend struct {
	h *hostConn
}

func newRemoteBackend(h *hostConn) *remoteBackend { return &remoteBackend{h: h} }

// call runs a unary RPC: send method+params, wait for the terminal reply, then
// either decode Result into out or reconstruct the typed error from Code/Result.
// A transport failure (socket gone, deadline) surfaces as ErrOffline so the host
// reads as offline rather than a generic 500.
func (b *remoteBackend) call(ctx context.Context, method string, params, out any) error {
	ctx, cancel := context.WithTimeout(ctx, remoteCallTimeout)
	defer cancel()
	resp, err := b.h.Call(ctx, method, params)
	if err != nil {
		return hostops.ErrOffline
	}
	if resp.Code != "" || resp.Error != "" {
		return hostops.ErrorForCode(resp.Code, resp.Error, resp.Result)
	}
	if out != nil && len(resp.Result) > 0 {
		return json.Unmarshal(resp.Result, out)
	}
	return nil
}

func (b *remoteBackend) ListStacks(ctx context.Context) ([]stacks.Stack, error) {
	var out []stacks.Stack
	err := b.call(ctx, agentrpc.MethodListStacks, nil, &out)
	return out, err
}

func (b *remoteBackend) GetStack(ctx context.Context, name string) (stacks.Stack, error) {
	var out stacks.Stack
	err := b.call(ctx, agentrpc.MethodGetStack, agentrpc.NameParam{Name: name}, &out)
	return out, err
}

func (b *remoteBackend) GetCompose(ctx context.Context, name string) (hostops.ComposeFile, error) {
	var out hostops.ComposeFile
	err := b.call(ctx, agentrpc.MethodGetCompose, agentrpc.NameParam{Name: name}, &out)
	return out, err
}

func (b *remoteBackend) ValidateCompose(ctx context.Context, name string, content []byte) (hostops.Validation, error) {
	var out hostops.Validation
	err := b.call(ctx, agentrpc.MethodValidateCompose, agentrpc.ComposeWriteParams{Name: name, Content: string(content)}, &out)
	return out, err
}

func (b *remoteBackend) GetEnv(ctx context.Context, name string) (hostops.EnvFile, error) {
	var out hostops.EnvFile
	err := b.call(ctx, agentrpc.MethodGetEnv, agentrpc.NameParam{Name: name}, &out)
	return out, err
}

func (b *remoteBackend) PutCompose(ctx context.Context, name string, content []byte, baseSha string) (hostops.ComposeFile, error) {
	var out hostops.ComposeFile
	err := b.call(ctx, agentrpc.MethodPutCompose, agentrpc.ComposeWriteParams{Name: name, Content: string(content), BaseSha256: baseSha}, &out)
	return out, err
}

func (b *remoteBackend) PutEnv(ctx context.Context, name string, content []byte, baseSha string) (hostops.EnvFile, error) {
	var out hostops.EnvFile
	err := b.call(ctx, agentrpc.MethodPutEnv, agentrpc.ComposeWriteParams{Name: name, Content: string(content), BaseSha256: baseSha}, &out)
	return out, err
}

func (b *remoteBackend) CreateStack(ctx context.Context, name, composeYAML string) (hostops.StackRef, error) {
	var out hostops.StackRef
	err := b.call(ctx, agentrpc.MethodCreateStack, agentrpc.CreateStackParams{Name: name, Compose: composeYAML}, &out)
	return out, err
}

func (b *remoteBackend) DeleteStack(ctx context.Context, name string, volumes bool) error {
	return b.call(ctx, agentrpc.MethodDeleteStack, agentrpc.DeleteStackParams{Name: name, Volumes: volumes}, nil)
}

func (b *remoteBackend) RenameStack(ctx context.Context, name, newName string) (hostops.StackRef, error) {
	var out hostops.StackRef
	err := b.call(ctx, agentrpc.MethodRenameStack, agentrpc.RenameStackParams{Name: name, NewName: newName}, &out)
	return out, err
}

func (b *remoteBackend) UpdateService(ctx context.Context, req hostops.UpdateServiceReq) (hostops.UpdateServiceResult, error) {
	var out hostops.UpdateServiceResult
	err := b.call(ctx, agentrpc.MethodUpdateService, agentrpc.UpdateServiceParams{
		Name: req.Name, Service: req.Service, Tag: req.Tag, BaseSha256: req.BaseSha256, Confirm: req.Confirm,
	}, &out)
	return out, err
}

// RunAction streams deploy output: each chunk frame is one output line; the
// terminal frame carries success or a typed error. The caller holds the lock.
func (b *remoteBackend) RunAction(ctx context.Context, name, action, service string, onLine func(string)) error {
	ch, _, err := b.h.CallStream(ctx, agentrpc.MethodRunAction, agentrpc.RunActionParams{Name: name, Action: action, Service: service})
	if err != nil {
		return hostops.ErrOffline
	}
	for resp := range ch {
		if resp.Kind == agentrpc.KindStream {
			var line string
			if json.Unmarshal(resp.Result, &line) == nil {
				onLine(line)
			}
			continue
		}
		if resp.Code != "" || resp.Error != "" {
			return hostops.ErrorForCode(resp.Code, resp.Error, resp.Result)
		}
		return nil
	}
	return nil
}

// Logs streams container log lines until ctx is cancelled (which sends a cancel
// to the agent so it stops its follow) or the stack's containers stop.
func (b *remoteBackend) Logs(ctx context.Context, name string, tail int, onLine func(hostops.LogLine)) error {
	ch, _, err := b.h.CallStream(ctx, agentrpc.MethodLogs, agentrpc.LogsParams{Stack: name, Tail: tail})
	if err != nil {
		return hostops.ErrOffline
	}
	for resp := range ch {
		if resp.Kind == agentrpc.KindStream {
			var ll hostops.LogLine
			if json.Unmarshal(resp.Result, &ll) == nil {
				onLine(ll)
			}
			continue
		}
		if resp.Code != "" || resp.Error != "" {
			return hostops.ErrorForCode(resp.Code, resp.Error, resp.Result)
		}
		return nil
	}
	return nil
}
