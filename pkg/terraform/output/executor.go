package output

//go:generate go run go.uber.org/mock/mockgen@latest -destination=mock_executor_test.go -package=output github.com/cloudposse/atmos/pkg/terraform/output TerraformRunner,WorkdirProvisioner,ComponentDescriber,StaticRemoteStateGetter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// String constants for logging.
const (
	logKeyComponent = "component"
	logKeyStack     = "stack"
	logKeyWorkspace = "workspace"
	dotSeparator    = "."
	// MaxLogValueLen is the maximum length of a value to log before truncating.
	maxLogValueLen = 100
)

// TerraformRunner abstracts terraform-exec operations for testability.
type TerraformRunner interface {
	// Init runs terraform init with the given options.
	Init(ctx context.Context, opts ...tfexec.InitOption) error
	// WorkspaceNew creates a new terraform workspace.
	WorkspaceNew(ctx context.Context, workspace string, opts ...tfexec.WorkspaceNewCmdOption) error
	// WorkspaceSelect selects an existing terraform workspace.
	WorkspaceSelect(ctx context.Context, workspace string, opts ...tfexec.WorkspaceSelectOption) error
	// Output retrieves terraform outputs.
	Output(ctx context.Context, opts ...tfexec.OutputOption) (map[string]tfexec.OutputMeta, error)
	// SetStdout sets the stdout writer.
	SetStdout(w io.Writer)
	// SetStderr sets the stderr writer.
	SetStderr(w io.Writer)
	// SetEnv sets environment variables for terraform commands.
	SetEnv(env map[string]string) error
}

// RunnerFactory creates TerraformRunner instances.
type RunnerFactory func(workdir, executable string) (TerraformRunner, error)

// DescribeComponentParams contains parameters for describing a component.
type DescribeComponentParams struct {
	AtmosConfig          *schema.AtmosConfiguration // Optional: Use provided config instead of initializing new one.
	Component            string
	Stack                string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	AuthManager          any
}

// ComponentDescriber abstracts component description to break circular dependency with internal/exec.
type ComponentDescriber interface {
	// DescribeComponent returns the component configuration sections.
	DescribeComponent(params *DescribeComponentParams) (map[string]any, error)
}

// StaticRemoteStateGetter abstracts static remote state retrieval.
type StaticRemoteStateGetter interface {
	// GetStaticRemoteStateOutputs returns static remote state outputs if configured.
	GetStaticRemoteStateOutputs(sections *map[string]any) map[string]any
}

// OutputOptions configures behavior for terraform output retrieval.
type OutputOptions struct {
	// QuietMode suppresses terraform init/workspace output (sends to io.Discard).
	// Use this when formatting output for scripts to avoid polluting stdout/stderr.
	// If an error occurs, captured stderr is included in the error message.
	QuietMode bool
	// SkipInit skips terraform init, workspace select, and workspace cleanup.
	// Use this when the component was just applied and .terraform/ state is already correct.
	// Only terraform output is executed.
	SkipInit bool
}

// Executor orchestrates terraform output retrieval with dependency injection.
type Executor struct {
	runnerFactory           RunnerFactory
	componentDescriber      ComponentDescriber
	staticRemoteStateGetter StaticRemoteStateGetter
	workdirProvisioner      WorkdirProvisioner
}

// ExecutorOption configures the Executor.
type ExecutorOption func(*Executor)

// WithRunnerFactory sets a custom runner factory (for testing).
func WithRunnerFactory(factory RunnerFactory) ExecutorOption {
	defer perf.Track(nil, "output.WithRunnerFactory")()

	return func(e *Executor) {
		e.runnerFactory = factory
	}
}

// WithStaticRemoteStateGetter sets a custom static remote state getter.
func WithStaticRemoteStateGetter(getter StaticRemoteStateGetter) ExecutorOption {
	defer perf.Track(nil, "output.WithStaticRemoteStateGetter")()

	return func(e *Executor) {
		e.staticRemoteStateGetter = getter
	}
}

// WithWorkdirProvisioner sets a custom workdir provisioner (for testing).
func WithWorkdirProvisioner(p WorkdirProvisioner) ExecutorOption {
	defer perf.Track(nil, "output.WithWorkdirProvisioner")()

	return func(e *Executor) {
		e.workdirProvisioner = p
	}
}

