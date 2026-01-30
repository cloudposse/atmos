package ansible

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Flags represents Ansible command-line flags.
type Flags struct {
	// Playbook specifies the Ansible playbook file to run.
	// Can be set via --playbook/-p flag or settings.ansible.playbook in stack manifest.
	Playbook string

	// Inventory specifies the Ansible inventory source.
	// Can be set via --inventory/-i flag or settings.ansible.inventory in stack manifest.
	Inventory string
}

// checkConfig validates that the necessary Ansible configuration is present.
func checkConfig(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "ansible.checkConfig")()

	if atmosConfig.Components.Ansible.BasePath == "" {
		return errUtils.ErrMissingAnsibleBasePath
	}
	return nil
}

// ExecutePlaybook executes an Ansible playbook command.
func ExecutePlaybook(
	info *schema.ConfigAndStacksInfo,
	flags *Flags,
) error {
	defer perf.Track(nil, "ansible.ExecutePlaybook")()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	// Validate ansible configuration.
	if err := checkConfig(&atmosConfig); err != nil {
		return err
	}

	// Add the `command` from `components.ansible.command` from `atmos.yaml`.
	if info.Command == "" {
		if atmosConfig.Components.Ansible.Command != "" {
			info.Command = atmosConfig.Components.Ansible.Command
		} else {
			info.Command = cfg.AnsibleComponentType
		}
	}

	*info, err = e.ProcessStacks(&atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errUtils.ErrMissingStack
	}

	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	// Check if the component exists as an Ansible component.
	componentPath, err := u.GetComponentPath(&atmosConfig, "ansible", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return errors.Join(errUtils.ErrPathResolution, fmt.Errorf("component path: %w", err))
	}

	// Auto-generate files BEFORE path validation.
	// This allows generating entire components from stack configuration.
	if err := maybeAutoGenerateFiles(&atmosConfig, info, componentPath); err != nil {
		return err
	}

	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		// Check if component has source configured for JIT provisioning.
		if provSource.HasSource(info.ComponentSection) {
			// Run JIT source provisioning before path validation.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := provSource.AutoProvisionSource(ctx, &atmosConfig, cfg.AnsibleComponentType, info.ComponentSection, info.AuthContext); err != nil {
				return errors.Join(errUtils.ErrProvisionerFailed, fmt.Errorf("auto-provision source: %w", err))
			}

			// Check if source provisioner set a workdir path (source + workdir case).
			// If so, use that path instead of the component path.
			if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok {
				componentPath = workdirPath
				componentPathExists = true
				err = nil // Clear any previous error since we have a valid workdir path.
			} else {
				// Re-check if component path now exists after provisioning (source only case).
				componentPathExists, err = u.IsDirectory(componentPath)
			}
		}

		// If still doesn't exist, return the error.
		if err != nil || !componentPathExists {
			// Get the base path for the error message, respecting the user's actual config.
			basePath, _ := u.GetComponentBasePath(&atmosConfig, "ansible")
			return fmt.Errorf("%w: '%s' points to the Ansible component '%s', but it does not exist in '%s'",
				errUtils.ErrInvalidComponent,
				info.ComponentFromArg,
				info.FinalComponent,
				basePath,
			)
		}
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute).
	// For Ansible, only `playbook` runs automation that changes infrastructure.
	if info.ComponentIsAbstract && info.SubCommand == "playbook" {
		return fmt.Errorf("%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
			errUtils.ErrAbstractComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true).
	if info.ComponentIsLocked && info.SubCommand == "playbook" {
		return fmt.Errorf("%w: component '%s' cannot be modified (metadata.locked: true)",
			errUtils.ErrLockedComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Resolve and install component dependencies.
	resolver := dependencies.NewResolver(&atmosConfig)
	deps, err := resolver.ResolveComponentDependencies("ansible", info.StackSection, info.ComponentSection)
	if err != nil {
		return errors.Join(errUtils.ErrDependencyResolution, err)
	}

	if len(deps) > 0 {
		log.Debug("Installing component dependencies", "component", info.ComponentFromArg, "stack", info.Stack, "tools", deps)
		installer := dependencies.NewInstaller(&atmosConfig)
		if err := installer.EnsureTools(deps); err != nil {
			return errors.Join(errUtils.ErrDependencyResolution, fmt.Errorf("install dependencies: %w", err))
		}

		// Build PATH with toolchain binaries and add to component environment.
		// This does NOT modify the global process environment - only the subprocess environment.
		toolchainPATH, err := dependencies.BuildToolchainPATH(&atmosConfig, deps)
		if err != nil {
			return errors.Join(errUtils.ErrPathResolution, fmt.Errorf("toolchain PATH: %w", err))
		}

		// Propagate toolchain PATH into environment for subprocess.
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("PATH=%s", toolchainPATH))
	}

	// Check if the component 'settings.validation' section is specified and validate the component.
	valid, err := e.ValidateComponent(
		&atmosConfig,
		info.ComponentFromArg,
		info.ComponentSection,
		"",
		"",
		nil,
		0,
	)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: the component '%s' did not pass the validation policies",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
		)
	}

	// Find Ansible playbook.
	// It can be specified in the `settings.ansible.playbook` section in the Atmos component manifest,
	// or on the command line via the flag `--playbook <playbook>` (shorthand `-p`).
	playbook := flags.Playbook
	if playbook == "" {
		playbookSetting, err := GetPlaybookFromSettings(&info.ComponentSettingsSection)
		if err != nil {
			return err
		}

		if playbookSetting != "" {
			playbook = playbookSetting
		}
	}

	// Find Ansible inventory.
	// It can be specified in the `settings.ansible.inventory` section in the Atmos component manifest,
	// or on the command line via the flag `--inventory <inventory>` (shorthand `-i`).
	inventory := flags.Inventory
	if inventory == "" {
		inventorySetting, err := GetInventoryFromSettings(&info.ComponentSettingsSection)
		if err != nil {
			return err
		}

		if inventorySetting != "" {
			inventory = inventorySetting
		}
	}

	// Print component variables.
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack, "variables", info.ComponentVarsSection)

	// Write variables to a file.
	varFilePath := constructVarfilePath(&atmosConfig, info)

	log.Debug("Writing the variables to file", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}

		// Defer cleanup of the variable file.
		// Use a closure to capture varFilePath and ensure cleanup runs even on early errors.
		defer func() {
			if removeErr := os.Remove(varFilePath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Trace("Failed to remove var file during cleanup", "error", removeErr, "file", varFilePath)
			}
		}()
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	workingDir := constructWorkingDir(&atmosConfig, info)

	log.Debug("Ansible context",
		"executable", info.Command,
		"command", info.SubCommand,
		"atmos component", info.ComponentFromArg,
		"atmos stack", info.StackFromArg,
		"ansible component", info.BaseComponentPath,
		"ansible playbook", playbook,
		"ansible inventory", inventory,
		"working directory", workingDir,
		"inheritance", inheritance,
		"arguments and flags", info.AdditionalArgsAndFlags,
	)

	// Prepare arguments and flags.
	allArgsAndFlags := []string{}

	// For playbook subcommand, use ansible-playbook.
	if info.SubCommand == "playbook" {
		// If playbook is not specified, return error.
		if playbook == "" {
			return errUtils.ErrAnsiblePlaybookMissing
		}

		// Add extra-vars file using full path for workdir/source provisioning scenarios.
		allArgsAndFlags = append(allArgsAndFlags, []string{"--extra-vars", "@" + varFilePath}...)

		// Add inventory if specified.
		if inventory != "" {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-i", inventory}...)
		}

		// Add any additional args and flags.
		allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

		// Add the playbook as the last argument.
		allArgsAndFlags = append(allArgsAndFlags, playbook)

		// Override command to ansible-playbook for playbook subcommand.
		info.Command = "ansible-playbook"
	} else {
		// For other subcommands, use the base ansible command with the subcommand.
		allArgsAndFlags = append(allArgsAndFlags, info.SubCommand)
		allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)
	}

	// Convert ComponentEnvSection to ComponentEnvList.
	e.ConvertComponentEnvSectionToList(info)

	// Prepare ENV vars.
	envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	log.Debug("Using ENV", "variables", envVars)

	return e.ExecuteShellCommand(
		atmosConfig,
		info.Command,
		allArgsAndFlags,
		componentPath,
		envVars,
		info.DryRun,
		info.RedirectStdErr,
	)
}

