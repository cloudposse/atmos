package describe

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ComponentPositionalArgsBuilder provides domain-specific builder for describe component command.
type ComponentPositionalArgsBuilder struct {
	builder *flags.PositionalArgsBuilder
}

// NewComponentPositionalArgsBuilder creates a new ComponentPositionalArgsBuilder.
func NewComponentPositionalArgsBuilder() *ComponentPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewComponentPositionalArgsBuilder")()

	return &ComponentPositionalArgsBuilder{
		builder: flags.NewPositionalArgsBuilder(),
	}
}

// WithComponent adds the component positional argument.
func (b *ComponentPositionalArgsBuilder) WithComponent(required bool) *ComponentPositionalArgsBuilder {
	defer perf.Track(nil, "flags.ComponentPositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&flags.PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    required,
		TargetField: "Component",
	})

	return b
}

// Build generates the positional args configuration.
func (b *ComponentPositionalArgsBuilder) Build() ([]*flags.PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.ComponentPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
