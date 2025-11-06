package list

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ListComponentsPositionalArgsBuilder provides domain-specific builder for list components command positional arguments.
//
// Features:
//   - Semantic method names (WithKey vs generic AddArg)
//   - Auto-configures TargetField mapping to StandardOptions.Key
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	specs, validator, usage := flags.NewListComponentsPositionalArgsBuilder().
//	    WithKey(false).  // Key is optional
//	    Build()
//
//	parser := flags.NewStandardOptionsBuilder().
//	    WithPositionalArgs(specs, validator, usage).
//	    Build()
type ListComponentsPositionalArgsBuilder struct {
	builder *flags.PositionalArgsBuilder
}

// NewListComponentsPositionalArgsBuilder creates a new ListComponentsPositionalArgsBuilder.
func NewListComponentsPositionalArgsBuilder() *ListComponentsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewListComponentsPositionalArgsBuilder")()

	return &ListComponentsPositionalArgsBuilder{
		builder: flags.NewPositionalArgsBuilder(),
	}
}

// WithKey adds the key positional argument.
// This maps to StandardOptions.Key field.
//
// Parameters:
//   - required: Whether key argument is required
//
// Example:
//
//	builder.WithKey(false) // [key] - optional
func (b *ListComponentsPositionalArgsBuilder) WithKey(required bool) *ListComponentsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.ListComponentsPositionalArgsBuilder.WithKey")()

	b.builder.AddArg(&flags.PositionalArgSpec{
		Name:        "key",
		Description: "Configuration key to filter components",
		Required:    required,
		TargetField: "Key", // Maps to StandardOptions.Key field
	})

	return b
}

// Build generates the positional args configuration.
//
// Returns:
//   - specs: Array of positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function
//   - usage: Usage string for Cobra Use field (e.g., "[key]")
func (b *ListComponentsPositionalArgsBuilder) Build() ([]*flags.PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.ListComponentsPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
