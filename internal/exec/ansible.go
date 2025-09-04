package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// AnsibleFlags type represents Ansible command-line flags.
type AnsibleFlags struct {
	Playbook  string
	Inventory string
}

// validateAnsibleComponent checks if the Ansible component exists.
func validateAnsibleComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	componentPath := filepath.Join(atmosConfig.AnsibleDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("%w: Atmos component `%s` points to the Ansible component `%s`, but it does not exist in `%s`",
			errUtils.ErrAnsibleComponentMissing,
			info.ComponentFromArg,
			info.FinalComponent,
			filepath.Join(atmosConfig.Components.Ansible.BasePath, info.ComponentFolderPrefix),
		)
	}
	return nil
}

// validateComponentPermissions checks if the component is allowed to be provisioned.
func validateComponentPermissions(info *schema.ConfigAndStacksInfo) error {
	// Check if the component is allowed to be provisioned (`metadata.type` attribute).
	if info.SubCommand == "playbook" && info.ComponentIsAbstract {
		return fmt.Errorf("%w: component `%s` is abstract and cannot be provisioned (`metadata.type = abstract`)",
			errUtils.ErrAbstractComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true).
	if info.ComponentIsLocked {
		// Allow read-only commands, block modification commands.
		switch info.SubCommand {
		case "playbook":
			return fmt.Errorf("%w: component `%s` is locked and cannot be modified (`metadata.locked = true`)",
				errUtils.ErrLockedComponentCantBeProvisioned,
				filepath.Join(info.ComponentFolderPrefix, info.Component))
		}
	}
	return nil
}

// resolveAnsibleSettings resolves playbook and inventory from flags and component settings.
func resolveAnsibleSettings(ansibleFlags *AnsibleFlags, settingsSection *schema.AtmosSectionMapType) (string, string, error) {
	// Find Ansible playbook.
	// It can be specified in the `settings.ansible.playbook` section in the Atmos component manifest,
	// or on the command line via the flag `--playbook <playbook> (shorthand `-p`)`.
	playbook := ansibleFlags.Playbook
	if playbook == "" {
		ansibleSettingPlaybook, err := GetAnsiblePlaybookFromSettings(settingsSection)
		if err != nil {
			return "", "", err
		}

		if ansibleSettingPlaybook != "" {
			playbook = ansibleSettingPlaybook
		}
	}

	// Find Ansible inventory.
	// It can be specified in the `settings.ansible.inventory` section in the Atmos component manifest,
	// or on the command line via the flag `--inventory <inventory> (shorthand `-i`)`.
	inventory := ansibleFlags.Inventory
	if inventory == "" {
		ansibleSettingInventory, err := GetAnsibleInventoryFromSettings(settingsSection)
		if err != nil {
			return "", "", err
		}

		if ansibleSettingInventory != "" {
			inventory = ansibleSettingInventory
		}
	}

	return playbook, inventory, nil
}

// prepareAnsibleArgs prepares command arguments based on the subcommand.
func prepareAnsibleArgs(info *schema.ConfigAndStacksInfo, playbook, inventory, varFile string) ([]string, error) {
	allArgsAndFlags := []string{}

	// Handle different subcommands.
	switch info.SubCommand {
	case "playbook":
		if playbook != "" {
			allArgsAndFlags = append(allArgsAndFlags, playbook)
		}
		if inventory != "" {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-i", inventory}...)
		}
		allArgsAndFlags = append(allArgsAndFlags, []string{"-e", "@" + varFile}...)
	case "inventory":
		info.Command = "ansible-inventory"
		if inventory != "" {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-i", inventory}...)
		}
	case "vault":
		info.Command = "ansible-vault"
	}

	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)
	return allArgsAndFlags, nil
}

// prepareAnsibleEnvVars prepares environment variables for Ansible execution.
func prepareAnsibleEnvVars(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) ([]string, error) {
	envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPathResolution, err)
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	log.Debug("Using ENV", "variables", envVars)
	return envVars, nil
}

// ExecuteAnsible executes Ansible commands.
func ExecuteAnsible(
	info *schema.ConfigAndStacksInfo,
	ansibleFlags *AnsibleFlags,
) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
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

	*info, err = ProcessStacks(&atmosConfig, *info, true, true, true, nil)
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
	err = validateAnsibleComponent(&atmosConfig, info)
	if err != nil {
		return err
	}

	// Check if the component is allowed to be provisioned.
	err = validateComponentPermissions(info)
	if err != nil {
		return err
	}

	// Resolve Ansible playbook and inventory settings.
	playbook, inventory, err := resolveAnsibleSettings(ansibleFlags, &info.ComponentSettingsSection)
	if err != nil {
		return err
	}

	// Print component variables
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack, "variables", info.ComponentVarsSection)

	// Write variables to a file
	varFile := constructAnsibleComponentVarfileName(info)
	varFilePath := constructAnsibleComponentVarfilePath(&atmosConfig, info)

	log.Debug("Writing the variables to file:", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	componentPath := filepath.Join(atmosConfig.AnsibleDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
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

	// Prepare command arguments and flags.
	allArgsAndFlags, err := prepareAnsibleArgs(info, playbook, inventory, varFile)
	if err != nil {
		return err
	}

	// Prepare environment variables.
	envVars, err := prepareAnsibleEnvVars(&atmosConfig, info)
	if err != nil {
		return err
	}

	err = ExecuteShellCommand(
		atmosConfig,
		info.Command,
		allArgsAndFlags,
		componentPath,
		envVars,
		info.DryRun,
		info.RedirectStdErr,
	)
	if err != nil {
		return err
	}

	// Cleanup
	if info.SubCommand == "playbook" && !info.DryRun {
		err = os.Remove(varFilePath)
		if err != nil {
			log.Warn(err.Error())
		}
	}

	return nil
}
