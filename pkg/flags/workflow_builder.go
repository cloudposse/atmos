package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkflowOptionsBuilder provides a type-safe, fluent interface for building WorkflowParser
// with strongly-typed flag definitions that map directly to WorkflowOptions fields.
//
// Example:
//
//	parser := flags.NewWorkflowOptionsBuilder().
//	    WithFile(true).         // Required file flag → .File field
//	    WithDryRun().           // Dry-run flag → .DryRun field
//	    WithFromStep().         // From-step flag → .FromStep field
//	    Build()
type WorkflowOptionsBuilder struct {
	options []Option
}

// NewWorkflowOptionsBuilder creates a new builder for WorkflowParser.
func NewWorkflowOptionsBuilder() *WorkflowOptionsBuilder {
	defer perf.Track(nil, "flags.NewWorkflowOptionsBuilder")()

	return &WorkflowOptionsBuilder{
		options: []Option{},
	}
}

// WithFile adds the file flag for specifying workflow file.
// Maps to WorkflowOptions.File field (inherited from StandardOptions).
//
// Parameters:
//   - required: if true, flag is marked as required
func (b *WorkflowOptionsBuilder) WithFile(required bool) *WorkflowOptionsBuilder {
	defer perf.Track(nil, "flags.WorkflowOptionsBuilder.WithFile")()

	if required {
		b.options = append(b.options, WithRequiredStringFlag("file", "f", "Specify the workflow file to run"))
	} else {
		b.options = append(b.options, WithStringFlag("file", "f", "", "Specify the workflow file to run"))
	}
	b.options = append(b.options, WithEnvVars("file", "ATMOS_WORKFLOW_FILE"))
	return b
}

// WithDryRun adds the dry-run flag.
// Maps to WorkflowOptions.DryRun field (inherited from StandardOptions).
func (b *WorkflowOptionsBuilder) WithDryRun() *WorkflowOptionsBuilder {
	defer perf.Track(nil, "flags.WorkflowOptionsBuilder.WithDryRun")()

	b.options = append(b.options, WithBoolFlag("dry-run", "", false, "Simulate the workflow without making any changes"))
	b.options = append(b.options, WithEnvVars("dry-run", "ATMOS_WORKFLOW_DRY_RUN"))
	return b
}

// WithFromStep adds the from-step flag for resuming workflows.
// Maps to WorkflowOptions.FromStep field.
func (b *WorkflowOptionsBuilder) WithFromStep() *WorkflowOptionsBuilder {
	defer perf.Track(nil, "flags.WorkflowOptionsBuilder.WithFromStep")()

	b.options = append(b.options, WithStringFlag("from-step", "", "", "Resume the workflow from the specified step"))
	b.options = append(b.options, WithEnvVars("from-step", "ATMOS_WORKFLOW_FROM_STEP"))
	return b
}

// WithStack adds the stack flag.
// Maps to WorkflowOptions.Stack field (inherited from StandardOptions).
//
// Parameters:
//   - required: if true, flag is marked as required
func (b *WorkflowOptionsBuilder) WithStack(required bool) *WorkflowOptionsBuilder {
	defer perf.Track(nil, "flags.WorkflowOptionsBuilder.WithStack")()

	if required {
		b.options = append(b.options, WithRequiredStringFlag("stack", "s", "Atmos stack"))
	} else {
		b.options = append(b.options, WithStringFlag("stack", "s", "", "Atmos stack"))
	}
	b.options = append(b.options, WithEnvVars("stack", "ATMOS_STACK"))
	return b
}

// Build creates the WorkflowParser with all configured flags.
// Returns a parser ready for RegisterFlags() and Parse() operations.
func (b *WorkflowOptionsBuilder) Build() *WorkflowParser {
	defer perf.Track(nil, "flags.WorkflowOptionsBuilder.Build")()

	return NewWorkflowParser(b.options...)
}

// WithIdentity adds the identity flag for authentication.
// Maps to WorkflowOptions.Identity field (inherited from GlobalFlags).
func (b *WorkflowOptionsBuilder) WithIdentity() *WorkflowOptionsBuilder {
	defer perf.Track(nil, "flags.WorkflowOptionsBuilder.WithIdentity")()

	b.options = append(b.options, WithIdentityFlag())
	return b
}
