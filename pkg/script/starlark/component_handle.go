package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

// componentHandle is a resolved component. It exposes every attribute of the underlying struct but
// prints compactly, like the lazy ctx.component handle, instead of dumping the whole configuration.
type componentHandle struct {
	attrs *starlarkstruct.Struct
	ref   script.ComponentRef
}

var _ starlark.HasAttrs = (*componentHandle)(nil)

// componentSummary is the compact form shared by lazy and resolved component handles.
func componentSummary(ref script.ComponentRef) string {
	return fmt.Sprintf("component(name = %q, stack = %q, type = %q)", ref.Name, ref.Stack, ref.Type)
}

// String describes the component without printing its configuration.
func (c *componentHandle) String() string {
	defer perf.Track(nil, "starlark.componentHandle.String")()

	return componentSummary(c.ref)
}

// Type matches the struct a component presents.
func (c *componentHandle) Type() string {
	defer perf.Track(nil, "starlark.componentHandle.Type")()

	return "struct"
}

// Freeze freezes the underlying struct.
func (c *componentHandle) Freeze() {
	defer perf.Track(nil, "starlark.componentHandle.Freeze")()

	c.attrs.Freeze()
}

// Truth reports that a component is always truthy.
func (c *componentHandle) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.componentHandle.Truth")()

	return true
}

// Hash is unsupported, like structs holding dictionaries.
func (c *componentHandle) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.componentHandle.Hash")()

	return 0, invalidArg("unhashable type: component")
}

// Attr delegates to the underlying struct.
func (c *componentHandle) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.componentHandle.Attr")()

	return c.attrs.Attr(name)
}

// AttrNames delegates to the underlying struct.
func (c *componentHandle) AttrNames() []string {
	defer perf.Track(nil, "starlark.componentHandle.AttrNames")()

	return c.attrs.AttrNames()
}
