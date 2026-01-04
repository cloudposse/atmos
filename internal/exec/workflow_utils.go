package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/samber/lo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	w "github.com/cloudposse/atmos/internal/tui/workflow"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Workflow error title for formatted output.
const WorkflowErrTitle = "Workflow Error"

// Local errors not in shared package (workflow-specific internal errors).
var (
	ErrNoWorkflowFilesToSelect = errors.New("no workflow files to select from")
	ErrNonTTYWorkflowSelection = errors.New("interactive workflow selection not available in non-TTY or CI environments")
)

// KnownWorkflowErrors contains all known workflow sentinel errors for error handling.
var KnownWorkflowErrors = []error{
	errUtils.ErrWorkflowNoSteps,
	errUtils.ErrInvalidWorkflowStepType,
	errUtils.ErrInvalidFromStep,
	errUtils.ErrWorkflowStepFailed,
	errUtils.ErrWorkflowNoWorkflow,
	errUtils.ErrWorkflowFileNotFound,
	errUtils.ErrInvalidWorkflowManifest,
}

// workflowStepErrorContext contains context needed to build workflow step errors.
type workflowStepErrorContext struct {
	WorkflowPath     string
	WorkflowBasePath string
	Workflow         string
	StepName         string
	Command          string
	CommandType      string
	FinalStack       string
}

// buildWorkflowStepError builds an error with resume hints when a workflow step fails.
func buildWorkflowStepError(err error, ctx *workflowStepErrorContext) error {
	log.Debug("Workflow failed", "error", err)

	// Remove the workflow base path, stacks/workflows.
	workflowFileName := strings.TrimPrefix(filepath.ToSlash(ctx.WorkflowPath), filepath.ToSlash(ctx.WorkflowBasePath))
	// Remove the leading slash.
	workflowFileName = strings.TrimPrefix(workflowFileName, "/")
	// Remove the file extension.
	workflowFileName = strings.TrimSuffix(workflowFileName, filepath.Ext(workflowFileName))

	resumeCommand := fmt.Sprintf(
		"%s workflow %s -f %s --from-step '%s'",
		config.AtmosCommand,
		ctx.Workflow,
		workflowFileName,
		ctx.StepName,
	)

	// Add stack parameter to resume command if a stack was used.
	if ctx.FinalStack != "" {
		resumeCommand = fmt.Sprintf("%s -s '%s'", resumeCommand, ctx.FinalStack)
	}

	failedCmd := ctx.Command
	if ctx.CommandType == config.AtmosCommand {
		failedCmd = config.AtmosCommand + " " + ctx.Command
		// Add stack parameter to failed command if a stack was used.
		if ctx.FinalStack != "" {
			failedCmd = fmt.Sprintf("%s -s '%s'", failedCmd, ctx.FinalStack)
		}
	}

	// Build error with context about the failed command.
	// Use fmt.Errorf with %w to wrap the underlying error while adding ErrWorkflowStepFailed to the chain.
	// This preserves both the error sentinel for errors.Is() checks and the underlying error's exit code.
	wrappedErr := fmt.Errorf("%w: %w", errUtils.ErrWorkflowStepFailed, err)

	// Now build the error with hints using the wrapped error.
	// This preserves the error chain while adding formatted hints.
	// Commands are wrapped in code fences for proper formatting and copy-paste.
	// Single quotes are used for shell safety (step names and stacks can contain spaces).
	builder := errUtils.Build(wrappedErr).
		WithTitle("Workflow Error").
		WithHintf("The following command failed to execute:\n\n```shell\n%s\n```", failedCmd).
		WithHintf("To resume the workflow from this step, run:\n\n```shell\n%s\n```", resumeCommand)

	// Extract exit code from the underlying error if available.
	if exitCode := errUtils.GetExitCode(err); exitCode != 0 {
		builder = builder.WithExitCode(exitCode)
	}

	return builder.Err()
}