// ExecuteVersion executes the ansible version command.
func ExecuteVersion(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "ansible.ExecuteVersion")()

	atmosConfig, err := cfg.InitCliConfig(*info, false)
	if err != nil {
		return err
	}

	// Get ansible command from config, defaulting to "ansible".
	command := atmosConfig.Components.Ansible.Command
	if command == "" {
		command = "ansible"
	}

	// Execute ansible --version directly.
	return e.ExecuteShellCommand(
		atmosConfig,
		command,
		[]string{"--version"},
		"",    // dir
		nil,   // env
		false, // dryRun
		"",    // redirectStdError
	)
}

// GetPlaybookFromSettings extracts the playbook from settings.ansible.playbook.
func GetPlaybookFromSettings(settingsSection *schema.AtmosSectionMapType) (string, error) {
	defer perf.Track(nil, "ansible.GetPlaybookFromSettings")()

	if settingsSection == nil {
		return "", nil
	}

	ansibleSection, ok := (*settingsSection)["ansible"].(map[string]any)
	if !ok {
		return "", nil
	}

	playbook, ok := ansibleSection["playbook"].(string)
	if !ok {
		return "", nil
	}

	return playbook, nil
}

// GetInventoryFromSettings extracts the inventory from settings.ansible.inventory.
func GetInventoryFromSettings(settingsSection *schema.AtmosSectionMapType) (string, error) {
	defer perf.Track(nil, "ansible.GetInventoryFromSettings")()

	if settingsSection == nil {
		return "", nil
	}

	ansibleSection, ok := (*settingsSection)["ansible"].(map[string]any)
	if !ok {
		return "", nil
	}

	inventory, ok := ansibleSection["inventory"].(string)
	if !ok {
		return "", nil
	}

	return inventory, nil
}

