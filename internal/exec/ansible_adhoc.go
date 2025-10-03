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

// ExecuteAnsibleAdhoc runs `ansible` ad-hoc commands with component vars and inventory.
func ExecuteAnsibleAdhoc(info *schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
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

	componentPath, err := u.GetComponentPath(&atmosConfig, cfg.AnsibleComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}
	if exists, err := u.IsDirectory(componentPath); err != nil || !exists {
		basePath, _ := u.GetComponentBasePath(&atmosConfig, cfg.AnsibleComponentType)
		return fmt.Errorf("%w: Atmos component `%s` points to the Ansible component `%s`, but it does not exist in `%s`",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	inventory, _ := GetAnsibleInventoryFromSettings(&info.ComponentSettingsSection)
	if inventory == "" {
		inventory = "inventory"
	}

	// Write vars to YAML
	varFile := constructAnsibleComponentVarfileName(info)
	varFilePath := constructAnsibleComponentVarfilePath(&atmosConfig, info)
	if !info.DryRun {
		if err := u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644); err != nil {
			return err
		}
	}

	args := []string{"-i", inventory, "-e", "@" + varFile}
	args = append(args, info.AdditionalArgsAndFlags...)

	envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePathAbs, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePathAbs))

	err = ExecuteShellCommand(
		atmosConfig,
		"ansible",
		args,
		componentPath,
		envVars,
		info.DryRun,
		info.RedirectStdErr,
	)
	if err != nil {
		return err
	}

	_ = os.Remove(varFilePath)
	return nil
}
