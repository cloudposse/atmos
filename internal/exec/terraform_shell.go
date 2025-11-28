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
	dryRun bool,
	atmosConfig *schema.AtmosConfiguration,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteShell")()

	log.Debug("ExecuteShell called",
		"component", component,
		"stack", stack,
		"processTemplates", processTemplates,
		"processFunctions", processFunctions,
		"skip", skip,
		"dryRun", dryRun,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
		StackFromArg:     stack,
		ComponentType:    "terraform",
		SubCommand:       "shell",
		DryRun:           dryRun,
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

	// In dry-run mode, print information and exit without executing.
	if info.DryRun {
		u.PrintMessage("Dry run mode: shell would be started with the following configuration:")
		u.PrintMessage("  Component: " + info.ComponentFromArg)
		u.PrintMessage("  Stack: " + info.Stack)
		u.PrintMessage("  Working directory: " + workingDir)
		u.PrintMessage("  Terraform workspace: " + info.TerraformWorkspace)
		u.PrintMessage("  Component path: " + componentPath)
		u.PrintMessage("  Varfile: " + varFile)
		return nil
	}

	// Write variables to varfile.
	varFilePath := constructTerraformComponentVarfilePath(atmosConfig, &info)
	err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0o644)
	if err != nil {
		return err
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
