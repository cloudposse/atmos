package exec

import (
	"context"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/workflow"
)

// Compile-time interface compliance checks.
var (
	_ workflow.CommandRunner = (*WorkflowCommandRunner)(nil)
	_ workflow.AuthProvider  = (*WorkflowAuthProvider)(nil)
	_ workflow.UIProvider    = (*WorkflowUIProvider)(nil)
)

// WorkflowCommandRunner implements workflow.CommandRunner using existing exec functions.
type WorkflowCommandRunner struct{}

// NewWorkflowCommandRunner creates a new WorkflowCommandRunner.
func NewWorkflowCommandRunner(retryConfig *schema.RetryConfig) *WorkflowCommandRunner {
	defer perf.Track(nil, "exec.NewWorkflowCommandRunner")()

	return &WorkflowCommandRunner{}
}

// RunShell executes a shell command using ExecuteShell.
func (r *WorkflowCommandRunner) RunShell(command, name, dir string, env []string, dryRun bool) error {
	defer perf.Track(nil, "exec.WorkflowCommandRunner.RunShell")()

	return ExecuteShell(command, name, dir, env, dryRun)
}

// RunAtmos executes an atmos command using ExecuteShellCommand.
// Note: Retry logic should be handled at a higher level if needed.
func (r *WorkflowCommandRunner) RunAtmos(params *workflow.AtmosExecParams) error {
	defer perf.Track(params.AtmosConfig, "exec.WorkflowCommandRunner.RunAtmos")()

	return ExecuteShellCommand(*params.AtmosConfig, "atmos", params.Args, params.Dir, params.Env, params.DryRun, "")
}

// WorkflowAuthProvider implements workflow.AuthProvider using auth.AuthManager.
type WorkflowAuthProvider struct {
	manager auth.AuthManager
}

// NewWorkflowAuthProvider creates a new WorkflowAuthProvider with the given auth manager.
func NewWorkflowAuthProvider(manager auth.AuthManager) *WorkflowAuthProvider {
	defer perf.Track(nil, "exec.NewWorkflowAuthProvider")()

	return &WorkflowAuthProvider{
		manager: manager,
	}
}

// NeedsAuth returns true if authentication is needed for the given steps.
func (p *WorkflowAuthProvider) NeedsAuth(steps []schema.WorkflowStep, commandLineIdentity string) bool {
	defer perf.Track(nil, "exec.WorkflowAuthProvider.NeedsAuth")()

	if commandLineIdentity != "" {
		return true
	}

	for _, step := range steps {
		if step.Identity != "" {
			return true
		}
	}

	return false
}

// Authenticate performs authentication for the given identity.
func (p *WorkflowAuthProvider) Authenticate(ctx context.Context, identity string) error {
	defer perf.Track(nil, "exec.WorkflowAuthProvider.Authenticate")()

	_, err := p.manager.Authenticate(ctx, identity)
	return err
}

// GetCachedCredentials returns cached credentials for the identity.
func (p *WorkflowAuthProvider) GetCachedCredentials(ctx context.Context, identity string) (any, error) {
	defer perf.Track(nil, "exec.WorkflowAuthProvider.GetCachedCredentials")()

	return p.manager.GetCachedCredentials(ctx, identity)
}

// PrepareEnvironment prepares environment variables for the authenticated identity.
func (p *WorkflowAuthProvider) PrepareEnvironment(ctx context.Context, identity string, baseEnv []string) ([]string, error) {
	defer perf.Track(nil, "exec.WorkflowAuthProvider.PrepareEnvironment")()

	// If no base env provided, use current OS environment.
	if baseEnv == nil {
		baseEnv = os.Environ()
	}

	return p.manager.PrepareShellEnvironment(ctx, identity, baseEnv)
}

// WorkflowUIProvider implements workflow.UIProvider using TUI utilities.
type WorkflowUIProvider struct{}

// NewWorkflowUIProvider creates a new WorkflowUIProvider.
func NewWorkflowUIProvider() *WorkflowUIProvider {
	defer perf.Track(nil, "exec.NewWorkflowUIProvider")()

	return &WorkflowUIProvider{}
}

// PrintMessage prints a message to the TUI.
func (p *WorkflowUIProvider) PrintMessage(format string, args ...any) {
	defer perf.Track(nil, "exec.WorkflowUIProvider.PrintMessage")()

	u.PrintfMessageToTUI(format, args...)
}

// PrintError prints an error using the error utilities.
func (p *WorkflowUIProvider) PrintError(err error, title, explanation string) {
	defer perf.Track(nil, "exec.WorkflowUIProvider.PrintError")()

	errUtils.CheckErrorAndPrint(err, title, explanation)
}
