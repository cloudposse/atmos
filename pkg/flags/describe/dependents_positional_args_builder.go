package describe

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DependentsPositionalArgsBuilder provides domain-specific builder for describe dependents command positional arguments.
//
// Features:
//   - Semantic method names (WithComponent vs generic AddArg)
//   - Auto-configures TargetField mapping to StandardOptions.Component
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	specs, validator, usage := describe.NewDependentsPositionalArgsBuilder().
//	    WithComponent(true).  // Component is required
//	    Build()
//
//	parser := flags.NewStandardOptionsBuilder().
//	    WithPositionalArgs(specs, validator, usage).
//	    Build()
type DependentsPositionalArgsBuilder struct {
	builder *flags.PositionalArgsBuilder
}

// NewDependentsPositionalArgsBuilder creates a new DependentsPositionalArgsBuilder.
func NewDependentsPositionalArgsBuilder() *DependentsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewDependentsPositionalArgsBuilder")()

	return &DependentsPositionalArgsBuilder{
		builder: flags.NewPositionalArgsBuilder(),
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
func (b *DependentsPositionalArgsBuilder) WithComponent(required bool) *DependentsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.DependentsPositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&flags.PositionalArgSpec{
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
func (b *DependentsPositionalArgsBuilder) Build() ([]*flags.PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.DependentsPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
