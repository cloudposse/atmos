package exec

import (
	"fmt"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteAnsibleGalaxy runs `ansible-galaxy` within the component directory, primarily for install operations.
func ExecuteAnsibleGalaxy(info *schema.ConfigAndStacksInfo) error {
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

    // if requirements.yml exists or configured in settings, run install by default
    args := info.AdditionalArgsAndFlags
    if len(args) == 0 {
        args = []string{"install", "-r", "requirements.yml"}
    }

    envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
    basePath, err := filepath.Abs(atmosConfig.BasePath)
    if err != nil {
        return err
    }
    envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))

    return ExecuteShellCommand(
        atmosConfig,
        "ansible-galaxy",
        args,
        componentPath,
        envVars,
        info.DryRun,
        info.RedirectStdErr,
    )
}


