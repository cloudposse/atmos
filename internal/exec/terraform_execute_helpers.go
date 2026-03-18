package exec

// terraform_execute_helpers.go contains helper functions extracted from ExecuteTerraform
// to reduce cyclomatic complexity and improve testability.
// Each function handles one discrete responsibility of the terraform execution pipeline.

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
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

// setupTerraformAuthCreator allows injection of a custom AuthManager creator for
// testing.  In production it delegates to the real implementation.
var setupTerraformAuthCreator authManagerCreator = auth.CreateAndAuthenticateManagerWithAtmosConfig

// setupTerraformAuth builds the merged auth config (global + component-specific via
// getMergedAuthConfig), creates and authenticates the AuthManager, stores the resolved
// identity back into info, and injects an auth resolver into the Atmos store registry.
//
// getMergedAuthConfig is the shared helper (utils_auth.go) that handles the
// component config fetch, debug logging on fallback, and the ErrInvalidComponent
// short-circuit. Using it here eliminates duplication and keeps both code paths in sync.
func setupTerraformAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	// Get merged auth config (global + component-specific if stack/component are set).
	// getMergedAuthConfig logs on debug when falling back to global config after an error.
	mergedAuthConfig, err := getMergedAuthConfig(atmosConfig, info)
	if err != nil {
		return nil, err
	}

	// Create and authenticate the AuthManager.
	authManager, err := setupTerraformAuthCreator(
		info.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, atmosConfig)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		return nil, err
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
// plan file, writes them to the varfile on disk.
// Workspace subcommands do not use varfiles and are skipped entirely.
func printAndWriteVarFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, varFile string) error {
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

// buildPlanSubcommandArgs extends allArgsAndFlags for the `terraform plan` subcommand.
// It adds the varfile, optionally the planfile, and handles the upload-status flag.
// Returns the updated args, the uploadStatus flag, and the (mutated) info.AdditionalArgsAndFlags.
func buildPlanSubcommandArgs(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	varFile, planFile string,
) ([]string, bool) {
	allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)

	if !u.SliceContainsString(info.AdditionalArgsAndFlags, outFlag) &&
		!u.SliceContainsStringHasPrefix(info.AdditionalArgsAndFlags, outFlag+"=") &&
		!atmosConfig.Components.Terraform.Plan.SkipPlanfile {
		allArgsAndFlags = append(allArgsAndFlags, outFlag, planFile)
	}

	uploadStatusFlag := parseUploadStatusFlag(info.AdditionalArgsAndFlags, cfg.UploadStatusFlag)
	info.AdditionalArgsAndFlags = u.SliceRemoveFlag(info.AdditionalArgsAndFlags, cfg.UploadStatusFlag)

	if uploadStatusFlag && !u.SliceContainsString(info.AdditionalArgsAndFlags, detailedExitCodeFlag) {
		allArgsAndFlags = append(allArgsAndFlags, detailedExitCodeFlag)
	}

	return allArgsAndFlags, uploadStatusFlag
}

// buildApplySubcommandArgs extends allArgsAndFlags for the `terraform apply` subcommand.
// When not consuming a pre-built plan, it appends the varfile.
// After all flags have been added, the planfile positional argument is appended.
func buildApplySubcommandArgs(
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	varFile, planFile string,
) []string {
	if !info.UseTerraformPlan {
		allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)
	}

	return allArgsAndFlags
}

// appendApplyPlanFileArg appends the plan-file positional argument to allArgsAndFlags for
// `terraform apply` when a pre-built plan is being consumed.
func appendApplyPlanFileArg(info *schema.ConfigAndStacksInfo, allArgsAndFlags []string, planFile string) []string {
	if info.SubCommand != "apply" || !info.UseTerraformPlan {
		return allArgsAndFlags
	}
	if info.PlanFile != "" {
		return append(allArgsAndFlags, info.PlanFile)
	}
	return append(allArgsAndFlags, planFile)
}

