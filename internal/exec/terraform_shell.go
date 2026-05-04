package exec

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	_ "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
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
	ui.Writeln("Dry run mode: shell would be started with the following configuration:")
	ui.Writeln("  Component: " + info.ComponentFromArg)
	ui.Writeln("  Stack: " + info.Stack)
	ui.Writeln("  Working directory: " + cfg.workingDir)
	ui.Writeln("  Terraform workspace: " + info.TerraformWorkspace)
	ui.Writeln("  Component path: " + cfg.componentPath)
	ui.Writeln("  Varfile: " + cfg.varFile)
}

// shellInfoFromOptions builds a ConfigAndStacksInfo from ShellOptions.
func shellInfoFromOptions(opts *ShellOptions) schema.ConfigAndStacksInfo {
	return schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		ComponentType:    "terraform",
		SubCommand:       "shell",
		DryRun:           opts.DryRun,
		Identity:         opts.Identity,
	}
}

// resolveWorkdirPath returns the workdir path from componentSection if set by a workdir provisioner,
// otherwise returns the original componentPath unchanged.
func resolveWorkdirPath(componentSection map[string]any, componentPath string) string {
	if workdirPath, ok := componentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		log.Debug("Using workdir path for shell", "workdirPath", workdirPath)
		return workdirPath
	}
	return componentPath
}

// ExecuteTerraformShell starts an interactive shell configured for a terraform component.
func ExecuteTerraformShell(opts *ShellOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformShell")()

	log.Debug(
		"ExecuteTerraformShell called",
		"component", opts.Component, "stack", opts.Stack,
		"processTemplates", opts.ProcessTemplates, "processFunctions", opts.ProcessFunctions,
		"skip", opts.Skip, "dryRun", opts.DryRun, "identity", opts.Identity,
	)

	info := shellInfoFromOptions(opts)

	// Create and authenticate AuthManager by merging global + component auth config.
	// This enables YAML functions like !terraform.state to use authenticated credentials.
	authManager, err := createAndAuthenticateAuthManager(atmosConfig, &info)
	if err != nil {
		// Special case: If user aborted (Ctrl+C), exit immediately without showing error.
		if errors.Is(err, errUtils.ErrUserAborted) {
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		return err
	}

	// Store AuthManager in configAndStacksInfo for YAML functions.
	if authManager != nil {
		info.AuthManager = authManager

		// Inject auth resolver into identity-aware stores.
		resolver := authbridge.NewResolver(authManager, &info)
		atmosConfig.Stores.SetAuthContextResolver(resolver)
	}

	info, err = ProcessStacks(atmosConfig, info, true, opts.ProcessTemplates, opts.ProcessFunctions, opts.Skip, authManager)
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

	// Honor metadata.component as a module subpath inside the cloned workdir
	// (issue #2364). ExecuteProvisioners sets WorkdirPathKey to the bare workdir
	// root; this joins the subpath onto it (only when the subdirectory exists)
	// so the shell, working directory, and varfile all land in the configured
	// submodule. When metadata.component is used as an inheritance pointer
	// rather than a real subpath, the helper leaves WorkdirPathKey at the root.
	if _, subpathErr := component.ApplyWorkdirSubpathToSection(&info); subpathErr != nil {
		// subpathErr already wraps ErrWorkdirProvision; preserve that
		// classification rather than re-wrapping with ErrProvisionerFailed
		// (which means an AutoProvisionSource hook failed, not a path
		// resolution failed).
		return fmt.Errorf("resolve metadata.component subpath inside workdir: %w", subpathErr)
	}

	// Check if workdir provisioner set a workdir path - if so, use it instead of the component path.
	componentPath = resolveWorkdirPath(info.ComponentSection, componentPath)

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
