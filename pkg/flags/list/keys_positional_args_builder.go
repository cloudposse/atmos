package list

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ListKeysPositionalArgsBuilder provides domain-specific builder for list keys command positional arguments.
//
// Features:
//   - Semantic method names (WithComponent vs generic AddArg)
//   - Auto-configures TargetField mapping to StandardOptions.Component
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	specs, validator, usage := flags.NewListKeysPositionalArgsBuilder().
//	    WithComponent(false).  // Component is optional
//	    Build()
//
//	parser := flags.NewStandardOptionsBuilder().
//	    WithPositionalArgs(specs, validator, usage).
//	    Build()
type ListKeysPositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewListKeysPositionalArgsBuilder creates a new ListKeysPositionalArgsBuilder.
func NewListKeysPositionalArgsBuilder() *ListKeysPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewListKeysPositionalArgsBuilder")()

	return &ListKeysPositionalArgsBuilder{
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
//	builder.WithComponent(false) // [component] - optional
func (b *ListKeysPositionalArgsBuilder) WithComponent(required bool) *ListKeysPositionalArgsBuilder {
	defer perf.Track(nil, "flags.ListKeysPositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name to filter keys",
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
//   - usage: Usage string for Cobra Use field (e.g., "[component]")
func (b *ListKeysPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.ListKeysPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