// NewExecutor creates an Executor with the required ComponentDescriber and optional configurations.
func NewExecutor(describer ComponentDescriber, opts ...ExecutorOption) *Executor {
	defer perf.Track(nil, "output.NewExecutor")()

	e := &Executor{
		runnerFactory:      defaultRunnerFactory,
		componentDescriber: describer,
		workdirProvisioner: &defaultWorkdirProvisioner{},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// GetAllOutputs retrieves all terraform outputs for a component in a stack.
// This is used by the --format flag to get all outputs at once for formatting.
func (e *Executor) GetAllOutputs(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	skipInit bool,
	authManager any,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.Executor.GetAllOutputs")()

	stackSlug := stackComponentKey(stack, component)
	if outputs := checkOutputsCache(stackSlug, component, stack); outputs != nil {
		return outputs, nil
	}

	message := fmt.Sprintf("Fetching all outputs from %s in %s", component, stack)
	stopSpinner := startSpinnerOrLog(atmosConfig, message, component, stack)
	defer stopSpinner()

	// Use quiet mode to suppress terraform init/workspace output.
	opts := &OutputOptions{QuietMode: true, SkipInit: skipInit}
	outputs, err := e.fetchAndCacheOutputs(atmosConfig, component, stack, stackSlug, nil, opts, authManager)
	if err != nil {
		ui.ClearLine()
		ui.Error(message)
		return nil, err
	}

	ui.ClearLine()
	ui.Success(message)
	return outputs, nil
}

// GetOutput retrieves a specific terraform output for a component in a stack.
//
//nolint:revive,funlen // argument-limit: matches GetTerraformOutput signature for backward compatibility.
func (e *Executor) GetOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "output.Executor.GetOutput")()

	// Validate authManager type if provided.
	if authManager != nil {
		if _, ok := authManager.(auth.AuthManager); !ok {
			return nil, false, fmt.Errorf("%w: expected auth.AuthManager", errUtils.ErrInvalidAuthManagerType)
		}
	}

	stackSlug := stackComponentKey(stack, component)

	// Check cache first.
	if !skipCache {
		if cachedOutputs, found := terraformOutputsCache.Load(stackSlug); found && cachedOutputs != nil {
			log.Debug("Cache hit for terraform output", "stack", stack, "component", component, "output", output)
			return getOutputVariable(atmosConfig, component, stack, cachedOutputs.(map[string]any), output)
		}
	}

	message := fmt.Sprintf("Fetching %s output from %s in %s", output, component, stack)
	stopSpinner := startSpinnerOrLog(atmosConfig, message, component, stack)
	defer stopSpinner()

	// Describe the component to get its configuration.
	sections, err := e.componentDescriber.DescribeComponent(&DescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		AuthManager:          authManager,
	})
	if err != nil {
		ui.ClearLine()
		ui.Error(message)
		return nil, false, wrapDescribeError(component, stack, err)
	}

	// Check for static remote state backend.
	if e.staticRemoteStateGetter != nil {
		if staticOutputs := e.staticRemoteStateGetter.GetStaticRemoteStateOutputs(&sections); staticOutputs != nil {
			terraformOutputsCache.Store(stackSlug, staticOutputs)
			value, exists, resultErr := GetStaticRemoteStateOutput(atmosConfig, component, stack, staticOutputs, output)
			if resultErr != nil {
				ui.ClearLine()
				ui.Error(message)
				return nil, false, resultErr
			}
			ui.ClearLine()
			ui.Success(message)
			return value, exists, nil
		}
	}

	// Execute terraform output.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	outputs, err := e.execute(ctx, atmosConfig, component, stack, sections, authContext, nil)
	if err != nil {
		ui.ClearLine()
		ui.Error(message)
		return nil, false, errUtils.Build(errUtils.ErrTerraformOutputFailed).
			WithCause(err).
			WithExplanationf("failed to execute terraform output for component %s in stack %s", component, stack).
			Err()
	}

	// Cache the result.
	terraformOutputsCache.Store(stackSlug, outputs)

	value, exists, resultErr := getOutputVariable(atmosConfig, component, stack, outputs, output)
	if resultErr != nil {
		ui.ClearLine()
		ui.Error(message)
		return nil, false, resultErr
	}

	ui.ClearLine()
	ui.Success(message)
	return value, exists, nil
}

