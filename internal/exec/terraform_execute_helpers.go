package exec

// terraform_execute_helpers.go contains helper functions extracted from ExecuteTerraform
// to reduce cyclomatic complexity and improve testability.
// Each function handles one discrete responsibility of the terraform execution pipeline.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/provisioner"
	_ "github.com/cloudposse/atmos/pkg/provisioner/source" // register source provisioner
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// resolveTerraformCommand sets info.Command from atmosConfig if not already set.
// Falls back to the cfg.TerraformComponentType default when neither is configured.
func resolveTerraformCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.Command != "" {
		return
	}
	if atmosConfig.Components.Terraform.Command != "" {
		info.Command = atmosConfig.Components.Terraform.Command
	} else {
		info.Command = cfg.TerraformComponentType
	}
}

// handleVersionSubcommand executes the `terraform version` command and returns the result.
// It resolves the toolchain binary and delegates directly to the shell, bypassing
// full stack processing.
func handleVersionSubcommand(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) error {
	tenv, err := dependencies.ForComponent(&atmosConfig, "terraform", nil, nil)
	if err != nil {
		return err
	}
	return ExecuteShellCommand(
		atmosConfig,
		tenv.Resolve(info.Command),
		[]string{info.SubCommand},
		"",
		tenv.EnvVars(),
		false,
		info.RedirectStdErr)
}

// setupTerraformAuth builds the merged auth config (global + component-specific via
// getMergedAuthConfig), creates and authenticates the AuthManager, stores the resolved
// identity back into info, and injects an auth resolver into the Atmos store registry.
//
// getMergedAuthConfig is the shared helper (utils_auth.go) that handles the
// component config fetch, debug logging on fallback, and the ErrInvalidComponent
// short-circuit. Using it here eliminates duplication and keeps both code paths in sync.
//
// The defaultAuthManagerCreator injectable var (utils_auth.go) is used so tests can
// substitute a fake creator without needing a separate var in this file.
// The defaultMergedAuthConfigGetter injectable var below allows tests to exercise the
// ErrInvalidAuthConfig wrap branch without requiring a real stack or component.

// defaultMergedAuthConfigGetter is the injectable function for getMergedAuthConfig.
// Overriding it in tests allows exercising error branches that are otherwise only
// reachable via MergeComponentAuthFromConfig failures (hard to trigger in unit tests).
var defaultMergedAuthConfigGetter = func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
	return getMergedAuthConfig(atmosConfig, info)
}

func setupTerraformAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	// Log the identity-selection decision point for easy debugging.
	log.Debug("Resolving auth config for terraform command",
		"stack", info.Stack, "component", info.ComponentFromArg, "subcommand", info.SubCommand)

	// Get merged auth config (global + component-specific if stack/component are set).
	// getMergedAuthConfig logs on debug when falling back to global config after an error.
	mergedAuthConfig, err := defaultMergedAuthConfigGetter(atmosConfig, info)
	if err != nil {
		// Propagate ErrInvalidComponent directly — prevents an auth prompt for a nonexistent component.
		if errors.Is(err, errUtils.ErrInvalidComponent) {
			return nil, err
		}
		// Wrap unexpected errors (e.g. MergeComponentAuthFromConfig failures) with the sentinel
		// to match the behaviour of createAndAuthenticateAuthManagerWithDeps.
		return nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	// Create and authenticate the AuthManager using the same injectable creator as
	// createAndAuthenticateAuthManagerWithDeps to keep injection points unified.
	authManager, err := defaultAuthManagerCreator(
		info.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, atmosConfig)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		// Wrap auth creation failures with the sentinel to match createAndAuthenticateAuthManagerWithDeps.
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Persist the auto-detected identity so downstream hooks don't re-prompt.
	storeAutoDetectedIdentity(authManager, info)

	// Store manager for nested YAML functions (e.g. !terraform.state).
	info.AuthManager = authManager

	// Bridge auth credentials into identity-aware stores (lazy resolution on first use).
	if authManager != nil {
		resolver := authbridge.NewResolver(authManager, info)
		atmosConfig.Stores.SetAuthContextResolver(resolver)
	}

	return authManager, nil
}

