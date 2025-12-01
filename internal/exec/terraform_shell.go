package exec

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// filePermissions is the standard file permission mode for generated files.
const filePermissions = 0o644

// shellConfig holds the configuration needed for shell execution.
type shellConfig struct {
	componentPath string
	workingDir    string
	varFile       string
}

// printShellDryRunInfo prints the shell configuration in dry-run mode.
func printShellDryRunInfo(info *schema.ConfigAndStacksInfo, cfg *shellConfig) {
	u.PrintMessage("Dry run mode: shell would be started with the following configuration:")
	u.PrintMessage("  Component: " + info.ComponentFromArg)
	u.PrintMessage("  Stack: " + info.Stack)
	u.PrintMessage("  Working directory: " + cfg.workingDir)
	u.PrintMessage("  Terraform workspace: " + info.TerraformWorkspace)
	u.PrintMessage("  Component path: " + cfg.componentPath)
	u.PrintMessage("  Varfile: " + cfg.varFile)
}

// ExecuteTerraformShell starts an interactive shell configured for a terraform component.
func ExecuteTerraformShell(opts *ShellOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteShell")()

	log.Debug("ExecuteShell called",
		"component", opts.Component, "stack", opts.Stack,
		"processTemplates", opts.ProcessTemplates, "processFunctions", opts.ProcessFunctions,
		"skip", opts.Skip, "dryRun", opts.DryRun,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component, Stack: opts.Stack, StackFromArg: opts.Stack,
		ComponentType: "terraform", SubCommand: "shell", DryRun: opts.DryRun,
	}

	info, err := ProcessStacks(atmosConfig, info, true, opts.ProcessTemplates, opts.ProcessFunctions, opts.Skip, nil)
	if err != nil {
		return err
	}

	componentPath, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return err
	}

	cfg := &shellConfig{
		componentPath: componentPath,
		workingDir:    constructTerraformComponentWorkingDir(atmosConfig, &info),
		varFile:       constructTerraformComponentVarfileName(&info),
	}

	if info.DryRun {
		printShellDryRunInfo(&info, cfg)
		return nil
	}

	varFilePath := constructTerraformComponentVarfilePath(atmosConfig, &info)
	if err := u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, filePermissions); err != nil {
		return err
	}

	return execTerraformShellCommand(atmosConfig, info.ComponentFromArg, info.Stack,
		info.ComponentEnvList, cfg.varFile, cfg.workingDir, info.TerraformWorkspace, cfg.componentPath)
}
