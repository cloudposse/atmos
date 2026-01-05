package output

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

//go:generate go run go.uber.org/mock/mockgen@latest -source=executor.go -destination=mock_executor_test.go -package=output

// String constants for logging.
const (
	logKeyComponent = "component"
	logKeyStack     = "stack"
	logKeyWorkspace = "workspace"
	dotSeparator    = "."
	// MaxLogValueLen is the maximum length of a value to log before truncating.
	maxLogValueLen = 100
)

// wrapDescribeError wraps an error from DescribeComponent, breaking the ErrInvalidComponent
// chain to prevent triggering component type fallback in detectComponentType.
// This is critical for proper error propagation when a referenced component is not found.
func wrapDescribeError(component, stack string, err error) error {
	if errors.Is(err, errUtils.ErrInvalidComponent) {
		// Break the ErrInvalidComponent chain by using ErrDescribeComponent as the base.
		// This ensures that errors from YAML function processing (like !terraform.output
		// referencing a missing component) don't trigger fallback to try other component types.
		// The original error message is preserved for debugging.
		return fmt.Errorf("%w: component '%s' in stack '%s': %s",
			errUtils.ErrDescribeComponent, component, stack, err.Error())
	}
	// For other errors, preserve the full chain.
	return fmt.Errorf("failed to describe component %s in stack %s: %w", component, stack, err)
}

// terraformOutputsCache caches terraform outputs by stack-component slug.
var terraformOutputsCache = sync.Map{}

// TerraformRunner abstracts terraform-exec operations for testability.
type TerraformRunner interface {
	// Init runs terraform init with the given options.
	Init(ctx context.Context, opts ...tfexec.InitOption) error
	// WorkspaceNew creates a new terraform workspace.
	WorkspaceNew(ctx context.Context, workspace string, opts ...tfexec.WorkspaceNewCmdOption) error
	// WorkspaceSelect selects an existing terraform workspace.
	WorkspaceSelect(ctx context.Context, workspace string) error
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

// defaultRunnerFactory creates a real terraform runner using tfexec.
func defaultRunnerFactory(workdir, executable string) (TerraformRunner, error) {
	return tfexec.NewTerraform(workdir, executable)
}

// DescribeComponentParams contains parameters for describing a component.
type DescribeComponentParams struct {
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
}

// quietModeWriter captures output during quiet mode operations.
// On success, output is discarded. On failure, captured stderr is included in errors.
type quietModeWriter struct {
	buffer *strings.Builder
}

func newQuietModeWriter() *quietModeWriter {
	return &quietModeWriter{buffer: &strings.Builder{}}
}

// Write implements io.Writer interface.
func (w *quietModeWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "output.quietModeWriter.Write")()

	return w.buffer.Write(p)
}

// String returns the captured output.
func (w *quietModeWriter) String() string {
	defer perf.Track(nil, "output.quietModeWriter.String")()

	return w.buffer.String()
}

// wrapErrorWithStderr wraps an error with captured stderr output if available.
// Used in quiet mode to include terraform output in error messages on failure.
func wrapErrorWithStderr(err error, capture *quietModeWriter) error {
	if capture == nil || capture.String() == "" {
		return err
	}
	return errUtils.Build(errUtils.ErrTerraformOutputFailed).
		WithCause(err).
		WithExplanation(strings.TrimSpace(capture.String())).
		Err()
}

