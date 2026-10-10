package starlark

import (
	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

// lazyComponent is ctx.component for a command's component. It exposes the same attributes as
// the resolved component struct but resolves (through the session memo) on first attribute access.
// Starlark attribute access does not expose the calling thread, so this handle uses the
// invocation context. Tasks that need a per-task resolution deadline must use components.get,
// whose builtin receives the calling thread and its context.
type lazyComponent struct {
	s   *session
	ref script.ComponentRef
}

var _ starlark.HasAttrs = (*lazyComponent)(nil)

func (c *lazyComponent) resolved() (starlark.HasAttrs, error) {
	value, err := c.s.component(c.s.ctx, c.ref)
	if err != nil {
		return nil, err
	}
	attrs, ok := value.(starlark.HasAttrs)
	if !ok {
		return nil, invalidArg("component %q has no attributes", c.ref.Name)
	}
	return attrs, nil
}

// String describes the handle without resolving it.
func (c *lazyComponent) String() string {
	defer perf.Track(nil, "starlark.lazyComponent.String")()

	return componentSummary(c.ref)
}

// Type matches the struct a resolved component presents.
func (c *lazyComponent) Type() string {
	defer perf.Track(nil, "starlark.lazyComponent.Type")()

	return "struct"
}

// Freeze is a no-op: the handle is immutable and the resolved struct is frozen on creation.
func (c *lazyComponent) Freeze() {
	defer perf.Track(nil, "starlark.lazyComponent.Freeze")()
}

// Truth reports that a component in scope is always truthy.
func (c *lazyComponent) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.lazyComponent.Truth")()

	return true
}

// Hash is unsupported, like structs holding dictionaries.
func (c *lazyComponent) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.lazyComponent.Hash")()

	return 0, invalidArg("unhashable type: component")
}

// Attr resolves the component on first use and delegates to the resolved struct.
func (c *lazyComponent) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.lazyComponent.Attr")()

	attrs, err := c.resolved()
	if err != nil {
		return nil, err
	}
	return attrs.Attr(name)
}

// AttrNames resolves the component on first use.
func (c *lazyComponent) AttrNames() []string {
	defer perf.Track(nil, "starlark.lazyComponent.AttrNames")()

	attrs, err := c.resolved()
	if err != nil {
		return nil
	}
	return attrs.AttrNames()
}
