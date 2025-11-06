package list

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ListSettingsPositionalArgsBuilder provides domain-specific builder for list settings command positional arguments.
//
// Features:
//   - Semantic method names (WithComponent vs generic AddArg)
//   - Auto-configures TargetField mapping to StandardOptions.Component
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	specs, validator, usage := flags.NewListSettingsPositionalArgsBuilder().
//	    WithComponent(false).  // Component is optional
//	    Build()
//
//	parser := flags.NewStandardOptionsBuilder().
//	    WithPositionalArgs(specs, validator, usage).
//	    Build()
type ListSettingsPositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewListSettingsPositionalArgsBuilder creates a new ListSettingsPositionalArgsBuilder.
func NewListSettingsPositionalArgsBuilder() *ListSettingsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewListSettingsPositionalArgsBuilder")()

	return &ListSettingsPositionalArgsBuilder{
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
func (b *ListSettingsPositionalArgsBuilder) WithComponent(required bool) *ListSettingsPositionalArgsBuilder {
	defer perf.Track(nil, "flags.ListSettingsPositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name to filter settings",
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
func (b *ListSettingsPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.ListSettingsPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