// buildInitSubcommandArgs extends allArgsAndFlags for the `terraform init` subcommand.
// It runs provisioners, optionally updates *componentPath via the workdir provisioner,
// and adds the -reconfigure / -var-file flags when configured.
func buildInitSubcommandArgs(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	varFile string,
	componentPath *string,
) ([]string, error) {
	newPath, provErr := prepareInitExecution(atmosConfig, info, *componentPath)
	if provErr != nil {
		return nil, provErr
	}
	*componentPath = newPath

	if atmosConfig.Components.Terraform.InitRunReconfigure {
		allArgsAndFlags = append(allArgsAndFlags, "-reconfigure")
	}
	if atmosConfig.Components.Terraform.Init.PassVars {
		allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)
	}

	return allArgsAndFlags, nil
}

// buildWorkspaceSubcommandArgs extends allArgsAndFlags for `terraform workspace` subcommands.
// Subcommands with a secondary argument (new, select, delete) also append the workspace name.
func buildWorkspaceSubcommandArgs(info *schema.ConfigAndStacksInfo, allArgsAndFlags []string) []string {
	switch {
	case info.SubCommand2 == "list" || info.SubCommand2 == "show":
		return append(allArgsAndFlags, info.SubCommand2)
	case info.SubCommand2 != "":
		return append(allArgsAndFlags, info.SubCommand2, info.TerraformWorkspace)
	}
	return allArgsAndFlags
}

// buildTerraformCommandArgs constructs the complete argument list for the main terraform
// command based on the subcommand.  For the "init" subcommand it also runs provisioners
// and may update *componentPath via the workdir provisioner.
// Returns the argument list, an uploadStatus flag, and any error from provisioners.
func buildTerraformCommandArgs(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	varFile, planFile string,
	componentPath *string,
) (allArgsAndFlags []string, uploadStatusFlag bool, err error) {
	allArgsAndFlags = strings.Fields(info.SubCommand)

	switch info.SubCommand {
	case "plan":
		allArgsAndFlags, uploadStatusFlag = buildPlanSubcommandArgs(atmosConfig, info, allArgsAndFlags, varFile, planFile)

	case "destroy", "import", "refresh":
		allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)

	case "apply":
		allArgsAndFlags = buildApplySubcommandArgs(info, allArgsAndFlags, varFile, planFile)

	case "init":
		allArgsAndFlags, err = buildInitSubcommandArgs(atmosConfig, info, allArgsAndFlags, varFile, componentPath)
		if err != nil {
			return nil, false, err
		}

	case "workspace":
		allArgsAndFlags = buildWorkspaceSubcommandArgs(info, allArgsAndFlags)
	}

	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Positional plan-file argument must come after all flags.
	allArgsAndFlags = appendApplyPlanFileArg(info, allArgsAndFlags, planFile)

	return allArgsAndFlags, uploadStatusFlag, nil
}

// runWorkspaceSetup selects (or creates) the Terraform workspace before the main command
// runs.  It is a no-op when: the subcommand is init, the caller is already operating on
// a named workspace (SubCommand2 != ""), the HTTP backend is in use, or the caller has
// set TF_WORKSPACE themselves.
func runWorkspaceSetup(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if info.SubCommand == "init" || (info.SubCommand == "workspace" && info.SubCommand2 != "") {
		return nil
	}
	if info.ComponentBackendType == "http" {
		return nil
	}
	if os.Getenv("TF_WORKSPACE") != "" {
		return nil
	}

	// Default: redirect workspace-select stderr to stdout so it is visible.
	workspaceSelectRedirectStdErr := "/dev/stdout"
	if info.RedirectStdErr != "" {
		workspaceSelectRedirectStdErr = info.RedirectStdErr
	}

	// For data-producing subcommands redirect "Switched to workspace…" to stderr
	// so it doesn't pollute captured stdout in $() substitutions.
	var wsOpts []ShellCommandOption
	if info.SubCommand == "output" || info.SubCommand == "show" {
		wsOpts = append(wsOpts, WithStdoutOverride(os.Stderr))
	}

	err := ExecuteShellCommand(
		*atmosConfig,
		info.Command,
		[]string{"workspace", "select", info.TerraformWorkspace},
		componentPath,
		info.ComponentEnvList,
		info.DryRun,
		workspaceSelectRedirectStdErr,
		wsOpts...,
	)
	if err == nil {
		return nil
	}

	// Exit code 1 means the workspace doesn't exist yet; create it.
	var exitCodeErr errUtils.ExitCodeError
	if !errors.As(err, &exitCodeErr) || exitCodeErr.Code != 1 {
		return err
	}

	return ExecuteShellCommand(
		*atmosConfig,
		info.Command,
		[]string{"workspace", "new", info.TerraformWorkspace},
		componentPath,
		info.ComponentEnvList,
		info.DryRun,
		info.RedirectStdErr,
	)
}

