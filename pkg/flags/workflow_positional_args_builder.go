package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkflowPositionalArgsBuilder provides domain-specific builder for Workflow command positional arguments.
// This builder configures the workflow name argument for the workflow command.
//
// Features:
//   - Semantic method names (WithWorkflowName vs generic AddArg)
//   - Auto-configures TargetField mapping to WorkflowOptions.WorkflowName
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	// Define positional args for workflow command
//	_, workflowValidator, workflowUsage := flags.NewWorkflowPositionalArgsBuilder().
//	    WithWorkflowName(true).  // Workflow name is required
//	    Build()
//
//	workflowCmd := &cobra.Command{
//	    Use:   "workflow " + workflowUsage,  // Auto-generates: "workflow <name>"
//	    Short: "Execute a workflow",
//	    Args:  workflowValidator,             // Auto-configured validator
//	    RunE: func(cmd *cobra.Command, args []string) error {
//	        // Parse flags AND positional args
//	        opts, err := parser.Parse(ctx, args)
//	        if err != nil {
//	            return err
//	        }
//
//	        // Access workflow name via dot notation - just like flags!
//	        fmt.Printf("Executing workflow: %s\n", opts.WorkflowName)
//	        return nil
//	    },
//	}
//
// See docs/prd/flag-handling/type-safe-positional-arguments.md for full pattern.
type WorkflowPositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewWorkflowPositionalArgsBuilder creates a new WorkflowPositionalArgsBuilder.
func NewWorkflowPositionalArgsBuilder() *WorkflowPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewWorkflowPositionalArgsBuilder")()

	return &WorkflowPositionalArgsBuilder{
		builder: NewPositionalArgsBuilder(),
	}
}

// WithWorkflowName adds the workflow name positional argument.
// This maps to WorkflowOptions.WorkflowName field.
//
// Parameters:
//   - required: Whether workflow name argument is required
//
// Example:
//
//	builder.WithWorkflowName(true)  // <name> - required
//	builder.WithWorkflowName(false) // [name] - optional
func (b *WorkflowPositionalArgsBuilder) WithWorkflowName(required bool) *WorkflowPositionalArgsBuilder {
	defer perf.Track(nil, "flags.WorkflowPositionalArgsBuilder.WithWorkflowName")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "name",
		Description: "Workflow name",
		Required:    required,
		TargetField: "WorkflowName", // Maps to WorkflowOptions.WorkflowName field
	})

	return b
}

// Build generates the positional args configuration.
//
// Returns:
//   - specs: Array of positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function
//   - usage: Usage string for Cobra Use field (e.g., "<name>")
//
// Example:
//
//	specs, validator, usage := builder.Build()
//	cmd.Use = "workflow " + usage   // "workflow <name>"
//	cmd.Args = validator             // Validates workflow name is provided
func (b *WorkflowPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.WorkflowPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