// resolveAndProvisionComponentPath resolves the filesystem path for a terraform component,
// optionally auto-generates files, performs JIT source provisioning, and validates
// that the resulting directory actually exists.
func resolveAndProvisionComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	componentPath, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", fmt.Errorf("failed to resolve component path: %w", err)
	}

	// Auto-generate source files before path validation when configured.
	// This allows entire components to be generated from stack configuration.
	if atmosConfig.Components.Terraform.AutoGenerateFiles && !info.DryRun { //nolint:nestif
		generateSection := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
		if generateSection != nil {
			if mkdirErr := os.MkdirAll(componentPath, 0o755); mkdirErr != nil { //nolint:revive
				return "", errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", mkdirErr))
			}
			if genErr := GenerateFilesForComponent(atmosConfig, info, componentPath); genErr != nil {
				return "", errors.Join(errUtils.ErrFileOperation, genErr)
			}
		}
	}

	componentPathExists, err := u.IsDirectory(componentPath)

	// JIT source provisioning: vendor the component from a remote source when configured.
	// Source provisioning takes precedence over local component files.
	if provSource.HasSource(info.ComponentSection) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if autoErr := provSource.AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, info.ComponentSection, info.AuthContext); autoErr != nil {
			return "", fmt.Errorf("failed to auto-provision component source: %w", autoErr)
		}

		if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok {
			// Source provisioner also set a workdir path → use that path.
			componentPath = workdirPath
			componentPathExists = true
			err = nil
		} else {
			// Re-check existence after provisioning.
			componentPathExists, err = u.IsDirectory(componentPath)
		}
	}

	if err != nil || !componentPathExists {
		basePath, _ := u.GetComponentBasePath(atmosConfig, cfg.TerraformComponentType)
		return "", fmt.Errorf(
			"%w: '%s' points to the Terraform component '%s', but it does not exist in '%s'",
			errUtils.ErrInvalidTerraformComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	return componentPath, nil
}

// checkComponentRestrictions returns an error when the requested subcommand is not
// permitted for the component due to its metadata (abstract, locked) or the configured
// backend type (HTTP backend does not support workspaces).
func checkComponentRestrictions(info *schema.ConfigAndStacksInfo) error {
	// Abstract components cannot be provisioned.
	if info.ComponentIsAbstract {
		switch info.SubCommand {
		case "plan", "apply", "deploy", "workspace":
			return fmt.Errorf(
				"%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
				errUtils.ErrAbstractComponentCantBeProvisioned,
				filepath.Join(info.ComponentFolderPrefix, info.Component),
			)
		}
	}

	// Locked components may not be mutated.
	if info.ComponentIsLocked {
		switch info.SubCommand {
		case "apply", "deploy", "destroy", "import", "state", "taint", "untaint":
			return fmt.Errorf(
				"%w: component '%s' cannot be modified (metadata.locked: true)",
				errUtils.ErrLockedComponentCantBeProvisioned,
				filepath.Join(info.ComponentFolderPrefix, info.Component),
			)
		}
	}

	// HTTP backend does not support workspace commands.
	if info.SubCommand == "workspace" && info.ComponentBackendType == "http" {
		return errUtils.ErrHTTPBackendWorkspaces
	}

	return nil
}

// printAndWriteVarFiles logs component variables and, when not using a pre-existing
// plan file, writes them to the varfile on disk (path derived from atmosConfig+info).
// Workspace subcommands do not use varfiles and are skipped entirely.
func printAndWriteVarFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.SubCommand == "workspace" {
		return nil
	}

	log.Debug("Variables for the component in the stack", logFieldComponent, info.ComponentFromArg, "stack", info.Stack)
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		if err := u.PrintAsYAMLToFileDescriptor(atmosConfig, info.ComponentVarsSection); err != nil {
			return err
		}
	}

	if !info.UseTerraformPlan {
		varFilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
		log.Debug("Writing the variables", "file", varFilePath)
		if !info.DryRun {
			if err := u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0o644); err != nil {
				return err
			}
		}
	}

	if cliVars, ok := info.ComponentSection[cfg.TerraformCliVarsSectionName].(map[string]any); ok && len(cliVars) > 0 {
		log.Debug("CLI variables (will override the variables defined in the stack manifests):")
		if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
			if err := u.PrintAsYAMLToFileDescriptor(atmosConfig, cliVars); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateTerraformComponent runs OPA/JSON-schema validation policies against the
// component's stack configuration section and returns an error if validation fails.
func validateTerraformComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	valid, err := ValidateComponent(
		atmosConfig,
		info.ComponentFromArg,
		info.ComponentSection,
		"", "", nil, 0,
	)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: the component '%s' did not pass the validation policies",
			errUtils.ErrInvalidComponent, info.ComponentFromArg)
	}
	return nil
}