// prepareStepEnvironment prepares environment variables for a workflow step.
// Starts with system env + global env from atmos.yaml.
// If identity is specified, it authenticates and adds credentials to the environment.
// Returns the environment variables to use for the step.
func prepareStepEnvironment(
	stepIdentity string,
	stepName string,
	authManager auth.AuthManager,
	globalEnv map[string]string,
) ([]string, error) {
	// Prepare base environment: system env + global env from atmos.yaml.
	// Global env has lowest priority and can be overridden by identity auth env vars.
	baseEnv := envpkg.MergeGlobalEnv(os.Environ(), globalEnv)

	// No identity specified, use base environment (system + global env).
	if stepIdentity == "" {
		return baseEnv, nil
	}

	if authManager == nil {
		return nil, errUtils.Build(errUtils.ErrAuthManager).
			WithExplanation("auth manager is not initialized").
			WithContext("identity", stepIdentity).
			WithContext("step", stepName).
			Err()
	}

	ctx := context.Background()

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	if _, err := authManager.GetCachedCredentials(ctx, stepIdentity); err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", stepIdentity, "error", err)
		// No valid cached credentials - perform full authentication.
		if _, err = authManager.Authenticate(ctx, stepIdentity); err != nil {
			// Check for user cancellation - return clean error without wrapping.
			if errors.Is(err, errUtils.ErrUserAborted) {
				return nil, errUtils.ErrUserAborted
			}
			return nil, fmt.Errorf("%w for identity %q in step %q: %w", errUtils.ErrAuthenticationFailed, stepIdentity, stepName, err)
		}
	}

	// Prepare shell environment with authentication credentials.
	// Start with base environment (system + global) and let PrepareShellEnvironment configure auth.
	stepEnv, err := authManager.PrepareShellEnvironment(ctx, stepIdentity, baseEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare shell environment for identity %q in step %q: %w", stepIdentity, stepName, err)
	}

	log.Debug("Prepared environment with identity", "identity", stepIdentity, "step", stepName)
	return stepEnv, nil
}

// IsKnownWorkflowError returns true if the error matches any known workflow error.
// This includes ExitCodeError which indicates a subcommand failure that's already been reported.
func IsKnownWorkflowError(err error) bool {
	// Check if it's an ExitCodeError - these are already reported by the subcommand
	var exitCodeErr errUtils.ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return true
	}

	// Check known workflow errors
	for _, knownErr := range KnownWorkflowErrors {
		if errors.Is(err, knownErr) {
			return true
		}
	}
	return false
}

// checkAndMergeDefaultIdentity checks if there's a default identity configured in atmos.yaml or stack configs.
// If a default identity is found in stack configs, it merges it into atmosConfig.Auth.
// Stack defaults take precedence over atmos.yaml defaults (following Atmos inheritance model).
// Returns true if a default identity exists after merging.
func checkAndMergeDefaultIdentity(atmosConfig *schema.AtmosConfiguration) bool {
	if len(atmosConfig.Auth.Identities) == 0 {
		return false
	}

	// Always load stack configs - stack defaults take precedence over atmos.yaml.
	stackDefaults, err := config.LoadStackAuthDefaults(atmosConfig)
	if err != nil {
		// On error, fall back to checking atmos.yaml defaults.
		for _, identity := range atmosConfig.Auth.Identities {
			if identity.Default {
				return true
			}
		}
		return false
	}

	// Merge stack defaults into auth config (stack takes precedence).
	if len(stackDefaults) > 0 {
		config.MergeStackAuthDefaults(&atmosConfig.Auth, stackDefaults)
	}

	// Check if we have a default after merging.
	for _, identity := range atmosConfig.Auth.Identities {
		if identity.Default {
			return true
		}
	}

	return false
}