// Executor orchestrates terraform output retrieval with dependency injection.
type Executor struct {
	runnerFactory           RunnerFactory
	componentDescriber      ComponentDescriber
	staticRemoteStateGetter StaticRemoteStateGetter
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

// NewExecutor creates an Executor with the required ComponentDescriber and optional configurations.
func NewExecutor(describer ComponentDescriber, opts ...ExecutorOption) *Executor {
	defer perf.Track(nil, "output.NewExecutor")()

	e := &Executor{
		runnerFactory:      defaultRunnerFactory,
		componentDescriber: describer,
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
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.Executor.GetAllOutputs")()

	stackSlug := fmt.Sprintf("%s-%s", stack, component)
	if outputs := checkOutputsCache(stackSlug, component, stack); outputs != nil {
		return outputs, nil
	}

	message := fmt.Sprintf("Fetching all outputs from %s in %s", component, stack)
	stopSpinner := startSpinnerOrLog(atmosConfig, message, component, stack)
	defer stopSpinner()

	// Note: skipInit is currently ignored - terraform init is always run to ensure correct state.
	_ = skipInit

	// Use quiet mode to suppress terraform init/workspace output.
	opts := &OutputOptions{QuietMode: true}
	outputs, err := e.fetchAndCacheOutputs(atmosConfig, component, stack, stackSlug, nil, opts)
	if err != nil {
		u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
		return nil, err
	}

	u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.Checkmark, message)
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

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

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
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		AuthManager:          authManager,
	})
	if err != nil {
		u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
		return nil, false, wrapDescribeError(component, stack, err)
	}

	// Check for static remote state backend.
	if e.staticRemoteStateGetter != nil {
		if staticOutputs := e.staticRemoteStateGetter.GetStaticRemoteStateOutputs(&sections); staticOutputs != nil {
			terraformOutputsCache.Store(stackSlug, staticOutputs)
			value, exists, resultErr := GetStaticRemoteStateOutput(atmosConfig, component, stack, staticOutputs, output)
			if resultErr != nil {
				u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
				return nil, false, resultErr
			}
			u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.Checkmark, message)
			return value, exists, nil
		}
	}

	// Execute terraform output.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	outputs, err := e.execute(ctx, atmosConfig, component, stack, sections, authContext, nil)
	if err != nil {
		u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
		return nil, false, fmt.Errorf("failed to execute terraform output for component %s in stack %s: %w", component, stack, err)
	}

	// Cache the result.
	terraformOutputsCache.Store(stackSlug, outputs)

	value, exists, resultErr := getOutputVariable(atmosConfig, component, stack, outputs, output)
	if resultErr != nil {
		u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.XMark, message)
		return nil, false, resultErr
	}

	u.PrintfMessageToTUI(terminal.EscResetLine+"%s %s\n", theme.Styles.Checkmark, message)
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
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "output.Executor.fetchAndCacheOutputs")()

	sections, err := e.componentDescriber.DescribeComponent(&DescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
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
		return nil, fmt.Errorf("failed to execute terraform output for component %s in stack %s: %w", component, stack, err)
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
	config, err := ExtractComponentConfig(
		sections,
		component,
		stack,
		atmosConfig.Components.Terraform.AutoGenerateBackendFile,
		atmosConfig.Components.Terraform.InitRunReconfigure,
	)
	if err != nil {
		return nil, err
	}

	// Step 3: Generate backend file if needed.
	backendGen := &defaultBackendGenerator{}
	if err := backendGen.GenerateBackendIfNeeded(config, component, stack, authContext); err != nil {
		return nil, err
	}

	// Step 4: Generate provider overrides if needed.
	if err := backendGen.GenerateProvidersIfNeeded(config, authContext); err != nil {
		return nil, err
	}

	// Step 5: Create terraform runner.
	runner, err := e.runnerFactory(config.ComponentPath, config.Executable)
	if err != nil {
		return nil, err
	}

	// Step 6: Configure quiet mode if requested.
	var stderrCapture *quietModeWriter
	if opts != nil && opts.QuietMode {
		runner.SetStdout(io.Discard)
		stderrCapture = newQuietModeWriter()
		runner.SetStderr(stderrCapture)
	}

	// Step 7: Setup environment variables.
	envSetup := &defaultEnvironmentSetup{}
	environMap, err := envSetup.SetupEnvironment(config, authContext)
	if err != nil {
		return nil, err
	}
	if len(environMap) > 0 {
		if err := runner.SetEnv(environMap); err != nil {
			return nil, err
		}
	}

	// Step 8: Clean workspace and run terraform init.
	workspaceMgr := &defaultWorkspaceManager{}
	workspaceMgr.CleanWorkspace(atmosConfig, config.ComponentPath)

	if err := e.runInit(ctx, runner, config, component, stack, stderrCapture); err != nil {
		return nil, err
	}

	// Step 9: Ensure workspace exists and is selected.
	if err := workspaceMgr.EnsureWorkspace(ctx, runner, config.Workspace, config.BackendType, component, stack, stderrCapture); err != nil {
		return nil, err
	}

	// Step 10: Execute terraform output.
	outputMeta, err := e.runOutput(ctx, runner, component, stack, stderrCapture)
	if err != nil {
		return nil, err
	}

	// Step 11: Process and convert output values.
	return processOutputs(outputMeta, atmosConfig), nil
}

// handleDisabledComponent returns empty outputs for disabled or abstract components.
func handleDisabledComponent(component, stack string, _, abstract bool) map[string]any {
	status := "disabled"
	if abstract {
		status = "abstract"
	}
	log.Debug("Skipping terraform output due to component status", "component", component, "stack", stack, "status", status)
	return map[string]any{}
}

// runInit executes terraform init with appropriate options.
//
//nolint:revive // argument-limit: internal function passing through execution context.
func (e *Executor) runInit(ctx context.Context, runner TerraformRunner, config *ComponentConfig, component, stack string, stderrCapture *quietModeWriter) error {
	defer perf.Track(nil, "output.Executor.runInit")()

	log.Debug("Executing terraform init", "component", component, "stack", stack)

	var initOptions []tfexec.InitOption
	initOptions = append(initOptions, tfexec.Upgrade(false))
	if config.InitRunReconfigure {
		initOptions = append(initOptions, tfexec.Reconfigure(true))
	}

	if err := runner.Init(ctx, initOptions...); err != nil {
		return wrapErrorWithStderr(
			errUtils.Build(errUtils.ErrTerraformInit).WithCause(err).Err(),
			stderrCapture,
		)
	}

	log.Debug("Completed terraform init", "component", component, "stack", stack)
	return nil
}