// checkTTYRequirement returns an error when `terraform apply` is invoked without
// -auto-approve in a non-interactive environment (stdin is nil).
func checkTTYRequirement(info *schema.ConfigAndStacksInfo) error {
	if os.Stdin != nil {
		return nil
	}
	if info.SubCommand == "apply" && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
		return fmt.Errorf(
			"%w: 'terraform apply' requires a user interaction, but no TTY is attached. "+
				"Use 'terraform apply -auto-approve' or 'terraform deploy' instead",
			errUtils.ErrNoTty,
		)
	}
	return nil
}

// addRegionEnvVarForImport appends AWS_REGION to the component env list when the
// subcommand is `import` and the component has a `region` variable configured.
func addRegionEnvVarForImport(info *schema.ConfigAndStacksInfo) {
	if info.SubCommand != "import" {
		return
	}
	if region, ok := info.ComponentVarsSection["region"].(string); ok {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("AWS_REGION=%s", region))
	}
}

// resolveExitCode extracts the integer exit code from an error returned by
// ExecuteShellCommand.  Returns 0 when err is nil, 1 for generic (non-typed) errors.
func resolveExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ec errUtils.ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	var osErr *osexec.ExitError
	if errors.As(err, &osErr) {
		return osErr.ExitCode()
	}
	return 1
}

// executeMainTerraformCommand runs the final terraform sub-command.
// It handles exit-code extraction, plan-status upload (for --upload-status), and
// appropriate error propagation.  A no-op when info.SubCommand is "workspace" with
// no sub-subcommand (workspace listing was already handled by runWorkspaceSetup).
func executeMainTerraformCommand(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	componentPath string,
	uploadStatusFlag bool,
	opts ...ShellCommandOption,
) error {
	// Bare `workspace` (no sub-subcommand) was fully handled by runWorkspaceSetup.
	if info.SubCommand == "workspace" && info.SubCommand2 == "" {
		return nil
	}

	err := ExecuteShellCommand(
		*atmosConfig,
		info.Command,
		allArgsAndFlags,
		componentPath,
		info.ComponentEnvList,
		info.DryRun,
		info.RedirectStdErr,
		opts...,
	)

	exitCode := resolveExitCode(err)

	if uploadStatusFlag && shouldUploadStatus(info) {
		client, cerr := pro.NewAtmosProAPIClientFromEnv(atmosConfig)
		if cerr != nil {
			return cerr
		}
		gitRepo := &git.DefaultGitRepo{}
		if uerr := uploadStatus(info, exitCode, client, gitRepo); uerr != nil {
			return uerr
		}
		// Exit codes 0 and 2 are both "success" for plan uploads.
		if exitCode == 0 {
			return nil
		}
		if exitCode == 2 {
			return errUtils.ExitCodeError{Code: 2}
		}
	}

	return err
}

// cleanupTerraformFiles removes ephemeral plan and varfiles that Atmos generates.
// Failures are logged at Trace level and not propagated, since cleanup errors should
// not mask the result of the main command.
func cleanupTerraformFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.SubCommand != "plan" && info.SubCommand != "show" && info.PlanFile == "" {
		planFilePath := constructTerraformComponentPlanfilePath(atmosConfig, info)
		if err := os.Remove(planFilePath); err != nil && !os.IsNotExist(err) {
			log.Trace("Failed to remove plan file during cleanup", "error", err, "file", planFilePath)
		}
	}

	if info.SubCommand == "apply" {
		varFilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
		if err := os.Remove(varFilePath); err != nil && !os.IsNotExist(err) {
			log.Trace("Failed to remove var file during cleanup", "error", err, "file", varFilePath)
		}
	}
}
