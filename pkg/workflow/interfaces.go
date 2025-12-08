package workflow

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// AtmosExecParams holds parameters for executing an atmos command.
type AtmosExecParams struct {
	// Ctx is the context for cancellation.
	Ctx context.Context
	// AtmosConfig is the atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
	// Args are command arguments (e.g., ["terraform", "plan", "vpc"]).
	Args []string
	// Dir is the working directory for the command.
	Dir string
	// Env are environment variables for the command.
	Env []string
	// DryRun if true, don't actually execute the command.
	DryRun bool
}

// CommandRunner abstracts the execution of shell and atmos commands.
// This interface enables testing workflow logic without spawning real processes.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=interfaces.go -destination=mock_interfaces_test.go -package=workflow
type CommandRunner interface {
	// RunShell executes a shell command with the given parameters.
	// Parameters:
	//   - command: The shell command to execute
	//   - name: A name for the command (for logging/identification)
	//   - dir: Working directory for the command
	//   - env: Environment variables for the command
	//   - dryRun: If true, don't actually execute the command
	// Returns an error if the command fails.
	RunShell(command, name, dir string, env []string, dryRun bool) error

	// RunAtmos executes an atmos command with the given parameters.
	// Returns an error if the command fails.
	RunAtmos(params *AtmosExecParams) error
}

// AuthProvider abstracts authentication operations for workflow steps.
// This interface enables testing identity-based workflows without real auth providers.
type AuthProvider interface {
	// NeedsAuth returns true if authentication is needed for the given steps.
	NeedsAuth(steps []schema.WorkflowStep, commandLineIdentity string) bool

	// Authenticate performs authentication for the given identity.
	// Returns an error if authentication fails.
	Authenticate(ctx context.Context, identity string) error

	// GetCachedCredentials returns cached credentials for the identity.
	// Returns an error if no valid cached credentials are available.
	GetCachedCredentials(ctx context.Context, identity string) (any, error)

	// PrepareEnvironment prepares environment variables for the authenticated identity.
	// Returns the modified environment slice.
	PrepareEnvironment(ctx context.Context, identity string, baseEnv []string) ([]string, error)
}

// WorkflowLoader abstracts loading and parsing workflow definitions.
// This interface enables testing without file system access.
type WorkflowLoader interface {
	// LoadWorkflow loads a workflow definition from the given path.
	// Parameters:
	//   - atmosConfig: The atmos configuration
	//   - workflowPath: Path to the workflow file
	//   - workflowName: Name of the workflow to load
	// Returns the workflow definition and any error.
	LoadWorkflow(atmosConfig schema.AtmosConfiguration, workflowPath, workflowName string) (*schema.WorkflowDefinition, error)

	// ListWorkflows returns all available workflows.
	ListWorkflows(atmosConfig schema.AtmosConfiguration) ([]schema.DescribeWorkflowsItem, error)
}

// UIProvider abstracts user interface operations for workflows.
// This interface enables testing without terminal interaction.
type UIProvider interface {
	// PrintMessage prints a message to the user.
	PrintMessage(format string, args ...any)

	// PrintError prints an error message to the user.
	PrintError(err error, title, explanation string)
}

// ExecuteOptions contains options for workflow execution.
type ExecuteOptions struct {
	// DryRun if true, commands are not actually executed.
	DryRun bool

	// CommandLineStack overrides the stack for all steps.
	CommandLineStack string

	// FromStep skips steps until this step name is reached.
	FromStep string

	// CommandLineIdentity sets the identity for steps without explicit identity.
	CommandLineIdentity string
}

// WorkflowParams contains parameters for workflow execution.
type WorkflowParams struct {
	// Ctx is the context for cancellation.
	Ctx context.Context
	// AtmosConfig is the atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
	// Workflow is the name of the workflow.
	Workflow string
	// WorkflowPath is the path to the workflow file.
	WorkflowPath string
	// WorkflowDefinition is the parsed workflow definition.
	WorkflowDefinition *schema.WorkflowDefinition
	// Opts are the execution options.
	Opts ExecuteOptions
}

// StepResult represents the result of executing a single workflow step.
type StepResult struct {
	// StepName is the name of the step.
	StepName string

	// Command is the command that was executed.
	Command string

	// Success indicates whether the step succeeded.
	Success bool

	// Error is the error if the step failed.
	Error error

	// Skipped indicates if the step was skipped (e.g., due to --from-step).
	Skipped bool
}

// ExecutionResult represents the result of executing a complete workflow.
type ExecutionResult struct {
	// WorkflowName is the name of the workflow.
	WorkflowName string

	// Steps contains results for each step.
	Steps []StepResult

	// Success indicates whether all steps succeeded.
	Success bool

	// Error is the first error encountered, if any.
	Error error

	// ResumeCommand is the command to resume from the failed step.
	ResumeCommand string
}