// constructVarfileName constructs the variable file name for an Ansible component.
// Component names containing path separators are sanitized to avoid creating nested directories.
func constructVarfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "ansible.constructVarfileName")()

	// Sanitize component name by replacing path separators with dashes.
	// This ensures the varfile name is a flat filename, not a nested path.
	sanitizedComponent := strings.ReplaceAll(info.Component, "/", "-")
	sanitizedComponent = strings.ReplaceAll(sanitizedComponent, string(filepath.Separator), "-")
	return fmt.Sprintf("%s-%s.ansible.vars.yaml", info.ContextPrefix, sanitizedComponent)
}

// constructVarfilePath constructs the full path to the variable file.
func constructVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "ansible.constructVarfilePath")()

	return filepath.Join(
		atmosConfig.AnsibleDirAbsolutePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		constructVarfileName(info),
	)
}

// constructWorkingDir constructs the working directory for an Ansible component.
func constructWorkingDir(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "ansible.constructWorkingDir")()

	return filepath.Join(
		atmosConfig.AnsibleDirAbsolutePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)
}

// getGenerateSectionFromComponent extracts the generate section from a component configuration.
// Returns nil if the component has no generate section defined.
func getGenerateSectionFromComponent(componentSection map[string]any) map[string]any {
	if componentSection == nil {
		return nil
	}

	generateSection, ok := componentSection["generate"].(map[string]any)
	if !ok {
		return nil
	}

	return generateSection
}

