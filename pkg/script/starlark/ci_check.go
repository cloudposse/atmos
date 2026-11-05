package starlark

import (
	"fmt"
	"slices"
	"sync"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// checkHandle is the value returned by ci.check. It remembers the check's name so update can
// address the same check run, and tracks the state last reported. The handle is shared by every
// task that can see it, so its state is guarded by a mutex and Freeze is a no-op.
type checkHandle struct {
	s    *session
	name string

	mu    sync.Mutex
	state ci.CheckRunState
	id    int64
	url   string
}

var _ starlark.HasAttrs = (*checkHandle)(nil)

// checkAttrs lists the handle's attributes in sorted order.
var checkAttrs = []string{"id", "name", "state", "update", "url"}

// ciCheck creates a check run and returns its handle.
func (s *session) ciCheck(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, description, url string
	state := string(ci.CheckRunStatePending)
	if err := unpackCI(b, args, kwargs, "name", &name, "state?", &state, "description?", &description, "url?", &url); err != nil {
		return nil, err
	}
	if err := requireText(b, ciArgName, name); err != nil {
		return nil, err
	}
	parsed, err := checkState(b, state)
	if err != nil {
		return nil, err
	}
	rc, err := s.reporter(t).Check(threadContext(t), ci.CheckRequest{Name: name, State: parsed, Description: description, URL: url})
	if err != nil {
		return nil, ciFail("check", err)
	}
	s.ciReport(t, "check", rc)
	h := &checkHandle{s: s, name: name, state: parsed, url: url}
	h.record(rc)
	return h, nil
}

// checkState validates a check run state and lists the choices on failure.
func checkState(b *starlark.Builtin, value string) (ci.CheckRunState, error) {
	return oneOf(b, "state", value,
		ci.CheckRunStatePending, ci.CheckRunStateInProgress, ci.CheckRunStateSuccess,
		ci.CheckRunStateFailure, ci.CheckRunStateError, ci.CheckRunStateCancelled)
}

// record stores the identity the provider reported, when it reported one. Callers hold no lock.
func (h *checkHandle) record(rc ci.Receipt) {
	if rc.Check == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if rc.Check.ID != 0 {
		h.id = rc.Check.ID
	}
	if rc.Check.DetailsURL != "" {
		h.url = rc.Check.DetailsURL
	}
}

// update moves the check run to a new state. It sends the id the create returned, so providers can
// address the exact check run when several share a name.
func (h *checkHandle) update(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var state, description, url string
	if err := unpackCI(b, args, kwargs, "state", &state, "description?", &description, "url?", &url); err != nil {
		return nil, err
	}
	parsed, err := checkState(b, state)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	id := h.id
	h.mu.Unlock()
	rc, err := h.s.reporter(t).UpdateCheck(threadContext(t), ci.CheckRequest{Name: h.name, ID: id, State: parsed, Description: description, URL: url})
	if err != nil {
		return nil, ciFail("check.update", err)
	}
	h.s.ciReport(t, "check.update", rc)
	h.mu.Lock()
	h.state = parsed
	if url != "" {
		h.url = url
	}
	h.mu.Unlock()
	h.record(rc)
	return starlark.None, nil
}

// String describes the handle.
func (h *checkHandle) String() string {
	defer perf.Track(nil, "starlark.checkHandle.String")()

	h.mu.Lock()
	defer h.mu.Unlock()
	return fmt.Sprintf("check(name = %q, state = %q)", h.name, h.state)
}

// Type names the value.
func (h *checkHandle) Type() string {
	defer perf.Track(nil, "starlark.checkHandle.Type")()

	return "check"
}

// Freeze is a no-op: state changes through update stay possible after steps.parallel freezes
// the values visible to its tasks.
func (h *checkHandle) Freeze() {
	defer perf.Track(nil, "starlark.checkHandle.Freeze")()
}

// Truth reports that a check handle is always truthy.
func (h *checkHandle) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.checkHandle.Truth")()

	return true
}

// Hash is unsupported: the handle is mutable.
func (h *checkHandle) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.checkHandle.Hash")()

	return 0, invalidArg("unhashable type: check")
}

// Attr returns the handle's attributes and its update method.
func (h *checkHandle) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.checkHandle.Attr")()

	if name == "update" {
		return starlark.NewBuiltin("ci.check.update", h.update), nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	switch name {
	case "name":
		return starlark.String(h.name), nil
	case "state":
		return starlark.String(h.state), nil
	case "id":
		return starlark.MakeInt64(h.id), nil
	case "url":
		return starlark.String(h.url), nil
	}
	return nil, nil
}

// AttrNames lists the handle's attributes.
func (h *checkHandle) AttrNames() []string {
	defer perf.Track(nil, "starlark.checkHandle.AttrNames")()

	return slices.Clone(checkAttrs)
}
