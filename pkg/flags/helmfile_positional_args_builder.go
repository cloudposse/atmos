package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HelmfilePositionalArgsBuilder provides domain-specific builder for Helmfile command positional arguments.
// This builder configures the component name argument for helmfile commands like apply, destroy, diff, sync.
//
// Features:
//   - Semantic method names (WithComponent vs generic AddArg)
//   - Auto-configures TargetField mapping to HelmfileOptions.Component
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	// Define positional args for apply command
//	_, applyValidator, applyUsage := flags.NewHelmfilePositionalArgsBuilder().
//	    WithComponent(true).  // Component is required
//	    Build()
//
//	commands := []*cobra.Command{
//	    {
//	        Use:   "apply " + applyUsage,  // Auto-generates: "apply <component>"
//	        Short: "Apply the specified Helmfile configuration",
//	        Args:  applyValidator,          // Auto-configured validator
//	        RunE: func(cmd *cobra.Command, args []string) error {
//	            // Parse flags AND positional args
//	            opts, err := parser.Parse(ctx, args)
//	            if err != nil {
//	                return err
//	            }
//
//	            // Access component via dot notation - just like flags!
//	            fmt.Printf("Applying component: %s\n", opts.Component)
//	            return nil
//	        },
//	    },
//	}
//
// See docs/prd/flag-handling/type-safe-positional-arguments.md for full pattern.
type HelmfilePositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewHelmfilePositionalArgsBuilder creates a new HelmfilePositionalArgsBuilder.
func NewHelmfilePositionalArgsBuilder() *HelmfilePositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewHelmfilePositionalArgsBuilder")()

	return &HelmfilePositionalArgsBuilder{
		builder: NewPositionalArgsBuilder(),
	}
}

// WithComponent adds the component positional argument.
// This maps to HelmfileOptions.Component field.
//
// Parameters:
//   - required: Whether component argument is required
//
// Example:
//
//	builder.WithComponent(true)  // <component> - required
//	builder.WithComponent(false) // [component] - optional
func (b *HelmfilePositionalArgsBuilder) WithComponent(required bool) *HelmfilePositionalArgsBuilder {
	defer perf.Track(nil, "flags.HelmfilePositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    required,
		TargetField: "Component", // Maps to HelmfileOptions.Component field
	})

	return b
}

// Build generates the positional args configuration.
//
// Returns:
//   - specs: Array of positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function
//   - usage: Usage string for Cobra Use field (e.g., "<component>")
//
// Example:
//
//	specs, validator, usage := builder.Build()
//	cmd.Use = "apply " + usage   // "apply <component>"
//	cmd.Args = validator          // Validates component is provided
func (b *HelmfilePositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.HelmfilePositionalArgsBuilder.Build")()

	return b.builder.Build()
}
