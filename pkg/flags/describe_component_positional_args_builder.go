package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DescribeComponentPositionalArgsBuilder provides domain-specific builder for describe component command.
type DescribeComponentPositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewDescribeComponentPositionalArgsBuilder creates a new DescribeComponentPositionalArgsBuilder.
func NewDescribeComponentPositionalArgsBuilder() *DescribeComponentPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewDescribeComponentPositionalArgsBuilder")()

	return &DescribeComponentPositionalArgsBuilder{
		builder: NewPositionalArgsBuilder(),
	}
}

// WithComponent adds the component positional argument.
func (b *DescribeComponentPositionalArgsBuilder) WithComponent(required bool) *DescribeComponentPositionalArgsBuilder {
	defer perf.Track(nil, "flags.DescribeComponentPositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    required,
		TargetField: "Component",
	})

	return b
}

// Build generates the positional args configuration.
func (b *DescribeComponentPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.DescribeComponentPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
