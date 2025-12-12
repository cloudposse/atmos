package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	u "github.com/cloudposse/atmos/pkg/utils"
	wfpkg "github.com/cloudposse/atmos/pkg/workflow"
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
	ErrNoWorkflowFilesToSelect = errUtils.ErrNoWorkflowFilesToSelect
	ErrNonTTYWorkflowSelection = errUtils.ErrNonTTYWorkflowSelection

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
	var authProvider wfpkg.AuthProvider

	// Validate workflow definition exists and has steps.
	if workflowDefinition == nil || len(workflowDefinition.Steps) == 0 {
		return errUtils.Build(errUtils.ErrWorkflowNoSteps).
			WithTitle(WorkflowErrTitle).
			WithExplanationf("Workflow `%s` has no steps defined", workflowName).
			WithContext("workflow", workflowName).
			WithContext("path", workflowPath).
			WithExitCode(1).
			Err()
	}

	steps := workflowDefinition.Steps

	// Ensure toolchain dependencies are installed and build PATH for workflow steps.
	_, err := ensureWorkflowToolchainDependencies(&atmosConfig, workflowDefinition)
	if err != nil {
		return err
	}

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
	executor := wfpkg.NewExecutor(runner, authProvider, uiProvider)

	// Build execution options.
	opts := wfpkg.ExecuteOptions{
		DryRun:              dryRun,
		CommandLineStack:    commandLineStack,
		FromStep:            fromStep,
		CommandLineIdentity: commandLineIdentity,
	}

	// Execute the workflow.
	params := &wfpkg.WorkflowParams{
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
// This is an alias to u.FormatList for backward compatibility.
func FormatList(items []string) string {
	return u.FormatList(items)
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
			return nil, nil, nil, errUtils.Build(errUtils.ErrInvalidWorkflowManifest).
				WithCause(err).
				WithExplanation(fmt.Sprintf("error parsing the workflow manifest '%s'", f)).
				Err()
		}

		if workflowManifest.Workflows == nil {
			return nil, nil, nil, errUtils.Build(errUtils.ErrInvalidWorkflowManifest).
				WithExplanation(fmt.Sprintf("the workflow manifest '%s' must be a map with the top-level 'workflows:' key", workflowPath)).
				Err()
		}

		workflowConfig := workflowManifest.Workflows
		allWorkflowsInFile := lo.Keys(workflowConfig)
		sort.Strings(allWorkflowsInFile)

		// Check if the workflow steps have the `name` attribute.
		lo.ForEach(allWorkflowsInFile, func(item string, _ int) {
			workflowDefinition := workflowConfig[item]
			wfpkg.CheckAndGenerateWorkflowStepNames(&workflowDefinition)
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

// ensureWorkflowToolchainDependencies loads and installs toolchain dependencies for a workflow.
// It loads tools from .tool-versions, merges with workflow-specific dependencies, installs missing
// tools, and returns the PATH string with toolchain binaries prepended.
func ensureWorkflowToolchainDependencies(
	atmosConfig *schema.AtmosConfiguration,
	workflowDefinition *schema.WorkflowDefinition,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ensureWorkflowToolchainDependencies")()

	// Load project-wide tools from .tool-versions.
	toolVersionsDeps, err := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrDependencyResolution).
			WithCause(err).
			WithExplanation("Failed to load .tool-versions file").
			Err()
	}

	// Get workflow-specific dependencies.
	resolver := dependencies.NewResolver(atmosConfig)
	workflowDeps, err := resolver.ResolveWorkflowDependencies(workflowDefinition)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrDependencyResolution).
			WithCause(err).
			WithExplanation("Failed to resolve workflow dependencies").
			Err()
	}

	// Merge: .tool-versions as base, workflow deps override.
	deps, err := dependencies.MergeDependencies(toolVersionsDeps, workflowDeps)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrDependencyResolution).
			WithCause(err).
			WithExplanation("Failed to merge dependencies").
			Err()
	}

	if len(deps) == 0 {
		return "", nil
	}

	log.Debug("Installing workflow dependencies", "tools", deps)

	// Install missing tools.
	installer := dependencies.NewInstaller(atmosConfig)
	if err := installer.EnsureTools(deps); err != nil {
		return "", errUtils.Build(errUtils.ErrToolInstall).
			WithCause(err).
			WithExplanation("Failed to install workflow dependencies").
			Err()
	}

	// Build PATH with toolchain binaries.
	toolchainPATH, err := dependencies.BuildToolchainPATH(atmosConfig, deps)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrDependencyResolution).
			WithCause(err).
			WithExplanation("Failed to build toolchain PATH").
			Err()
	}

	return toolchainPATH, nil
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