// generateConfigFiles writes the backend configuration, generated files, and
// provider overrides for the component into the working directory.
func generateConfigFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	if err := generateBackendConfig(atmosConfig, info, workingDir); err != nil {
		return err
	}
	if err := GenerateFilesForComponent(atmosConfig, info, workingDir); err != nil {
		return err
	}
	return generateProviderOverrides(atmosConfig, info, workingDir)
}

// warnOnConflictingEnvVars inspects the current process environment for variables
// that are known to interfere with Atmos's management of Terraform, and emits a
// warning when any are detected.
func warnOnConflictingEnvVars() {
	warnOnExactVars := []string{"TF_CLI_ARGS", "TF_WORKSPACE"}
	warnOnPrefixVars := []string{"TF_VAR_", "TF_CLI_ARGS_"}

	var problematicVars []string
	for _, envVar := range os.Environ() {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if u.SliceContainsString(warnOnExactVars, parts[0]) {
			problematicVars = append(problematicVars, parts[0])
			continue
		}
		for _, prefix := range warnOnPrefixVars {
			if strings.HasPrefix(parts[0], prefix) {
				problematicVars = append(problematicVars, parts[0])
				break
			}
		}
	}

	if len(problematicVars) > 0 {
		log.Warn("Detected environment variables that may interfere with Atmos's control of Terraform",
			"variables", problematicVars)
	}
}

// assembleComponentEnvVars builds the complete list of environment variables for
// the terraform subprocess.  It combines the component env section, standard Atmos
// variables (ATMOS_CLI_CONFIG_PATH, ATMOS_BASE_PATH, TF_IN_AUTOMATION), the
// TF_APPEND_USER_AGENT value, the plugin-cache env, and any toolchain PATH overrides.
func assembleComponentEnvVars(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, tenv *dependencies.ToolchainEnvironment) error {
	// Convert ComponentEnvSection (set by auth hooks and stack config) to a list.
	for k, v := range info.ComponentEnvSection {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("%s=%v", k, v))
	}

	info.ComponentEnvList = append(info.ComponentEnvList,
		fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))

	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))

	// Suppress verbose Terraform instructions in automated environments.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_in_automation
	info.ComponentEnvList = append(info.ComponentEnvList, "TF_IN_AUTOMATION=true")

	// Precedence: OS env > atmos.yaml > default (empty/omitted).
	appendUserAgent := atmosConfig.Components.Terraform.AppendUserAgent
	if envUA, exists := os.LookupEnv("TF_APPEND_USER_AGENT"); exists && envUA != "" {
		appendUserAgent = envUA
	}
	if appendUserAgent != "" {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("TF_APPEND_USER_AGENT=%s", appendUserAgent))
	}

	// Plugin cache directory.
	info.ComponentEnvList = append(info.ComponentEnvList, configurePluginCache(atmosConfig)...)

	// Toolchain PATH must come last so it takes precedence over all other PATH entries.
	if tenv != nil {
		info.ComponentEnvList = append(info.ComponentEnvList, tenv.EnvVars()...)
	}

	if len(info.ComponentEnvList) > 0 {
		log.Debug("Using ENV vars:")
		for _, v := range info.ComponentEnvList {
			log.Debug(v)
		}
	}

	return nil
}

// shouldRunTerraformInit returns true when a `terraform init` should be executed as a
// pre-step before the main command.  Init is skipped when: the subcommand is init
// itself (init runs as the main command), deploy with DeployRunInit=false is configured,
// or the caller passed the --skip-init flag.
func shouldRunTerraformInit(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	if info.SubCommand == "init" {
		return false
	}
	if info.SubCommand == "deploy" && !atmosConfig.Components.Terraform.DeployRunInit {
		return false
	}
	if info.SkipInit {
		log.Debug("Skipping over 'terraform init' due to '--skip-init' flag being passed")
		return false
	}
	return true
}

