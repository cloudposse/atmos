package exec

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformShell starts an interactive shell configured for a terraform component.
func ExecuteTerraformShell(
	component, stack string,
	processTemplates, processFunctions bool,
	skip []string,
	atmosConfig *schema.AtmosConfiguration,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteShell")()

	log.Debug("ExecuteShell called",
		"component", component,
		"stack", stack,
		"processTemplates", processTemplates,
		"processFunctions", processFunctions,
		"skip", skip,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
		StackFromArg:     stack,
		ComponentType:    "terraform",
		SubCommand:       "shell",
	}

	// Process stacks to get component configuration.
	info, err := ProcessStacks(atmosConfig, info, true, processTemplates, processFunctions, skip, nil)
	if err != nil {
		return err
	}

	// Get the component path.
	componentPath, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return err
	}

	// Get the working directory.
	workingDir := constructTerraformComponentWorkingDir(atmosConfig, &info)

	// Get the varfile name.
	varFile := constructTerraformComponentVarfileName(&info)

	// Write variables to varfile if not in dry run mode.
	if !info.DryRun {
		varFilePath := constructTerraformComponentVarfilePath(atmosConfig, &info)
		err := u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	// Execute the shell command.
	err = execTerraformShellCommand(
		atmosConfig,
		info.ComponentFromArg,
		info.Stack,
		info.ComponentEnvList,
		varFile,
		workingDir,
		info.TerraformWorkspace,
		componentPath,
	)
	if err != nil {
		return err
	}

	return nil
}