// runOutput executes terraform output with retry logic.
func (e *Executor) runOutput(ctx context.Context, runner TerraformRunner, component, stack string, stderrCapture *quietModeWriter) (map[string]tfexec.OutputMeta, error) {
	defer perf.Track(nil, "output.Executor.runOutput")()

	log.Debug("Executing terraform output", "component", component, "stack", stack)

	// Add small delay on Windows to prevent file locking issues.
	windowsFileDelay()

	var outputMeta map[string]tfexec.OutputMeta
	err := retryOnWindows(func() error {
		var outputErr error
		outputMeta, outputErr = runner.Output(ctx)
		return outputErr
	})
	if err != nil {
		return nil, wrapErrorWithStderr(err, stderrCapture)
	}

	log.Debug("Completed terraform output", "component", component, "stack", stack)
	return outputMeta, nil
}

// processOutputs converts tfexec.OutputMeta to map[string]any.
func processOutputs(outputMeta map[string]tfexec.OutputMeta, atmosConfig *schema.AtmosConfiguration) map[string]any {
	defer perf.Track(atmosConfig, "output.processOutputs")()

	return lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
		s := string(v.Value)

		// Log summary to avoid multiline value formatting issues.
		valueSummary := summarizeValue(s)
		log.Debug("Converting output from JSON to Go data type", "key", k, "value_summary", valueSummary)

		d, err := u.ConvertFromJSON(s)
		if err != nil {
			log.Error("Failed to convert output", "key", k, "error", err)
			return k, nil
		}

		return k, d
	})
}

// summarizeValue creates a summary for logging long or multiline values.
func summarizeValue(s string) string {
	if strings.Contains(s, "\n") {
		lineCount := strings.Count(s, "\n") + 1
		return fmt.Sprintf("<multiline: %d lines, %d bytes>", lineCount, len(s))
	}
	if len(s) > maxLogValueLen {
		return s[:maxLogValueLen] + "..."
	}
	return s
}

// checkOutputsCache checks if terraform outputs are already cached for the given stack/component.
func checkOutputsCache(stackSlug, component, stack string) map[string]any {
	cachedOutputs, found := terraformOutputsCache.Load(stackSlug)
	if found && cachedOutputs != nil {
		log.Debug("Cache hit for terraform outputs", "stack", stack, "component", component)
		return cachedOutputs.(map[string]any)
	}
	return nil
}

// startSpinnerOrLog starts a spinner in normal mode or logs in debug mode, returns a stop function.
func startSpinnerOrLog(atmosConfig *schema.AtmosConfiguration, message, _, _ string) func() {
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		log.Debug(message)
		return func() {}
	}
	p := NewSpinner(message)
	spinnerDone := make(chan struct{})
	RunSpinner(p, spinnerDone, message)
	return func() { StopSpinner(p, spinnerDone) }
}

// extractYqValue extracts a value from a map using yq expression.
// It returns the extracted value, whether the key exists, and any error.
func extractYqValue(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	output string,
	errContext string,
) (any, bool, error) {
	// Use yq to extract the value (handles nested paths, alternative operators, etc.).
	val := output
	if !strings.HasPrefix(output, dotSeparator) {
		val = dotSeparator + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, data, val)
	if err != nil {
		return nil, false, fmt.Errorf("failed to evaluate %s: %w", errContext, err)
	}

	// Check if this is a simple key lookup (no yq operators).
	hasYqOperators := strings.Contains(output, "//") ||
		strings.Contains(output, "|") ||
		strings.Contains(output, "=") ||
		strings.Contains(output, "[") ||
		strings.Contains(output, "]")

	if !hasYqOperators {
		outputKey := strings.TrimPrefix(output, dotSeparator)
		if !strings.Contains(outputKey, dotSeparator) {
			_, exists := data[outputKey]
			if !exists {
				return nil, false, nil
			}
		}
	}

	return res, true, nil
}

// getOutputVariable extracts a specific output variable using yq expression.
func getOutputVariable(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	outputs map[string]any,
	output string,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "output.getOutputVariable")()

	errContext := fmt.Sprintf("terraform output for component %s in stack %s", component, stack)
	return extractYqValue(atmosConfig, outputs, output, errContext)
}

// GetStaticRemoteStateOutput extracts a specific output from static remote state.
// This is exported for use by terraform_state_utils.go and other callers that need
// to extract values from static remote state sections.
func GetStaticRemoteStateOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "output.GetStaticRemoteStateOutput")()

	errContext := fmt.Sprintf("static remote state for component %s in stack %s", component, stack)
	return extractYqValue(atmosConfig, remoteStateSection, output, errContext)
}