// buildInitArgs constructs the argument list for `terraform init`.
// It adds -reconfigure when the component uses the workspace subcommand or when
// InitRunReconfigure is enabled, and appends the varfile flag when PassVars is set.
func buildInitArgs(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, varFile string) []string {
	if info.SubCommand == "workspace" || atmosConfig.Components.Terraform.InitRunReconfigure {
		if atmosConfig.Components.Terraform.Init.PassVars {
			return []string{"init", "-reconfigure", varFileFlag, varFile}
		}
		return []string{"init", "-reconfigure"}
	}
	if atmosConfig.Components.Terraform.Init.PassVars {
		return []string{"init", varFileFlag, varFile}
	}
	return []string{"init"}
}

// prepareInitExecution performs the pre-init housekeeping:
//  1. Deletes the .terraform/environment file so Terraform doesn't prompt for workspace selection.
//  2. Executes all provisioners registered for the before.terraform.init hook event.
//  3. Returns the effective component path (which may be overridden by a workdir provisioner).
func prepareInitExecution(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) (string, error) {
	cleanTerraformWorkspace(*atmosConfig, componentPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := provisioner.ExecuteProvisioners(
		ctx,
		provisioner.HookEvent(beforeTerraformInitEvent),
		atmosConfig,
		info.ComponentSection,
		info.AuthContext,
	); err != nil {
		return componentPath, fmt.Errorf("provisioner execution failed: %w", err)
	}

	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		log.Debug("Using workdir path for terraform command", "workdirPath", workdirPath)
		return workdirPath, nil
	}

	return componentPath, nil
}

// executeTerraformInitPhase runs `terraform init` as a pre-step before the main command.
// It prepares the init execution environment, builds the init args, and delegates to
// ExecuteShellCommand.  Returns the (possibly updated) component path.
//
// MUTUAL EXCLUSION CONTRACT: this function is called ONLY when shouldRunTerraformInit()
// returns true (i.e. SubCommand ≠ "init").  For the "init" subcommand itself,
// buildInitSubcommandArgs in terraform_execute_helpers_args.go handles the provisioner
// invocation via prepareInitExecution.  These two code paths must never both execute
// in the same command invocation or provisioners will run twice.
func executeTerraformInitPhase(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string) (string, error) {
	newPath, err := prepareInitExecution(atmosConfig, info, componentPath)
	if err != nil {
		return componentPath, err
	}

	initArgs := buildInitArgs(atmosConfig, info, varFile)
	if err = ExecuteShellCommand(
		*atmosConfig,
		info.Command,
		initArgs,
		newPath,
		info.ComponentEnvList,
		info.DryRun,
		info.RedirectStdErr,
	); err != nil {
		return newPath, err
	}

	return newPath, nil
}

// handleDeploySubcommand converts `deploy` into `apply` and ensures -auto-approve is
// added when appropriate.  When ApplyAutoApprove is set in atmos.yaml, it is also
// applied to plain `apply` subcommands.
func handleDeploySubcommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.SubCommand == "deploy" {
		info.SubCommand = "apply"
		if !info.UseTerraformPlan && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	if info.SubCommand == "apply" && atmosConfig.Components.Terraform.ApplyAutoApprove && !info.UseTerraformPlan {
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}
}

// logTerraformContext emits a debug log line with the full execution context
// (executable, command, component, stack, flags, working directory, inheritance chain).
func logTerraformContext(info *schema.ConfigAndStacksInfo, workingDir string) {
	command := info.SubCommand
	if info.SubCommand2 != "" {
		command = fmt.Sprintf("%s %s", info.SubCommand, info.SubCommand2)
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	log.Debug("Terraform context",
		"executable", info.Command,
		"command", command,
		logFieldComponent, info.ComponentFromArg,
		"stack", info.StackFromArg,
		"arguments and flags", info.AdditionalArgsAndFlags,
		"terraform component", info.BaseComponentPath,
		"inheritance", inheritance,
		"working directory", workingDir,
	)
}
