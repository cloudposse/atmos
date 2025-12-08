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

	"github.com/samber/lo"
	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	w "github.com/cloudposse/atmos/internal/tui/workflow"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Static error definitions.
var (
	WorkflowErrTitle           = "Workflow Error"
	ErrWorkflowNoSteps         = errors.New("workflow has no steps defined")
	ErrInvalidWorkflowStepType = errors.New("invalid workflow step type")
	ErrInvalidFromStep         = errors.New("invalid from-step flag")
	ErrWorkflowStepFailed      = errors.New("workflow step execution failed")
	ErrWorkflowNoWorkflow      = errors.New("no workflow found")
	ErrWorkflowFileNotFound    = errors.New("workflow file not found")
	ErrInvalidWorkflowManifest = errors.New("invalid workflow manifest")

	KnownWorkflowErrors = []error{
		ErrWorkflowNoSteps,
		ErrInvalidWorkflowStepType,
		ErrInvalidFromStep,
		ErrWorkflowStepFailed,
		ErrWorkflowNoWorkflow,
		ErrWorkflowFileNotFound,
		ErrInvalidWorkflowManifest,
	}
)

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
		errUtils.CheckErrorAndPrint(
			ErrWorkflowNoSteps,
			WorkflowErrTitle,
			fmt.Sprintf("\n## Explanation\nWorkflow `%s` is empty and requires at least one step to execute.", workflow),
		)
		return ErrWorkflowNoSteps
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
			errUtils.CheckErrorAndPrint(
				ErrInvalidFromStep,
				WorkflowErrTitle,
				fmt.Sprintf("\n## Explanation\nThe `--from-step` flag was set to `%s`, but this step does not exist in workflow `%s`. \n### Available steps:\n%s", fromStep, workflow, FormatList(stepNames)),
			)
			return ErrInvalidFromStep
		}
	}

	// Create auth manager if any step has an identity or if command-line identity is specified.
	// We check once upfront to avoid repeated initialization.
	var authManager auth.AuthManager
	var authStackInfo *schema.ConfigAndStacksInfo
	needsAuth := commandLineIdentity != "" || lo.SomeBy(steps, func(step schema.WorkflowStep) bool {
		return strings.TrimSpace(step.Identity) != ""
	})

	// Also check if there's a default identity configured (in atmos.yaml or stack configs).
	// This enables workflows to use default identity without explicit --identity flag.
	if !needsAuth {
		needsAuth = checkAndMergeDefaultIdentity(&atmosConfig)
	}

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

		// Prepare environment variables if identity is specified for this step.
		var stepEnv []string
		if stepIdentity != "" {
			if authManager == nil {
				return fmt.Errorf("identity %q specified for step %q but auth manager is not initialized", stepIdentity, step.Name)
			}

			ctx := context.Background()

			// Try to use cached credentials first (passive check, no prompts).
			// Only authenticate if cached credentials are not available or expired.
			_, err := authManager.GetCachedCredentials(ctx, stepIdentity)
			if err != nil {
				log.Debug("No valid cached credentials found, authenticating", "identity", stepIdentity, "error", err)
				// No valid cached credentials - perform full authentication.
				_, err = authManager.Authenticate(ctx, stepIdentity)
				if err != nil {
					// Check for user cancellation - return clean error without wrapping.
					if errors.Is(err, errUtils.ErrUserAborted) {
						return errUtils.ErrUserAborted
					}
					return fmt.Errorf("%w for identity %q in step %q: %w", errUtils.ErrAuthenticationFailed, stepIdentity, step.Name, err)
				}
			}

			// Prepare shell environment with authentication credentials.
			// Start with current OS environment and let PrepareShellEnvironment configure auth.
			stepEnv, err = authManager.PrepareShellEnvironment(ctx, stepIdentity, os.Environ())
			if err != nil {
				return fmt.Errorf("failed to prepare shell environment for identity %q in step %q: %w", stepIdentity, step.Name, err)
			}

			log.Debug("Prepared environment with identity", "identity", stepIdentity, "step", step.Name)
		} else {
			// No identity specified, use empty environment (subprocess inherits from parent).
			stepEnv = []string{}
		}

		var err error
		if commandType == "shell" {
			commandName := fmt.Sprintf("%s-step-%d", workflow, stepIdx)
			err = ExecuteShell(command, commandName, ".", stepEnv, dryRun)
		} else if commandType == "atmos" {
			// Use shell.Fields() instead of strings.Fields() to properly handle
			// quoted arguments. This fixes issues with flags like --query that
			// contain quoted expressions with spaces (e.g., --query '.field == "value"').
			// See: https://pkg.go.dev/mvdan.cc/sh/v3/shell#Fields
			args, parseErr := shell.Fields(command, nil)
			if parseErr != nil {
				// If shell parsing fails, fall back to simple splitting.
				// This maintains backwards compatibility for simple commands.
				log.Debug("Shell parsing failed, falling back to strings.Fields", "error", parseErr, "command", command)
				args = strings.Fields(command)
			}

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

			u.PrintfMessageToTUI("Executing command: `atmos %s`\n", command)
			err = retry.With7Params(context.Background(), step.Retry,
				ExecuteShellCommand,
				atmosConfig, "atmos", args, ".", stepEnv, dryRun, "")
		} else {
			errUtils.CheckErrorAndPrint(
				ErrInvalidWorkflowStepType,
				WorkflowErrTitle,
				fmt.Sprintf("\n## Explanation\nStep type `%s` is not supported. Each step must specify a valid type. \n### Available types:\n%s", commandType, FormatList([]string{"atmos", "shell"})),
			)
			return ErrInvalidWorkflowStepType
		}

		if err != nil {
			log.Debug("Workflow failed", "error", err)

			// Remove the workflow base path, stacks/workflows
			workflowFileName := strings.TrimPrefix(filepath.ToSlash(workflowPath), filepath.ToSlash(atmosConfig.Workflows.BasePath))
			// Remove the leading slash
			workflowFileName = strings.TrimPrefix(workflowFileName, "/")
			// Remove the file extension
			workflowFileName = strings.TrimSuffix(workflowFileName, filepath.Ext(workflowFileName))

			resumeCommand := fmt.Sprintf(
				"%s workflow %s -f %s --from-step %s",
				config.AtmosCommand,
				workflow,
				workflowFileName,
				step.Name,
			)

			// Add stack parameter to resume command if a stack was used
			if finalStack != "" {
				resumeCommand = fmt.Sprintf("%s -s %s", resumeCommand, finalStack)
			}

			failedCmd := command
			if commandType == config.AtmosCommand {
				failedCmd = config.AtmosCommand + " " + command
				// Add stack parameter to failed command if a stack was used
				if finalStack != "" {
					failedCmd = fmt.Sprintf("%s -s %s", failedCmd, finalStack)
				}
			}

			errUtils.CheckErrorAndPrint(
				ErrWorkflowStepFailed,
				WorkflowErrTitle,
				fmt.Sprintf("\n## Explanation\nThe following command failed to execute:\n```\n%s\n```\nTo resume the workflow from this step, run:\n```\n%s\n```", failedCmd, resumeCommand),
			)
			// Return joined error to preserve classification and exit-code unwrapping.
			// Returning only the underlying err drops ErrWorkflowStepFailed from the error chain,
			// which can hinder classification and checks. Join both so callers can use
			// errors.Is(err, ErrWorkflowStepFailed) and still unwrap ExitCodeError for proper exit codes.
			return errors.Join(ErrWorkflowStepFailed, err)
		}
	}

	return nil
}

// FormatList formats a list of strings into a markdown bullet list.
func FormatList(items []string) string {
	var result strings.Builder
	for _, item := range items {
		result.WriteString(fmt.Sprintf("- `%s`\n", item))
	}
	return result.String()
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
			return nil, nil, nil, err
		}

		workflowManifest, err := u.UnmarshalYAML[schema.WorkflowManifest](string(fileContent))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error parsing the workflow manifest '%s': %v", f, err)
		}

		if workflowManifest.Workflows == nil {
			return nil, nil, nil, fmt.Errorf("the workflow manifest '%s' must be a map with the top-level 'workflows:' key", workflowPath)
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