// GetOutputWithOptions retrieves a specific terraform output with explicit execution options.
// Unlike GetOutput, this method accepts OutputOptions so callers can pass SkipInit: true
// when the workdir is already initialised (e.g. after-terraform-apply hooks).
//
//nolint:revive // argument-limit: matches GetOutput signature plus opts.
func (e *Executor) GetOutputWithOptions(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
	opts *OutputOptions,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "output.Executor.GetOutputWithOptions")()

	// Validate authManager type if provided.
	if authManager != nil {
		if _, ok := authManager.(auth.AuthManager); !ok {
			return nil, false, fmt.Errorf("%w: expected auth.AuthManager", errUtils.ErrInvalidAuthManagerType)
		}
	}

	stackSlug := stackComponentKey(stack, component)

	// Check cache first.
	if !skipCache {
		if cachedOutputs, found := terraformOutputsCache.Load(stackSlug); found && cachedOutputs != nil {
			log.Debug("Cache hit for terraform output", "stack", stack, "component", component, "output", output)
			return getOutputVariable(atmosConfig, component, stack, cachedOutputs.(map[string]any), output)
		}
	}

	message := fmt.Sprintf("Fetching %s output from %s in %s", output, component, stack)
	stopSpinner := startSpinnerOrLog(atmosConfig, message, component, stack)
	defer stopSpinner()

	// Describe the component to get its configuration.
	// When SkipInit is set and no authManager is provided, skip YAML function
	// processing to avoid failures on auth-backed functions (e.g. !terraform.state)
	// that need credentials which may not be available in post-hook context.
	processYamlFunctions := true
	if opts != nil && opts.SkipInit && authManager == nil {
		processYamlFunctions = false
	}

	sections, err := e.componentDescriber.DescribeComponent(&DescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: processYamlFunctions,
		AuthManager:          authManager,
	})
	if err != nil {
		ui.ClearLine()
		ui.Error(message)
		return nil, false, wrapDescribeError(component, stack, err)
	}

	// Check for static remote state backend.
	if e.staticRemoteStateGetter != nil {
		if staticOutputs := e.staticRemoteStateGetter.GetStaticRemoteStateOutputs(&sections); staticOutputs != nil {
			terraformOutputsCache.Store(stackSlug, staticOutputs)
			value, exists, resultErr := GetStaticRemoteStateOutput(atmosConfig, component, stack, staticOutputs, output)
			if resultErr != nil {
				ui.ClearLine()
				ui.Error(message)
				return nil, false, resultErr
			}
			ui.ClearLine()
			ui.Success(message)
			return value, exists, nil
		}
	}

	// Execute terraform output with caller-supplied options.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	outputs, err := e.execute(ctx, atmosConfig, component, stack, sections, authContext, opts)
	if err != nil {
		ui.ClearLine()
		ui.Error(message)
		return nil, false, errUtils.Build(errUtils.ErrTerraformOutputFailed).
			WithCause(err).
			WithExplanationf("failed to execute terraform output for component %s in stack %s", component, stack).
			Err()
	}

	// Cache the result.
	terraformOutputsCache.Store(stackSlug, outputs)

	value, exists, resultErr := getOutputVariable(atmosConfig, component, stack, outputs, output)
	if resultErr != nil {
		ui.ClearLine()
		ui.Error(message)
		return nil, false, resultErr
	}

	ui.ClearLine()
	ui.Success(message)
	return value, exists, nil
}

// ExecuteWithSections retrieves terraform outputs using pre-loaded sections.
// This is used when the caller already has sections from ExecuteDescribeComponent.
func (e *Executor) ExecuteWithSections(
	atmosConfig *schema.AtmosConfiguration,
	component, stack string,
	sections map[string]any,
	authContext *schema.AuthContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.Executor.ExecuteWithSections")()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	return e.execute(ctx, atmosConfig, component, stack, sections, authContext, nil)
}

// fetchAndCacheOutputs retrieves outputs and stores them in cache.
//
//nolint:revive // argument-limit: internal function with complex state.
func (e *Executor) fetchAndCacheOutputs(
	atmosConfig *schema.AtmosConfiguration,
	component, stack, stackSlug string,
	authContext *schema.AuthContext,
	opts *OutputOptions,
	authManager any,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.Executor.fetchAndCacheOutputs")()

	// When skipInit is set and no authManager is provided, skip YAML function
	// processing. YAML functions like !terraform.state need credentials that
	// may not be available in PostRunE context. We only need the component path
	// and workspace info to run terraform output.
	processYamlFunctions := true
	if opts != nil && opts.SkipInit && authManager == nil {
		processYamlFunctions = false
	}

	sections, err := e.componentDescriber.DescribeComponent(&DescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: processYamlFunctions,
		AuthManager:          authManager,
	})
	if err != nil {
		return nil, wrapDescribeError(component, stack, err)
	}

	// Check for static remote state backend.
	if e.staticRemoteStateGetter != nil {
		if staticOutputs := e.staticRemoteStateGetter.GetStaticRemoteStateOutputs(&sections); staticOutputs != nil {
			terraformOutputsCache.Store(stackSlug, staticOutputs)
			return staticOutputs, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	outputs, err := e.execute(ctx, atmosConfig, component, stack, sections, authContext, opts)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrTerraformOutputFailed).
			WithCause(err).
			WithExplanationf("failed to execute terraform output for component %s in stack %s", component, stack).
			Err()
	}

	terraformOutputsCache.Store(stackSlug, outputs)
	return outputs, nil
}