// maybeAutoGenerateFiles conditionally generates files for a component before path validation.
// It generates files when:
//   - auto_generate_files is enabled in the Ansible configuration
//   - the component has a generate section
//   - not in dry-run mode (to avoid filesystem modifications)
//
// Returns nil if generation is skipped or succeeds, error otherwise.
func maybeAutoGenerateFiles(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentPath string,
) error {
	defer perf.Track(atmosConfig, "ansible.maybeAutoGenerateFiles")()

	// Skip if auto-generation is disabled or in dry-run mode.
	if !atmosConfig.Components.Ansible.AutoGenerateFiles || info.DryRun {
		return nil
	}

	// Skip if component has no generate section.
	generateSection := getGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		return nil
	}

	// Ensure component directory exists for file generation.
	if mkdirErr := os.MkdirAll(componentPath, 0o755); mkdirErr != nil { //nolint:revive
		return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", mkdirErr))
	}

	// Generate files before path validation.
	if genErr := e.GenerateFilesForComponent(atmosConfig, info, componentPath); genErr != nil {
		return errors.Join(errUtils.ErrFileOperation, genErr)
	}

	return nil
}

// resolveComponentPath resolves the component path, handling JIT source provisioning if needed.
// It returns the resolved component path or an error if the component cannot be found.
func resolveComponentPath(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	initialPath string,
) (string, error) {
	defer perf.Track(atmosConfig, "ansible.resolveComponentPath")()

	componentPath := initialPath
	componentPathExists, err := u.IsDirectory(componentPath)

	// If path exists, return it directly.
	if err == nil && componentPathExists {
		return componentPath, nil
	}

	// Check if component has source configured for JIT provisioning.
	if provSource.HasSource(info.ComponentSection) {
		// Run JIT source provisioning before path validation.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if provErr := provSource.AutoProvisionSource(ctx, atmosConfig, cfg.AnsibleComponentType, info.ComponentSection, info.AuthContext); provErr != nil {
			return "", errors.Join(errUtils.ErrProvisionerFailed, fmt.Errorf("auto-provision source: %w", provErr))
		}

		// Check if source provisioner set a workdir path (source + workdir case).
		// If so, use that path instead of the component path.
		if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok {
			return workdirPath, nil
		}

		// Re-check if component path now exists after provisioning (source only case).
		componentPathExists, err = u.IsDirectory(componentPath)
		if err == nil && componentPathExists {
			return componentPath, nil
		}
	}

	// If still doesn't exist, return the error.
	basePath, _ := u.GetComponentBasePath(atmosConfig, "ansible")
	return "", fmt.Errorf("%w: '%s' points to the Ansible component '%s', but it does not exist in '%s'",
		errUtils.ErrInvalidComponent,
		info.ComponentFromArg,
		info.FinalComponent,
		basePath,
	)
}

