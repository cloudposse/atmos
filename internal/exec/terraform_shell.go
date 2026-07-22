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
		SkipInit:         opts.SkipInit,
	}
}

// applyShellSecretEnv exports secret-bearing variables into the interactive shell as
// TF_VAR_<name> environment variables when withSecrets is true. When false, the secrets
// are withheld (they were already kept out of the on-disk varfile) and a warning explains
// how to opt in. Requires computeTerraformSecretVarKeys to have run first.
func applyShellSecretEnv(info *schema.ConfigAndStacksInfo, withSecrets bool) error {
	if !withSecrets {
		if len(info.TerraformSecretVarKeys) > 0 {
			log.Warn("Secret-bearing variables are not exported into the shell; pass --with-secrets to export them as TF_VAR_* environment variables",
				"count", len(info.TerraformSecretVarKeys))
		}
		return nil
	}

	secretEnv, err := secretVarEnv(info)
	if err != nil {
		return err
	}
	if len(secretEnv) > 0 {
		info.ComponentEnvList = append(info.ComponentEnvList, secretEnv...)
		log.Debug("Exporting secret variables into the shell as TF_VAR_* environment variables", "count", len(secretEnv))
	}
	return nil
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

		injectTerraformStoreAuthResolver(atmosConfig, &info, authManager)
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

	// ExecuteProvisioners above sets WorkdirPathKey to the bare workdir root;
	// joining the metadata.component subpath onto it must happen before the
	// shell, working directory, and varfile paths are read below.
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

	return runShellSession(atmosConfig, &info, cfg, opts.WithSecrets)
}

// runShellSession performs the per-component setup (prepareShellExecution) and then runs
// `terraform init`/`workspace` and launches the interactive shell (executeShellLifecycle).
// It is split out of ExecuteTerraformShell so the post-ProcessStacks lifecycle can be unit-tested
// without resolving real stacks or launching a shell.
func runShellSession(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, cfg *shellConfig, withSecrets bool) error {
	defer perf.Track(atmosConfig, "exec.runShellSession")()

	if err := prepareShellExecution(atmosConfig, info, cfg, withSecrets); err != nil {
		return err
	}

	// Remove the temporary Terraform CLI config (TF_CLI_CONFIG_FILE) after init, workspace, and
	// the whole interactive shell session complete. Deferred here (not in prepareShellExecution)
	// so the file survives every subprocess and the shell.
	if info.RCCleanup != nil {
		defer func() {
			if cleanupErr := info.RCCleanup(); cleanupErr != nil {
				log.Debug("Failed to remove temporary Terraform CLI config", "error", cleanupErr)
			}
		}()
	}

	return executeShellLifecycle(atmosConfig, info, cfg)
}

// Seams for testing the shell lifecycle without launching real subprocesses or an interactive
// shell. They default to the real implementations (mirrors the execTerraformFn pattern).
var (
	shellInitFn      = executeTerraformInitCommand
	shellWorkspaceFn = runWorkspaceSetup
	shellExecFn      = execTerraformShellCommand
)

// executeShellLifecycle runs `terraform init` and selects/creates the workspace before launching
// the interactive shell, so the user lands in an initialized component and the correct workspace
// (not `default`). This matches the documented behavior and the pre-v1.202.0 flow where shell ran
// through the shared ExecuteTerraform pipeline. The before.terraform.init provisioners already ran
// in ExecuteTerraformShell, so init uses the provisioner-free path to avoid running them twice.
func executeShellLifecycle(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, cfg *shellConfig) error {
	defer perf.Track(atmosConfig, "exec.executeShellLifecycle")()

	if shouldRunTerraformInit(atmosConfig, info) {
		// Mirror prepareInitExecution's non-workdir cleanup of .terraform/environment so
		// Terraform doesn't prompt for workspace selection. Skipped for workdir components.
		if _, isWorkdir := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); !isWorkdir {
			cleanTerraformWorkspace(*atmosConfig, cfg.componentPath)
		}
		if err := shellInitFn(atmosConfig, info, cfg.componentPath, cfg.varFile); err != nil {
			return err
		}
	}

	// Select (or create) the workspace. No-op for the http backend or when TF_WORKSPACE is set
	// (see shouldSkipWorkspaceSetup); resolves to `default` when workspaces_enabled: false.
	if err := shellWorkspaceFn(atmosConfig, info, cfg.componentPath); err != nil {
		return err
	}

	return shellExecFn(atmosConfig, info.ComponentFromArg, info.Stack,
		info.ComponentEnvList, cfg.varFile, cfg.workingDir, info.TerraformWorkspace, cfg.componentPath)
}

// prepareShellExecution performs the per-component setup the shell needs before running
// `terraform init`/`workspace` and launching the interactive shell: resolving the terraform/tofu
// binary (including the toolchain), writing the disk-safe varfile, generating backend and
// provider-override config files, and assembling the component environment. It registers
// info.RCCleanup so the temporary Terraform CLI config survives init, workspace, and the shell.
func prepareShellExecution(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, cfg *shellConfig, withSecrets bool) error {
	defer perf.Track(atmosConfig, "exec.prepareShellExecution")()

	// Shell setup also writes a Terraform varfile and can assemble TF_VAR_* values.
	// Do not allow an inspection-only `(computed)` marker across that boundary.
	if err := rejectComputedTerraformVars(info.ComponentVarsSection); err != nil {
		return err
	}

	// Resolve the terraform/tofu binary (config + toolchain) so init/workspace and the shell
	// all use the correct executable.
	resolveTerraformCommand(atmosConfig, info)
	tenv, err := resolveAndInstallToolchainDeps(atmosConfig, info)
	if err != nil {
		return err
	}
	info.Command = tenv.Resolve(info.Command)

	// Keep resolved secrets out of the on-disk varfile. With --with-secrets, export them
	// into the interactive shell as TF_VAR_* env vars; otherwise they are not available
	// (terraform commands in the shell that need them will prompt or fail).
	computeTerraformSecretVarKeys(info)

	varFilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
	if err := u.WriteToFileAsJSON(varFilePath, diskSafeVars(info), filePermissions); err != nil {
		return err
	}

	// Generate backend + provider-override files so `terraform init` configures the backend.
	if err := generateConfigFiles(atmosConfig, info, cfg.workingDir); err != nil {
		return err
	}

	// Assemble env vars (TF_IN_AUTOMATION, ATMOS_BASE_PATH, plugin cache, TF_CLI_CONFIG_FILE,
	// toolchain PATH). assembleComponentEnvVars injects secret TF_VAR_* unconditionally, so
	// suppress that here (snapshot/clear/restore TerraformSecretVarKeys) and instead route
	// secrets through applyShellSecretEnv, preserving the "secrets withheld unless --with-secrets"
	// shell behavior.
	secretKeys := info.TerraformSecretVarKeys
	info.TerraformSecretVarKeys = nil
	err = assembleComponentEnvVars(atmosConfig, info, tenv)
	info.TerraformSecretVarKeys = secretKeys
	if err != nil {
		return err
	}

	if err := applyShellSecretEnv(info, withSecrets); err != nil {
		return err
	}

	return nil
}
