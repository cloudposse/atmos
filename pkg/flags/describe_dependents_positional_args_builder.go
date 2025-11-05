package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DescribeDependentsPositionalArgsBuilder provides domain-specific builder for describe dependents command positional arguments.
//
// Features:
//   - Semantic method names (WithComponent vs generic AddArg)
//   - Auto-configures TargetField mapping to StandardOptions.Component
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	specs, validator, usage := flags.NewDescribeDependentsPositionalArgsBuilder().
//	    WithComponent(true).  // Component is required
//	    Build()
//
//	parser := flags.NewStandardOptionsBuilder().
//	    WithPositionalArgs(specs, validator, usage).
//	    Build()
type DescribeDependentsPositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewDescribeDependentsPositionalArgsBuilder creates a new DescribeDependentsPositionalArgsBuilder.
func NewDescribeDependentsPositionalArgsBuilder() *DescribeDependentsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewDescribeDependentsPositionalArgsBuilder")()

	return &DescribeDependentsPositionalArgsBuilder{
		builder: NewPositionalArgsBuilder(),
	}
}

// WithComponent adds the component positional argument.
// This maps to StandardOptions.Component field.
//
// Parameters:
//   - required: Whether component argument is required
//
// Example:
//
//	builder.WithComponent(true) // <component> - required
func (b *DescribeDependentsPositionalArgsBuilder) WithComponent(required bool) *DescribeDependentsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.DescribeDependentsPositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name to find dependents for",
		Required:    required,
		TargetField: "Component", // Maps to StandardOptions.Component field
	})

	return b
}

// Build generates the positional args configuration.
//
// Returns:
//   - specs: Array of positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function
//   - usage: Usage string for Cobra Use field (e.g., "<component>")
func (b *DescribeDependentsPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.DescribeDependentsPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