// validateComponentMetadata checks if the component can be provisioned based on metadata.
// Returns an error if the component is abstract or locked and the subcommand is "playbook".
func validateComponentMetadata(info *schema.ConfigAndStacksInfo) error {
	// Check if the component is allowed to be provisioned (`metadata.type` attribute).
	// For Ansible, only `playbook` runs automation that changes infrastructure.
	if info.ComponentIsAbstract && info.SubCommand == "playbook" {
		return fmt.Errorf("%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
			errUtils.ErrAbstractComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true).
	if info.ComponentIsLocked && info.SubCommand == "playbook" {
		return fmt.Errorf("%w: component '%s' cannot be modified (metadata.locked: true)",
			errUtils.ErrLockedComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	return nil
}

// ensureDependencies resolves and installs component dependencies, returning the updated environment list.
// If dependencies are found, it installs them and adds the toolchain PATH to the environment.
func ensureDependencies(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
) ([]string, error) {
	defer perf.Track(atmosConfig, "ansible.ensureDependencies")()

	resolver := dependencies.NewResolver(atmosConfig)
	deps, err := resolver.ResolveComponentDependencies("ansible", info.StackSection, info.ComponentSection)
	if err != nil {
		return nil, errors.Join(errUtils.ErrDependencyResolution, err)
	}

	envList := info.ComponentEnvList

	if len(deps) > 0 {
		log.Debug("Installing component dependencies", "component", info.ComponentFromArg, "stack", info.Stack, "tools", deps)
		installer := dependencies.NewInstaller(atmosConfig)
		if err := installer.EnsureTools(deps); err != nil {
			return nil, errors.Join(errUtils.ErrDependencyResolution, fmt.Errorf("install dependencies: %w", err))
		}

		// Build PATH with toolchain binaries and add to component environment.
		// This does NOT modify the global process environment - only the subprocess environment.
		toolchainPATH, err := dependencies.BuildToolchainPATH(atmosConfig, deps)
		if err != nil {
			return nil, errors.Join(errUtils.ErrPathResolution, fmt.Errorf("toolchain PATH: %w", err))
		}

		// Propagate toolchain PATH into environment for subprocess.
		envList = append(envList, fmt.Sprintf("PATH=%s", toolchainPATH))
	}

	return envList, nil
}

// PlaybookConfig holds the resolved playbook and inventory configuration.
type PlaybookConfig struct {
	Playbook  string
	Inventory string
}

// resolvePlaybookConfig resolves the playbook and inventory from flags and settings.
// Flag values take precedence over settings values.
func resolvePlaybookConfig(
	flags *Flags,
	settingsSection *schema.AtmosSectionMapType,
) (*PlaybookConfig, error) {
	defer perf.Track(nil, "ansible.resolvePlaybookConfig")()

	config := &PlaybookConfig{}

	// Resolve playbook: flag takes precedence over settings.
	if flags.Playbook != "" {
		config.Playbook = flags.Playbook
	} else {
		playbookSetting, err := GetPlaybookFromSettings(settingsSection)
		if err != nil {
			return nil, err
		}
		config.Playbook = playbookSetting
	}

	// Resolve inventory: flag takes precedence over settings.
	if flags.Inventory != "" {
		config.Inventory = flags.Inventory
	} else {
		inventorySetting, err := GetInventoryFromSettings(settingsSection)
		if err != nil {
			return nil, err
		}
		config.Inventory = inventorySetting
	}

	return config, nil
}

// CommandArgs holds the command and arguments for execution.
type CommandArgs struct {
	Command string
	Args    []string
}

// buildCommandArgs builds the command and arguments based on subcommand type.
// For "playbook" subcommand, it uses ansible-playbook with appropriate flags.
// For other subcommands, it uses the base ansible command.
func buildCommandArgs(
	info *schema.ConfigAndStacksInfo,
	playbookConfig *PlaybookConfig,
	varFilePath string,
) (*CommandArgs, error) {
	defer perf.Track(nil, "ansible.buildCommandArgs")()

	result := &CommandArgs{
		Command: info.Command,
		Args:    []string{},
	}

	if info.SubCommand == "playbook" {
		// If playbook is not specified, return error.
		if playbookConfig.Playbook == "" {
			return nil, errUtils.ErrAnsiblePlaybookMissing
		}

		// Add extra-vars file using full path for workdir/source provisioning scenarios.
		result.Args = append(result.Args, "--extra-vars", "@"+varFilePath)

		// Add inventory if specified.
		if playbookConfig.Inventory != "" {
			result.Args = append(result.Args, "-i", playbookConfig.Inventory)
		}

		// Add any additional args and flags.
		result.Args = append(result.Args, info.AdditionalArgsAndFlags...)

		// Add the playbook as the last argument.
		result.Args = append(result.Args, playbookConfig.Playbook)

		// Override command to ansible-playbook for playbook subcommand.
		result.Command = "ansible-playbook"
	} else {
		// For other subcommands, use the base ansible command with the subcommand.
		result.Args = append(result.Args, info.SubCommand)
		result.Args = append(result.Args, info.AdditionalArgsAndFlags...)
	}

	return result, nil
}

// prepareEnvVars prepares the environment variables for command execution.
func prepareEnvVars(atmosConfig *schema.AtmosConfiguration, envList []string) ([]string, error) {
	defer perf.Track(atmosConfig, "ansible.prepareEnvVars")()

	envVars := append(envList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))

	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return nil, err
	}

	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	return envVars, nil
}
