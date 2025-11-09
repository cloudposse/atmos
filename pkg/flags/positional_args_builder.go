package flags

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// PositionalArgSpec defines a single positional argument specification.
// This is used by builders to configure positional argument parsing.
//
// Example:
//
//	spec := &PositionalArgSpec{
//	    Name:        "component",
//	    Description: "Component name",
//	    Required:    true,
//	    TargetField: "Component", // Field name in options struct (e.g., TerraformOptions.Component)
//	}
type PositionalArgSpec struct {
	Name        string // Argument name (e.g., "component", "workflow")
	Description string // Human-readable description for usage/help
	Required    bool   // Whether this argument is required
	TargetField string // Name of field in Options struct to populate (e.g., "Component")
}

// PositionalArgsBuilder provides low-level builder pattern for positional arguments.
// This is the foundation for domain-specific builders like TerraformPositionalArgsBuilder.
//
// Features:
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string (e.g., "<component>" or "[component]")
//   - Maps positional args to struct fields via TargetField
//   - Type-safe extraction without manual array indexing
//
// Usage:
//
//	builder := flags.NewPositionalArgsBuilder()
//	builder.AddArg(&flags.PositionalArgSpec{
//	    Name:        "component",
//	    Description: "Component name",
//	    Required:    true,
//	    TargetField: "Component", // Maps to TerraformOptions.Component field
//	})
//	specs, validator, usage := builder.Build()
type PositionalArgsBuilder struct {
	specs []*PositionalArgSpec
}

// NewPositionalArgsBuilder creates a new PositionalArgsBuilder.
func NewPositionalArgsBuilder() *PositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewPositionalArgsBuilder")()

	return &PositionalArgsBuilder{
		specs: make([]*PositionalArgSpec, 0),
	}
}

// AddArg adds a positional argument specification to the builder.
//
// Example:
//
//	builder.AddArg(&flags.PositionalArgSpec{
//	    Name:        "component",
//	    Description: "Component name",
//	    Required:    true,
//	    TargetField: "Component",
//	})
func (b *PositionalArgsBuilder) AddArg(spec *PositionalArgSpec) *PositionalArgsBuilder {
	defer perf.Track(nil, "flags.PositionalArgsBuilder.AddArg")()

	b.specs = append(b.specs, spec)
	return b
}

// Build generates the positional args configuration.
//
// Returns:
//   - specs: Array of positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function (validates required/optional args)
//   - usage: Usage string for Cobra Use field (e.g., "<component>" or "[workflow]")
//
// Example:
//
//	specs, validator, usage := builder.Build()
//	cmd.Use = "deploy " + usage   // "deploy <component>"
//	cmd.Args = validator           // Validates component is provided
func (b *PositionalArgsBuilder) Build() (specs []*PositionalArgSpec, validator cobra.PositionalArgs, usage string) {
	defer perf.Track(nil, "flags.PositionalArgsBuilder.Build")()

	// Generate usage string (e.g., "<component>" or "[workflow]")
	usage = b.generateUsage()

	// Generate validator function
	validator = b.generateValidator()

	return b.specs, validator, usage
}

// generateUsage generates a usage string for Cobra Use field.
// Required args: <name>, Optional args: [name]
//
// Examples:
//   - Single required: "<component>"
//   - Single optional: "[workflow]"
//   - Multiple: "<component> [stack]"
func (b *PositionalArgsBuilder) generateUsage() string {
	defer perf.Track(nil, "flags.PositionalArgsBuilder.generateUsage")()

	if len(b.specs) == 0 {
		return ""
	}

	usage := ""
	for i, spec := range b.specs {
		if i > 0 {
			usage += " "
		}

		if spec.Required {
			usage += fmt.Sprintf("<%s>", spec.Name)
		} else {
			usage += fmt.Sprintf("[%s]", spec.Name)
		}
	}

	return usage
}

// generateValidator creates a Cobra PositionalArgs validator function.
// This validates the number of positional args based on required/optional specs.
//
// Logic:
//   - Counts required args (minimum)
//   - Counts total args (maximum)
//   - Returns validator that checks: minArgs <= provided <= maxArgs
func (b *PositionalArgsBuilder) generateValidator() cobra.PositionalArgs {
	defer perf.Track(nil, "flags.PositionalArgsBuilder.generateValidator")()

	if len(b.specs) == 0 {
		// No positional args defined - accept any args
		return cobra.ArbitraryArgs
	}

	// Count required args (minimum required)
	requiredCount := 0
	for _, spec := range b.specs {
		if spec.Required {
			requiredCount++
		}
	}

	totalCount := len(b.specs)

	// Generate validator based on required/optional args
	if requiredCount == totalCount {
		// All args required - use exact count validator
		return cobra.ExactArgs(requiredCount)
	}

	if requiredCount == 0 {
		// All optional - use maximum count validator
		return cobra.MaximumNArgs(totalCount)
	}

	// Mixed required/optional - use range validator
	return cobra.RangeArgs(requiredCount, totalCount)
}
