package packer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PositionalArgsBuilder provides domain-specific builder for Packer command positional arguments.
// This builder configures the component name argument for packer commands like build, init, inspect, validate.
//
// Features:
//   - Semantic method names (WithComponent vs generic AddArg)
//   - Auto-configures TargetField mapping to PackerOptions.Component
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	// Define positional args for build command
//	_, buildValidator, buildUsage := packer.NewPositionalArgsBuilder().
//	    WithComponent(true).  // Component is required
//	    Build()
//
//	commands := []*cobra.Command{
//	    {
//	        Use:   "build " + buildUsage,  // Auto-generates: "build <component>"
//	        Short: "Build the specified Packer image",
//	        Args:  buildValidator,          // Auto-configured validator
//	        RunE: func(cmd *cobra.Command, args []string) error {
//	            // Parse flags AND positional args
//	            opts, err := parser.Parse(ctx, args)
//	            if err != nil {
//	                return err
//	            }
//
//	            // Access component via dot notation - just like flags!
//	            fmt.Printf("Building component: %s\n", opts.Component)
//	            return nil
//	        },
//	    },
//	}
//
// See docs/prd/flag-handling/type-safe-positional-arguments.md for full pattern.
type PositionalArgsBuilder struct {
	builder *flags.PositionalArgsBuilder
}

// NewPositionalArgsBuilder creates a new PositionalArgsBuilder.
func NewPositionalArgsBuilder() *PositionalArgsBuilder {
	defer perf.Track(nil, "packer.NewPositionalArgsBuilder")()

	return &PositionalArgsBuilder{
		builder: flags.NewPositionalArgsBuilder(),
	}
}

// WithComponent adds the component positional argument.
// This maps to Options.Component field.
//
// Parameters:
//   - required: Whether component argument is required
//
// Example:
//
//	builder.WithComponent(true)  // <component> - required
//	builder.WithComponent(false) // [component] - optional
func (b *PositionalArgsBuilder) WithComponent(required bool) *PositionalArgsBuilder {
	defer perf.Track(nil, "packer.PositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&flags.PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    required,
		TargetField: "Component", // Maps to Options.Component field
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
//	cmd.Use = "build " + usage   // "build <component>"
//	cmd.Args = validator          // Validates component is provided
func (b *PositionalArgsBuilder) Build() ([]*flags.PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "packer.PositionalArgsBuilder.Build")()

	return b.builder.Build()
}