// execute retrieves terraform outputs for a component.
// This is the core execution logic that orchestrates all the terraform output retrieval steps.
//
//nolint:revive,funlen // argument-limit: internal function with complex state.
func (e *Executor) execute(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	component, stack string,
	sections map[string]any,
	authContext *schema.AuthContext,
	opts *OutputOptions,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.Executor.execute")()

	// Step 1: Check if component should be processed.
	enabled, abstract := IsComponentProcessable(sections)
	if !enabled || abstract {
		return handleDisabledComponent(component, stack, enabled, abstract), nil
	}

	// Step 2: Extract and validate component configuration.
	// ExtractComponentConfig uses utils.GetComponentPath internally to ensure
	// proper path resolution that works correctly with --chdir and when running
	// from non-project-root directories.
	config, err := ExtractComponentConfig(atmosConfig, sections, component, stack)
	if err != nil {
		return nil, err
	}

	// Step 2.5: Auto-provision JIT workdir if needed before toolchain resolution and init.
	// Must run first so the workdir directory exists when backend files are written into it.
	if err := e.ensureWorkdirProvisioned(ctx, atmosConfig, sections, authContext, component, stack, config); err != nil {
		return nil, err
	}

	// Step 3: Resolve toolchain dependencies and executable path.
	// This ensures that toolchain-installed executables (e.g., tofu via `atmos toolchain install`)
	// are found even when they are not on the system PATH. Without this, template functions like
	// atmos.Component() and YAML functions like !terraform.output fail with "executable not found".
	tenv, err := dependencies.ForSections(atmosConfig, sections)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve toolchain for %s in %s: %w", component, stack, err)
	}
	config.Executable = tenv.Resolve(config.Executable)

	// Step 4: Generate backend file if needed.
	backendGen := &defaultBackendGenerator{}
	if err := backendGen.GenerateBackendIfNeeded(config, component, stack, authContext); err != nil {
		return nil, err
	}

	// Step 5: Generate provider overrides if needed.
	if err := backendGen.GenerateProvidersIfNeeded(config, authContext); err != nil {
		return nil, err
	}

	// Step 6: Create terraform runner.
	runner, err := e.runnerFactory(config.ComponentPath, config.Executable)
	if err != nil {
		return nil, err
	}

	// Step 7: Configure quiet mode if requested.
	var stderrCapture *quietModeWriter
	if opts != nil && opts.QuietMode {
		runner.SetStdout(io.Discard)
		stderrCapture = newQuietModeWriter()
		runner.SetStderr(stderrCapture)
	}

	// Step 8: Setup environment variables.
	envSetup := &defaultEnvironmentSetup{}
	environMap, err := envSetup.SetupEnvironment(config, authContext)
	if err != nil {
		return nil, err
	}
	// Prepend toolchain bin dirs to subprocess PATH so terraform/tofu subprocesses
	// can also find toolchain-installed binaries. Uses PrependToPath to preserve
	// any PATH overrides from the component's env section.
	if len(tenv.ToolchainDirs()) > 0 {
		environMap["PATH"] = tenv.PrependToPath(environMap["PATH"])
	}
	if len(environMap) > 0 {
		if err := runner.SetEnv(environMap); err != nil {
			return nil, err
		}
	}

	// Step 9: Clean workspace and run terraform init (skipped when SkipInit is set).
	// SkipInit is used when the component was just applied and .terraform/ state
	// is already correct — re-initializing would require auth credentials that
	// may not be available in PostRunE context.
	skipInit := opts != nil && opts.SkipInit
	if !skipInit {
		workspaceMgr := &defaultWorkspaceManager{}
		workspaceMgr.CleanWorkspace(atmosConfig, config.ComponentPath)

		if err := e.runInit(ctx, runner, config, component, stack, stderrCapture); err != nil {
			return nil, err
		}

		// Step 10: Ensure workspace exists and is selected.
		if err := workspaceMgr.EnsureWorkspace(ctx, runner, config.Workspace, config.BackendType, component, stack, stderrCapture); err != nil {
			return nil, err
		}
	}

	// Step 11: Execute terraform output.
	outputMeta, err := e.runOutput(ctx, runner, component, stack, stderrCapture)
	if err != nil {
		return nil, err
	}

	// Step 12: Process and convert output values.
	return processOutputs(outputMeta, atmosConfig), nil
}
