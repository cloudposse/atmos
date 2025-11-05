package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformPositionalArgsBuilder provides domain-specific builder for Terraform command positional arguments.
// This builder configures the component name argument for terraform commands like plan, apply, deploy.
//
// Features:
//   - Semantic method names (WithComponent vs generic AddArg)
//   - Auto-configures TargetField mapping to TerraformOptions.Component
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	// Define positional args for deploy command
//	_, deployValidator, deployUsage := flags.NewTerraformPositionalArgsBuilder().
//	    WithComponent(true).  // Component is required
//	    Build()
//
//	commands := []*cobra.Command{
//	    {
//	        Use:   "deploy " + deployUsage,  // Auto-generates: "deploy <component>"
//	        Short: "Deploy the specified infrastructure using Terraform",
//	        Args:  deployValidator,           // Auto-configured validator
//	        RunE: func(cmd *cobra.Command, args []string) error {
//	            // Parse flags AND positional args
//	            opts, err := parser.Parse(ctx, args)
//	            if err != nil {
//	                return err
//	            }
//
//	            // Access component via dot notation - just like flags!
//	            fmt.Printf("Deploying component: %s\n", opts.Component)
//	            return nil
//	        },
//	    },
//	}
//
// See docs/prd/flag-handling/type-safe-positional-arguments.md for full pattern.
type TerraformPositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewTerraformPositionalArgsBuilder creates a new TerraformPositionalArgsBuilder.
func NewTerraformPositionalArgsBuilder() *TerraformPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewTerraformPositionalArgsBuilder")()

	return &TerraformPositionalArgsBuilder{
		builder: NewPositionalArgsBuilder(),
	}
}

// WithComponent adds the component positional argument.
// This maps to TerraformOptions.Component field.
//
// Parameters:
//   - required: Whether component argument is required
//
// Example:
//
//	builder.WithComponent(true)  // <component> - required
//	builder.WithComponent(false) // [component] - optional
func (b *TerraformPositionalArgsBuilder) WithComponent(required bool) *TerraformPositionalArgsBuilder {
	defer perf.Track(nil, "flags.TerraformPositionalArgsBuilder.WithComponent")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    required,
		TargetField: "Component", // Maps to TerraformOptions.Component field
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
//	cmd.Use = "deploy " + usage   // "deploy <component>"
//	cmd.Args = validator           // Validates component is provided
func (b *TerraformPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.TerraformPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