// ExecuteWorkflow executes an Atmos workflow.
func ExecuteWorkflow(
	atmosConfig schema.AtmosConfiguration,
	workflow string,
	workflowPath string,
	workflowDefinition *schema.WorkflowDefinition,
	dryRun bool,
	commandLineStack string,
	fromStep string,
	commandLineIdentity string,
) error {
	defer perf.Track(&atmosConfig, "exec.ExecuteWorkflow")()

	steps := workflowDefinition.Steps

	if len(steps) == 0 {
		return errUtils.Build(errUtils.ErrWorkflowNoSteps).
			WithTitle(WorkflowErrTitle).
			WithExplanationf("Workflow `%s` is empty and requires at least one step to execute.", workflow).
			WithContext("workflow", workflow).
			WithExitCode(1).
			Err()
	}

	// Check if the workflow steps have the `name` attribute
	checkAndGenerateWorkflowStepNames(workflowDefinition)

	log.Debug("Executing workflow", "workflow", workflow, "path", workflowPath)

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		err := u.PrintAsYAMLToFileDescriptor(&atmosConfig, workflowDefinition)
		if err != nil {
			return err
		}
	}

	// If `--from-step` is specified, skip all the previous steps
	if fromStep != "" {
		steps = lo.DropWhile[schema.WorkflowStep](steps, func(step schema.WorkflowStep) bool {
			return step.Name != fromStep
		})

		if len(steps) == 0 {
			stepNames := lo.Map(workflowDefinition.Steps, func(step schema.WorkflowStep, _ int) string { return step.Name })
			return errUtils.Build(errUtils.ErrInvalidFromStep).
				WithTitle(WorkflowErrTitle).
				WithExplanationf("The `--from-step` flag was set to `%s`, but this step does not exist in workflow `%s`.\n\n### Available steps:\n\n%s", fromStep, workflow, u.FormatList(stepNames)).
				WithContext("from_step", fromStep).
				WithContext("workflow", workflow).
				WithExitCode(1).
				Err()
		}
	}

	// Create auth manager if any step has an identity or if command-line identity is specified.
	// We check once upfront to avoid repeated initialization.
	var authManager auth.AuthManager
	var authStackInfo *schema.ConfigAndStacksInfo
	needsAuth := commandLineIdentity != "" || lo.SomeBy(steps, func(step schema.WorkflowStep) bool {
		return strings.TrimSpace(step.Identity) != ""
	})
	if needsAuth {
		// Create a ConfigAndStacksInfo for the auth manager to populate with AuthContext.
		// This enables YAML template functions to access authenticated credentials.
		authStackInfo = &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{},
		}

		credStore := credentials.NewCredentialStore()
		validator := validation.NewValidator()
		var err error
		authManager, err = auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
		}
	}

	for stepIdx, step := range steps {
		command := strings.TrimSpace(step.Command)
		commandType := strings.TrimSpace(step.Type)
		stepIdentity := strings.TrimSpace(step.Identity)

		// If step doesn't specify identity, use command-line identity (if provided).
		if stepIdentity == "" && commandLineIdentity != "" {
			stepIdentity = commandLineIdentity
		}

		finalStack := ""

		log.Debug("Executing workflow step", "step", stepIdx, "name", step.Name, "command", command)

		if commandType == "" {
			commandType = "atmos"
		}

		// Prepare environment variables: start with system env + global env from atmos.yaml.
		// If identity is specified, also authenticate and add credentials.
		stepEnv, err := prepareStepEnvironment(stepIdentity, step.Name, authManager, atmosConfig.GetCaseSensitiveEnvVars())
		if err != nil {
			return err
		}
		switch commandType {
		case "shell":
			commandName := fmt.Sprintf("%s-step-%d", workflow, stepIdx)
			err = ExecuteShell(&atmosConfig, command, commandName, ".", stepEnv, dryRun)
		case "atmos":
			args := strings.Fields(command)

			workflowStack := strings.TrimSpace(workflowDefinition.Stack)
			stepStack := strings.TrimSpace(step.Stack)

			// The workflow `stack` attribute overrides the stack in the `command` (if specified)
			// The step `stack` attribute overrides the stack in the `command` and the workflow `stack` attribute
			// The stack defined on the command line (`atmos workflow <name> -f <file> -s <stack>`) has the highest priority,
			// it overrides all other stacks attributes
			if workflowStack != "" {
				finalStack = workflowStack
			}
			if stepStack != "" {
				finalStack = stepStack
			}
			if commandLineStack != "" {
				finalStack = commandLineStack
			}

			if finalStack != "" {
				if idx := slices.Index(args, "--"); idx != -1 {
					// Insert before the "--"
					// Take everything up to idx, then add "-s", finalStack, then tack on the rest
					args = append(args[:idx], append([]string{"-s", finalStack}, args[idx:]...)...)
				} else {
					// just append at the end
					args = append(args, []string{"-s", finalStack}...)
				}

				log.Debug("Using stack", "stack", finalStack)
			}

			_ = ui.Infof("Executing command: `atmos %s`", command)
			err = retry.With7Params(context.Background(), step.Retry,
				ExecuteShellCommand,
				atmosConfig, "atmos", args, ".", stepEnv, dryRun, "")
		default:
			return errUtils.Build(errUtils.ErrInvalidWorkflowStepType).
				WithTitle(WorkflowErrTitle).
				WithHintf("Step type '%s' is not supported", commandType).
				WithHint("Each step must specify a valid type: 'atmos' or 'shell'").
				WithExitCode(1).
				Err()
		}

		if err != nil {
			return buildWorkflowStepError(err, &workflowStepErrorContext{
				WorkflowPath:     workflowPath,
				WorkflowBasePath: atmosConfig.Workflows.BasePath,
				Workflow:         workflow,
				StepName:         step.Name,
				Command:          command,
				CommandType:      commandType,
				FinalStack:       finalStack,
			})
		}
	}

	return nil
}

