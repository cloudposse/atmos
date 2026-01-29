package exec

import (
	"context"
	"errors"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
	ui.Writeln("Dry run mode: shell would be started with the following configuration:")
	ui.Writeln("  Component: " + info.ComponentFromArg)
	ui.Writeln("  Stack: " + info.Stack)
	ui.Writeln("  Working directory: " + cfg.workingDir)
	ui.Writeln("  Terraform workspace: " + info.TerraformWorkspace)
	ui.Writeln("  Component path: " + cfg.componentPath)
	ui.Writeln("  Varfile: " + cfg.varFile)
}

// ExecuteTerraformShell starts an interactive shell configured for a terraform component.
func ExecuteTerraformShell(opts *ShellOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformShell")()

	log.Debug("ExecuteTerraformShell called",
		"component", opts.Component, "stack", opts.Stack,
		"processTemplates", opts.ProcessTemplates, "processFunctions", opts.ProcessFunctions,
		"skip", opts.Skip, "dryRun", opts.DryRun, "identity", opts.Identity,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component, Stack: opts.Stack, StackFromArg: opts.Stack,
		ComponentType: "terraform", SubCommand: "shell", DryRun: opts.DryRun,
		Identity: opts.Identity,
	}

	// Create and authenticate AuthManager if identity is specified (via --identity flag).
	// This enables YAML functions like !terraform.state to use authenticated credentials.
	authManager, err := createShellAuthManager(atmosConfig, &info)
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

// createShellAuthManager creates and authenticates an AuthManager for the shell command.
// It merges global auth config with component-specific auth config, then creates and
// authenticates the AuthManager using the identity from the --identity flag.
func createShellAuthManager(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	defer perf.Track(atmosConfig, "exec.createShellAuthManager")()

	// Get merged auth config (global + component-specific if available).
	mergedAuthConfig, err := getShellMergedAuthConfig(atmosConfig, info)
	if err != nil {
		return nil, err
	}

	// Create and authenticate AuthManager from --identity flag if specified.
	// Uses merged auth config that includes both global and component-specific identities/defaults.
	// This enables YAML template functions like !terraform.state to use authenticated credentials.
	authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(info.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, atmosConfig)
	if err != nil {
		return nil, err
	}

	// If AuthManager was created and identity was auto-detected (info.Identity was empty),
	// store the authenticated identity back into info.Identity.
	storeAuthenticatedIdentity(authManager, info)

	return authManager, nil
}

// getShellMergedAuthConfig merges global auth config with component-specific auth config.
func getShellMergedAuthConfig(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
	defer perf.Track(atmosConfig, "exec.getShellMergedAuthConfig")()

	// Start with global auth config.
	mergedAuthConfig := auth.CopyGlobalAuthConfig(&atmosConfig.Auth)

	// If stack or component are missing, use global auth config only.
	if info.Stack == "" || info.ComponentFromArg == "" {
		return mergedAuthConfig, nil
	}

	// Get component configuration from stack.
	// Use nil AuthManager and disable functions to avoid circular dependency.
	componentConfig, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            info.ComponentFromArg,
		Stack:                info.Stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false, // Critical: avoid circular dependency with YAML functions that need auth.
		Skip:                 nil,
		AuthManager:          nil, // Critical: no AuthManager yet, we're determining which identity to use.
	})
	if err != nil {
		// If component doesn't exist, exit immediately before attempting authentication.
		// This prevents prompting for identity when the component is invalid.
		if errors.Is(err, errUtils.ErrInvalidComponent) {
			return nil, err
		}
		// For other errors (e.g., permission issues), continue with global auth config.
		return mergedAuthConfig, nil
	}

	// Merge component-specific auth with global auth.
	return auth.MergeComponentAuthFromConfig(&atmosConfig.Auth, componentConfig, atmosConfig, cfg.AuthSectionName)
}

// storeAuthenticatedIdentity stores the authenticated identity from AuthManager back into info.Identity.
func storeAuthenticatedIdentity(authManager auth.AuthManager, info *schema.ConfigAndStacksInfo) {
	if authManager == nil || info.Identity != "" {
		return
	}

	chain := authManager.GetChain()
	if len(chain) == 0 {
		return
	}

	// The last element in the chain is the authenticated identity.
	authenticatedIdentity := chain[len(chain)-1]
	info.Identity = authenticatedIdentity
	log.Debug("Stored authenticated identity for shell", "identity", authenticatedIdentity)
}
