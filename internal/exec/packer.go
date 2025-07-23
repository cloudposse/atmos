package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecutePacker executes Packer commands.
func ExecutePacker(info schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	// Add the `command` from `components.packer.command` from `atmos.yaml`.
	if info.Command == "" {
		if atmosConfig.Components.Packer.Command != "" {
			info.Command = atmosConfig.Components.Packer.Command
		} else {
			info.Command = cfg.PackerComponentType
		}
	}

	if info.SubCommand == "version" {
		return ExecuteShellCommand(
			info.Command,
			[]string{info.SubCommand},
			"",
			nil,
			false,
			info.RedirectStdErr,
		)
	}

	info, err = ProcessStacks(&atmosConfig, info, true, true, true, nil)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errors.New("stack must be specified")
	}

	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	// Check if the component exists as a Packer component.
	componentPath := filepath.Join(atmosConfig.PackerDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("'%s' points to the Packer component '%s', but it does not exist in '%s'",
			info.ComponentFromArg,
			info.FinalComponent,
			filepath.Join(atmosConfig.Components.Packer.BasePath, info.ComponentFolderPrefix),
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute)
	if (info.SubCommand == "build") && info.ComponentIsAbstract {
		return fmt.Errorf("abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true)
	if info.ComponentIsLocked {
		// Allow read-only commands, block modification commands
		switch info.SubCommand {
		case "build":
			return fmt.Errorf("component `%s` is locked and cannot be modified (metadata.locked = true)",
				filepath.Join(info.ComponentFolderPrefix, info.Component))
		}
	}

	// Print component variables
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack, "variables", info.ComponentVarsSection)

	// Write variables to a file
	varFile := constructPackerComponentVarfileName(&info)
	varFilePath := constructPackerComponentVarfilePath(&atmosConfig, &info)

	log.Debug("Writing the variables to file:", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	workingDir := constructPackerComponentWorkingDir(&atmosConfig, &info)

	log.Debug("Packer context",
		"executable", info.Command,
		"command", info.SubCommand,
		"component", info.ComponentFromArg,
		"stack", info.StackFromArg,
		"arguments and flags", info.AdditionalArgsAndFlags,
		"packer component", info.BaseComponentPath,
		"inheritance", inheritance,
		"working directory", workingDir,
	)

	// Prepare arguments and flags
	allArgsAndFlags := []string{}
	allArgsAndFlags = append(allArgsAndFlags, info.SubCommand)
	allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Prepare ENV vars
	envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	log.Debug("Using ENV", "variables", envVars)

	err = ExecuteShellCommand(
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
	err = os.Remove(varFilePath)
	if err != nil {
		log.Warn(err.Error())
	}

	return nil
}
