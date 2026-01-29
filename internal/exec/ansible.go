package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// AnsibleFlags represents Ansible command-line flags passed to ExecuteAnsible.
type AnsibleFlags struct {
	// Playbook specifies the Ansible playbook file to run.
	// Can be set via --playbook/-p flag or settings.ansible.playbook in stack manifest.
	Playbook string

	// Inventory specifies the Ansible inventory source.
	// Can be set via --inventory/-i flag or settings.ansible.inventory in stack manifest.
	Inventory string
}

// checkAnsibleConfig validates that the necessary Ansible configuration is present.
func checkAnsibleConfig(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.checkAnsibleConfig")()

	if atmosConfig.Components.Ansible.BasePath == "" {
		return errUtils.ErrMissingAnsibleBasePath
	}
	return nil
}

// ExecuteAnsible executes Ansible commands.
func ExecuteAnsible(
	info *schema.ConfigAndStacksInfo,
	ansibleFlags *AnsibleFlags,
) error {
	defer perf.Track(nil, "exec.ExecuteAnsible")()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	// Validate ansible configuration.
	if err := checkAnsibleConfig(&atmosConfig); err != nil {
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

	if info.SubCommand == "version" {
		return ExecuteShellCommand(
			atmosConfig,
			info.Command,
			[]string{"--version"},
			"",
			nil,
			false,
			info.RedirectStdErr,
		)
	}

	*info, err = ProcessStacks(&atmosConfig, *info, true, true, true, nil, nil)
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
		return fmt.Errorf("failed to resolve component path: %w", err)
	}

	// Auto-generate files BEFORE path validation when the following conditions hold.
	// 1. auto_generate_files is enabled.
	// 2. Component has a generate section.
	// 3. Not in dry-run mode (to avoid filesystem modifications).
	// This allows generating entire components from stack configuration.
	if atmosConfig.Components.Ansible.AutoGenerateFiles && !info.DryRun { //nolint:nestif
		generateSection := getGenerateSectionFromComponent(info.ComponentSection)
		if generateSection != nil {
			// Ensure component directory exists for file generation.
			if mkdirErr := os.MkdirAll(componentPath, 0o755); mkdirErr != nil { //nolint:revive
				return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", mkdirErr))
			}

			// Generate files before path validation.
			if genErr := generateFilesForComponent(&atmosConfig, info, componentPath); genErr != nil {
				return errors.Join(errUtils.ErrFileOperation, genErr)
			}
		}
	}

	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		// Check if component has source configured for JIT provisioning.
		if provSource.HasSource(info.ComponentSection) {
			// Run JIT source provisioning before path validation.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := provSource.AutoProvisionSource(ctx, &atmosConfig, cfg.AnsibleComponentType, info.ComponentSection, info.AuthContext); err != nil {
				return fmt.Errorf("failed to auto-provision component source: %w", err)
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
		return fmt.Errorf("failed to resolve component dependencies: %w", err)
	}

	if len(deps) > 0 {
		log.Debug("Installing component dependencies", "component", info.ComponentFromArg, "stack", info.Stack, "tools", deps)
		installer := dependencies.NewInstaller(&atmosConfig)
		if err := installer.EnsureTools(deps); err != nil {
			return fmt.Errorf("failed to install component dependencies: %w", err)
		}

		// Build PATH with toolchain binaries and add to component environment.
		// This does NOT modify the global process environment - only the subprocess environment.
		toolchainPATH, err := dependencies.BuildToolchainPATH(&atmosConfig, deps)
		if err != nil {
			return fmt.Errorf("failed to build toolchain PATH: %w", err)
		}

		// Propagate toolchain PATH into environment for subprocess.
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("PATH=%s", toolchainPATH))
	}

	// Check if the component 'settings.validation' section is specified and validate the component.
	valid, err := ValidateComponent(
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
	playbook := ansibleFlags.Playbook
	if playbook == "" {
		playbookSetting, err := GetAnsiblePlaybookFromSettings(&info.ComponentSettingsSection)
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
	inventory := ansibleFlags.Inventory
	if inventory == "" {
		inventorySetting, err := GetAnsibleInventoryFromSettings(&info.ComponentSettingsSection)
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
	varFile := constructAnsibleComponentVarfileName(info)
	varFilePath := constructAnsibleComponentVarfilePath(&atmosConfig, info)

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

	workingDir := constructAnsibleComponentWorkingDir(&atmosConfig, info)

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

	// For playbook subcommand, use ansible-playbook
	if info.SubCommand == "playbook" {
		// If playbook is not specified, return error.
		if playbook == "" {
			return errUtils.ErrAnsiblePlaybookMissing
		}

		// Add extra-vars file.
		allArgsAndFlags = append(allArgsAndFlags, []string{"--extra-vars", "@" + varFile}...)

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
	ConvertComponentEnvSectionToList(info)

	// Prepare ENV vars.
	envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	log.Debug("Using ENV", "variables", envVars)

	return ExecuteShellCommand(
		atmosConfig,
		info.Command,
		allArgsAndFlags,
		componentPath,
		envVars,
		info.DryRun,
		info.RedirectStdErr,
	)
}

// GetAnsiblePlaybookFromSettings extracts the playbook from settings.ansible.playbook.
func GetAnsiblePlaybookFromSettings(settingsSection *schema.AtmosSectionMapType) (string, error) {
	defer perf.Track(nil, "exec.GetAnsiblePlaybookFromSettings")()

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

// GetAnsibleInventoryFromSettings extracts the inventory from settings.ansible.inventory.
func GetAnsibleInventoryFromSettings(settingsSection *schema.AtmosSectionMapType) (string, error) {
	defer perf.Track(nil, "exec.GetAnsibleInventoryFromSettings")()

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

// constructAnsibleComponentVarfileName constructs the variable file name for an Ansible component.
func constructAnsibleComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	return fmt.Sprintf("%s-%s.ansible.vars.yaml", info.ContextPrefix, info.Component)
}

// constructAnsibleComponentVarfilePath constructs the full path to the variable file.
func constructAnsibleComponentVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		atmosConfig.AnsibleDirAbsolutePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		constructAnsibleComponentVarfileName(info),
	)
}

// constructAnsibleComponentWorkingDir constructs the working directory for an Ansible component.
func constructAnsibleComponentWorkingDir(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
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
