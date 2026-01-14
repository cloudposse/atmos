package exec

import (
	"context"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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
	_ = ui.Writeln("Dry run mode: shell would be started with the following configuration:")
	_ = ui.Writeln("  Component: " + info.ComponentFromArg)
	_ = ui.Writeln("  Stack: " + info.Stack)
	_ = ui.Writeln("  Working directory: " + cfg.workingDir)
	_ = ui.Writeln("  Terraform workspace: " + info.TerraformWorkspace)
	_ = ui.Writeln("  Component path: " + cfg.componentPath)
	_ = ui.Writeln("  Varfile: " + cfg.varFile)
}

// ExecuteTerraformShell starts an interactive shell configured for a terraform component.
func ExecuteTerraformShell(opts *ShellOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformShell")()

	log.Debug("ExecuteTerraformShell called",
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

	// Run provisioners to ensure workdir exists if configured.
	// This handles the workdir provisioner which may copy component files to an isolated directory.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err = provisioner.ExecuteProvisioners(ctx, provisioner.HookEvent(beforeTerraformInitEvent), atmosConfig, info.ComponentSection, info.AuthContext)
	if err != nil {
		return errUtils.Build(errUtils.ErrProvisionerFailed).
			WithCause(err).
			WithExplanation("provisioner execution failed before terraform shell").
			Err()
	}

	// Check if workdir provisioner set a workdir path - if so, use it instead of the component path.
	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		componentPath = workdirPath
		log.Debug("Using workdir path for shell", "workdirPath", workdirPath)
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
