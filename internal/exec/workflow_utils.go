package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/lo"

	errUtils "github.com/cloudposse/atmos/errors"
	w "github.com/cloudposse/atmos/internal/tui/workflow"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/workflow"
)

// Workflow error aliases from errors package for backward compatibility.
var (
	WorkflowErrTitle           = "Workflow Error"
	ErrWorkflowNoSteps         = errUtils.ErrWorkflowNoSteps
	ErrInvalidWorkflowStepType = errUtils.ErrInvalidWorkflowStepType
	ErrInvalidFromStep         = errUtils.ErrInvalidFromStep
	ErrWorkflowStepFailed      = errUtils.ErrWorkflowStepFailed
	ErrWorkflowNoWorkflow      = errUtils.ErrWorkflowNoWorkflow
	ErrWorkflowFileNotFound    = errUtils.ErrWorkflowFileNotFound
	ErrInvalidWorkflowManifest = errUtils.ErrInvalidWorkflowManifest

	KnownWorkflowErrors = []error{
		errUtils.ErrWorkflowNoSteps,
		errUtils.ErrInvalidWorkflowStepType,
		errUtils.ErrInvalidFromStep,
		errUtils.ErrWorkflowStepFailed,
		errUtils.ErrWorkflowNoWorkflow,
		errUtils.ErrWorkflowFileNotFound,
		errUtils.ErrInvalidWorkflowManifest,
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

// ExecuteWorkflow executes an Atmos workflow using the pkg/workflow executor.
// This function creates the appropriate adapters and delegates to the Executor.
func ExecuteWorkflow(
	atmosConfig schema.AtmosConfiguration,
	workflowName string,
	workflowPath string,
	workflowDefinition *schema.WorkflowDefinition,
	dryRun bool,
	commandLineStack string,
	fromStep string,
	commandLineIdentity string,
) error {
	defer perf.Track(&atmosConfig, "exec.ExecuteWorkflow")()

	log.Debug("Executing workflow", "workflow", workflowName, "path", workflowPath)

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		err := u.PrintAsYAMLToFileDescriptor(&atmosConfig, workflowDefinition)
		if err != nil {
			return err
		}
	}

	// Create auth provider if needed.
	var authProvider workflow.AuthProvider
	steps := workflowDefinition.Steps

	// Check if any step needs authentication.
	needsAuth := commandLineIdentity != "" || lo.SomeBy(steps, func(step schema.WorkflowStep) bool {
		return strings.TrimSpace(step.Identity) != ""
	})

	// Also check if there's a default identity configured (in atmos.yaml or stack configs).
	if !needsAuth {
		needsAuth = checkAndMergeDefaultIdentity(&atmosConfig)
	}

	if needsAuth {
		// Create a ConfigAndStacksInfo for the auth manager to populate with AuthContext.
		authStackInfo := &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{},
		}

		credStore := credentials.NewCredentialStore()
		validator := validation.NewValidator()
		authManager, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
		}
		authProvider = NewWorkflowAuthProvider(authManager)
	}

	// Create command runner - we need to handle retry per-step.
	// The runner will be created for each step with the step's retry config.
	runner := NewWorkflowCommandRunner(nil)

	// Create UI provider.
	uiProvider := NewWorkflowUIProvider()

	// Create executor with dependencies.
	executor := workflow.NewExecutor(runner, authProvider, uiProvider)

	// Build execution options.
	opts := workflow.ExecuteOptions{
		DryRun:              dryRun,
		CommandLineStack:    commandLineStack,
		FromStep:            fromStep,
		CommandLineIdentity: commandLineIdentity,
	}

	// Execute the workflow.
	params := &workflow.WorkflowParams{
		Ctx:                context.Background(),
		AtmosConfig:        &atmosConfig,
		Workflow:           workflowName,
		WorkflowPath:       workflowPath,
		WorkflowDefinition: workflowDefinition,
		Opts:               opts,
	}
	result, err := executor.Execute(params)
	if err != nil {
		log.Debug("Workflow failed", "error", err, "resumeCommand", result.ResumeCommand)
		return err
	}

	return nil
}

// FormatList formats a list of strings into a markdown bullet list.
// This is an alias to workflow.FormatList for backward compatibility.
func FormatList(items []string) string {
	return workflow.FormatList(items)
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