// ExecuteDescribeWorkflows executes `atmos describe workflows` command.
func ExecuteDescribeWorkflows(
	atmosConfig schema.AtmosConfiguration,
) ([]schema.DescribeWorkflowsItem, map[string][]string, map[string]schema.WorkflowManifest, error) {
	defer perf.Track(&atmosConfig, "exec.ExecuteDescribeWorkflows")()

	listResult := []schema.DescribeWorkflowsItem{}
	mapResult := make(map[string][]string)
	allResult := make(map[string]schema.WorkflowManifest)

	if atmosConfig.Workflows.BasePath == "" {
		return nil, nil, nil, errUtils.ErrWorkflowBasePathNotConfigured
	}

	// If `workflows.base_path` is a relative path, join it with `stacks.base_path`
	var workflowsDir string
	if u.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
		workflowsDir = atmosConfig.Workflows.BasePath
	} else {
		workflowsDir = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath)
	}

	isDirectory, err := u.IsDirectory(workflowsDir)
	if err != nil || !isDirectory {
		return nil, nil, nil, fmt.Errorf("the workflow directory '%s' does not exist. Review 'workflows.base_path' in 'atmos.yaml'", workflowsDir)
	}

	files, err := u.GetAllYamlFilesInDir(workflowsDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error reading the directory '%s' defined in 'workflows.base_path' in 'atmos.yaml': %v",
			atmosConfig.Workflows.BasePath, err)
	}

	for _, f := range files {
		var workflowPath string
		if u.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
			workflowPath = filepath.Join(atmosConfig.Workflows.BasePath, f)
		} else {
			workflowPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath, f)
		}

		fileContent, err := os.ReadFile(workflowPath)
		if err != nil {
			// Skip files that can't be read (permission issues, etc.).
			log.Warn("Skipping workflow file", "file", f, "error", err)
			continue
		}

		workflowManifest, err := u.UnmarshalYAML[schema.WorkflowManifest](string(fileContent))
		if err != nil {
			// Skip files that can't be parsed as YAML.
			log.Warn("Skipping invalid workflow file", "file", f, "error", err)
			continue
		}

		if workflowManifest.Workflows == nil {
			// Skip files without the workflows key.
			log.Warn("Skipping workflow file without 'workflows:' key", "file", f)
			continue
		}

		workflowConfig := workflowManifest.Workflows
		allWorkflowsInFile := lo.Keys(workflowConfig)
		sort.Strings(allWorkflowsInFile)

		// Check if the workflow steps have the `name` attribute
		lo.ForEach(allWorkflowsInFile, func(item string, _ int) {
			workflowDefinition := workflowConfig[item]
			checkAndGenerateWorkflowStepNames(&workflowDefinition)
		})

		mapResult[f] = allWorkflowsInFile
		allResult[f] = workflowManifest
	}

	for k, v := range mapResult {
		for _, w := range v {
			listResult = append(listResult, schema.DescribeWorkflowsItem{
				File:     k,
				Workflow: w,
			})
		}
	}

	return listResult, mapResult, allResult, nil
}

// WorkflowMatch represents a workflow found during auto-discovery.
type WorkflowMatch struct {
	File        string // Workflow file name (e.g., "networking.yaml")
	Name        string // Workflow name (e.g., "deploy-all")
	Description string // Workflow description (if available)
}

