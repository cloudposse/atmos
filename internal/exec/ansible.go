package exec

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// AnsibleFlags represents Ansible command-line flags.
type AnsibleFlags struct {
	Playbook string
}

// ExecuteAnsible executes Ansible commands via `ansible-playbook`.
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
			info.Command = "ansible-playbook"
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
	componentPath, err := u.GetComponentPath(&atmosConfig, cfg.AnsibleComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}

	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		basePath, _ := u.GetComponentBasePath(&atmosConfig, cfg.AnsibleComponentType)
		return fmt.Errorf("%w: Atmos component `%s` points to the Ansible component `%s`, but it does not exist in `%s`",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute)
	if info.ComponentIsAbstract {
		return fmt.Errorf("%w: component `%s` is abstract and cannot be provisioned (`metadata.type = abstract`)",
			errUtils.ErrAbstractComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	if info.ComponentIsLocked {
		return fmt.Errorf("%w: component `%s` is locked and cannot be modified (`metadata.locked = true`)",
			errUtils.ErrLockedComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Find Ansible playbook.
	playbook := ansibleFlags.Playbook
	if playbook == "" {
		p, err := GetAnsiblePlaybookFromSettings(&info.ComponentSettingsSection)
		if err != nil {
			return err
		}
		playbook = p
	}
	if playbook == "" {
		return fmt.Errorf("ansible playbook is required; specify `settings.ansible.playbook` or pass with -- [playbook options]")
	}

	// Print component variables
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack, "variables", info.ComponentVarsSection)

	// Write vars to a file (YAML)
	varFile := constructAnsibleComponentVarfileName(info)
	varFilePath := constructAnsibleComponentVarfilePath(&atmosConfig, info)
	log.Debug("Writing the variables to file:", "file", varFilePath)
	if !info.DryRun {
		err = u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	workingDir := constructAnsibleComponentWorkingDir(&atmosConfig, info)
	log.Debug("Ansible context",
		"executable", info.Command,
		"atmos component", info.ComponentFromArg,
		"atmos stack", info.StackFromArg,
		"ansible component", info.BaseComponentPath,
		"playbook", playbook,
		"working directory", workingDir,
		"arguments and flags", info.AdditionalArgsAndFlags,
	)

	// Prepare args: ansible-playbook -e @varfile playbook.yml + trailing args
	allArgsAndFlags := []string{"-e", "@" + varFile}
	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)
	allArgsAndFlags = append(allArgsAndFlags, playbook)

	// ENV vars
	envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))

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
	if rmErr := os.Remove(varFilePath); rmErr != nil {
		log.Warn(rmErr.Error())
	}

	return nil
}