// findWorkflowAcrossFiles searches for a workflow by name across all workflow files.
// Returns a list of matching workflows with their file locations.
func findWorkflowAcrossFiles(workflowName string, atmosConfig *schema.AtmosConfiguration) ([]WorkflowMatch, error) {
	defer perf.Track(atmosConfig, "exec.findWorkflowAcrossFiles")()

	listResult, _, allWorkflows, err := ExecuteDescribeWorkflows(*atmosConfig)
	if err != nil {
		return nil, err
	}

	var matches []WorkflowMatch
	for _, item := range listResult {
		if item.Workflow == workflowName {
			// Get description if available.
			description := ""
			if manifest, ok := allWorkflows[item.File]; ok {
				if workflowDef, ok := manifest.Workflows[workflowName]; ok {
					description = workflowDef.Description
				}
			}

			matches = append(matches, WorkflowMatch{
				File:        item.File,
				Name:        workflowName,
				Description: description,
			})
		}
	}

	return matches, nil
}

// promptForWorkflowFile shows an interactive selector for choosing a workflow file.
// Uses the Huh library with Atmos theme (same pattern as identity selector).
func promptForWorkflowFile(matches []WorkflowMatch) (string, error) {
	defer perf.Track(nil, "exec.promptForWorkflowFile")()

	if len(matches) == 0 {
		return "", ErrNoWorkflowFilesToSelect
	}

	// Check if we're in a TTY environment.
	if !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		return "", ErrNonTTYWorkflowSelection
	}

	// Sort matches alphabetically by file name for consistent ordering.
	sortedMatches := make([]WorkflowMatch, len(matches))
	copy(sortedMatches, matches)
	sort.Slice(sortedMatches, func(i, j int) bool {
		return sortedMatches[i].File < sortedMatches[j].File
	})

	// Build options for the selector.
	// Each option shows the file name with description if available.
	options := make([]huh.Option[string], len(sortedMatches))
	for i, match := range sortedMatches {
		label := match.File
		if match.Description != "" {
			label = fmt.Sprintf("%s - %s", match.File, match.Description)
		}
		options[i] = huh.NewOption(label, match.File)
	}

	var selectedFile string

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Multiple workflows found with name '%s'. Please choose:", sortedMatches[0].Name)).
				Description("Press ctrl+c or esc to exit").
				Options(options...).
				Value(&selectedFile),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf("workflow selection failed: %w", err)
	}

	return selectedFile, nil
}

func checkAndGenerateWorkflowStepNames(workflowDefinition *schema.WorkflowDefinition) {
	steps := workflowDefinition.Steps

	if steps == nil {
		return
	}

	// Check if the steps have the `name` attribute.
	// If not, generate a friendly name consisting of a prefix of `step` and followed by the index of the
	// step (the index starts with 1, so the first generated step name would be `step1`)
	for index, step := range steps {
		if step.Name == "" {
			// When iterating through a slice with a range loop, if elements need to be changed,
			// changing the returned value from the range is not changing the original slice element.
			// That return value is a copy of the element.
			// So doing changes to it will not affect the original elements.
			// We need to access the element with the index returned from the range iterator and change it there.
			// https://medium.com/@nsspathirana/common-mistakes-with-go-slices-95f2e9b362a9
			steps[index].Name = fmt.Sprintf("step%d", index+1)
		}
	}
}

func ExecuteWorkflowUI(atmosConfig schema.AtmosConfiguration) (string, string, string, error) {
	defer perf.Track(&atmosConfig, "exec.ExecuteWorkflowUI")()

	_, _, allWorkflows, err := ExecuteDescribeWorkflows(atmosConfig)
	if err != nil {
		return "", "", "", err
	}

	// Start the UI
	app, err := w.Execute(allWorkflows)
	fmt.Println()
	if err != nil {
		return "", "", "", err
	}

	selectedWorkflowFile := app.GetSelectedWorkflowFile()
	selectedWorkflow := app.GetSelectedWorkflow()
	selectedWorkflowStep := app.GetSelectedWorkflowStep()

	// If the user quit the UI, exit
	if app.ExitStatusQuit() || selectedWorkflowFile == "" || selectedWorkflow == "" {
		return "", "", "", nil
	}

	c := fmt.Sprintf("atmos workflow %s --file %s --from-step \"%s\"", selectedWorkflow, selectedWorkflowFile, selectedWorkflowStep)
	log.Info("Executing", "command", c)

	return selectedWorkflowFile, selectedWorkflow, selectedWorkflowStep, nil
}
