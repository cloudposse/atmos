package terraform

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/terraform/shared"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	h "github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// errWrapFormat is the format string for wrapping errors with a cause.
const errWrapFormat = "%w: %w"

func runHooks(event h.HookEvent, cmd_ *cobra.Command, args []string) error {
	// Build args for ProcessCommandLineArgs.
	// Note: Double-dash processing is handled by AtmosFlagParser in terraformRun (RunE).
	// Hooks run in PostRunE after terraformRun has already parsed and executed.
	// Hooks only need component/stack info, not separated args for terraform.
	finalArgs := append([]string{cmd_.Name()}, args...)

	info, err := e.ProcessCommandLineArgs("terraform", cmd_, finalArgs, nil)
	if err != nil {
		return err
	}

	// Initialize the CLI config
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return errors.Join(errUtils.ErrInitializeCLIConfig, err)
	}

	hooks, err := h.GetHooks(&atmosConfig, &info)
	if err != nil {
		return errors.Join(errUtils.ErrGetHooks, err)
	}

	if hooks != nil && hooks.HasHooks() {
		log.Info("Running hooks", "event", event)
		err := hooks.RunAll(event, &atmosConfig, &info, cmd_, args)
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
	}

	return nil
}

// resolveComponentPath resolves a path-based component argument to a component name.
// It validates the component exists in the specified stack and handles ambiguous paths.
func resolveComponentPath(info *schema.ConfigAndStacksInfo, commandName string) error {
	// Initialize config with processStacks=true to enable stack-based validation.
	// This is needed to detect ambiguous paths (multiple components referencing the same folder).
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrPathResolutionFailed, err)
	}

	// Resolve component from path WITH stack validation.
	// This will:
	// 1. Extract the component name from the path (e.g., "vpc" from "components/terraform/vpc").
	// 2. Look up which Atmos components reference this terraform folder in the stack.
	// 3. If multiple components reference the same folder, return an ambiguous path error.
	resolvedComponent, err := e.ResolveComponentFromPath(
		&atmosConfig,
		info.ComponentFromArg,
		info.Stack,
		commandName,
	)
	if err != nil {
		return handlePathResolutionError(err)
	}

	log.Debug("Resolved component from path",
		"original_path", info.ComponentFromArg,
		"resolved_component", resolvedComponent,
		"stack", info.Stack,
	)

	info.ComponentFromArg = resolvedComponent
	info.NeedsPathResolution = false // Mark as resolved.
	return nil
}

// handlePathResolutionError wraps path resolution errors with appropriate hints.
func handlePathResolutionError(err error) error {
	// These errors already have detailed hints from the resolver, return directly.
	// Using fmt.Errorf to wrap would lose the cockroachdb/errors hints.
	if errors.Is(err, errUtils.ErrAmbiguousComponentPath) ||
		errors.Is(err, errUtils.ErrComponentNotInStack) ||
		errors.Is(err, errUtils.ErrStackNotFound) ||
		errors.Is(err, errUtils.ErrUserAborted) {
		return err
	}
	// Generic path resolution error - add hint.
	// Use WithCause to preserve the underlying error for errors.Is introspection.
	return errUtils.Build(errUtils.ErrPathResolutionFailed).
		WithCause(err).
		WithHint("Make sure the path is within your component directories").
		Err()
}

// executeAffectedCommand handles the --affected flag execution flow.
func executeAffectedCommand(parentCmd *cobra.Command, args []string, info *schema.ConfigAndStacksInfo) error {
	// Add these flags because `atmos describe affected` needs them, but `atmos terraform --affected` does not define them.
	parentCmd.PersistentFlags().String("file", "", "")
	parentCmd.PersistentFlags().String("format", "yaml", "")
	parentCmd.PersistentFlags().Bool("verbose", false, "")
	parentCmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "")
	parentCmd.PersistentFlags().Bool("include-settings", false, "")
	parentCmd.PersistentFlags().Bool("upload", false, "")

	a, err := e.ParseDescribeAffectedCliArgs(parentCmd, args)
	if err != nil {
		return err
	}

	a.IncludeSpaceliftAdminStacks = false
	a.IncludeSettings = false
	a.Upload = false
	a.OutputFile = ""

	err = e.ExecuteTerraformAffected(&a, info)
	errUtils.CheckErrorPrintAndExit(err, "", "")
	return nil
}

// isMultiComponentExecution checks if the command should be routed to multi-component execution.
func isMultiComponentExecution(info *schema.ConfigAndStacksInfo) bool {
	return info.All || len(info.Components) > 0 || info.Query != "" || (info.Stack != "" && info.ComponentFromArg == "")
}

// executeSingleComponent executes terraform for a single component.
func executeSingleComponent(info *schema.ConfigAndStacksInfo) error {
	log.Debug("Routing to ExecuteTerraform (single-component)")
	err := e.ExecuteTerraform(*info)
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			errUtils.CheckErrorAndPrint(err, "", "")
			return err
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	return nil
}

// terraformRun is for simple subcommands without their own parsers.
// It binds terraformParser and delegates to terraformRunWithOptions.
func terraformRun(parentCmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	if err := terraformParser.BindFlagsToViper(actualCmd, v); err != nil {
		return err
	}

	opts := ParseTerraformRunOptions(v)
	return terraformRunWithOptions(parentCmd, actualCmd, args, opts)
}

// applyOptionsToInfo transfers parsed options to the info struct.
func applyOptionsToInfo(info *schema.ConfigAndStacksInfo, opts *TerraformRunOptions) {
	info.ProcessTemplates = opts.ProcessTemplates
	info.ProcessFunctions = opts.ProcessFunctions
	info.Skip = opts.Skip
	info.Components = opts.Components
	info.DryRun = opts.DryRun
	info.SkipInit = opts.SkipInit
	info.All = opts.All
	info.Affected = opts.Affected
	info.Query = opts.Query

	// Backend execution flags (only apply if set via CLI).
	if opts.AutoGenerateBackendFile != "" {
		info.AutoGenerateBackendFile = opts.AutoGenerateBackendFile
	}
	if opts.InitRunReconfigure != "" {
		info.InitRunReconfigure = opts.InitRunReconfigure
	}
	if opts.InitPassVars {
		info.InitPassVars = "true"
	}

	// Plan/Apply/Deploy specific flags.
	if opts.PlanFile != "" {
		info.PlanFile = opts.PlanFile
		info.UseTerraformPlan = true
	}
	if opts.PlanSkipPlanfile {
		info.PlanSkipPlanfile = "true"
	}
	if opts.DeployRunInit {
		info.DeployRunInit = "true"
	}
}

// terraformRunWithOptions is the shared execution logic for terraform subcommands.
// Commands with their own parsers (plan, apply, deploy) bind their parsers in RunE.
func terraformRunWithOptions(parentCmd, actualCmd *cobra.Command, args []string, opts *TerraformRunOptions) error {
	subCommand := actualCmd.Name()
	log.Debug("terraformRunWithOptions entry", "subCommand", subCommand, "args", args)

	// Validate Atmos config first to provide specific error messages.
	if err := internal.ValidateAtmosConfig(); err != nil {
		return err
	}

	// Build info from args. SeparatedArgs are terraform pass-through flags.
	separatedArgs := compat.GetSeparated()
	argsWithSubCommand := append([]string{subCommand}, args...)
	info, err := e.ProcessCommandLineArgs(cfg.TerraformComponentType, parentCmd, argsWithSubCommand, separatedArgs)
	if err != nil {
		return err
	}

	// Resolve paths and prompt for missing component/stack interactively.
	if err := resolveAndPromptForArgs(&info, actualCmd); err != nil {
		return err
	}

	if info.NeedHelp {
		err := actualCmd.Usage()
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	applyOptionsToInfo(&info, opts)

	// Handle --identity flag for interactive selection when used without a value.
	if info.Identity == cfg.IdentityFlagSelectValue {
		handleInteractiveIdentitySelection(&info)
	}

	// Check Terraform Single-Component and Multi-Component flags.
	err = checkTerraformFlags(&info)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	// Route to appropriate execution path.
	if info.Affected {
		return executeAffectedCommand(parentCmd, args, &info)
	}
	if isMultiComponentExecution(&info) {
		log.Debug("Routing to ExecuteTerraformQuery (multi-component)")
		err = e.ExecuteTerraformQuery(&info)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}
	return executeSingleComponent(&info)
}

// hasMultiComponentFlags checks if any multi-component flags are set.
func hasMultiComponentFlags(info *schema.ConfigAndStacksInfo) bool {
	return info.All || info.Affected || len(info.Components) > 0 || info.Query != ""
}

// hasNonAffectedMultiFlags checks if multi-component flags (excluding --affected) are set.
func hasNonAffectedMultiFlags(info *schema.ConfigAndStacksInfo) bool {
	return info.All || len(info.Components) > 0 || info.Query != ""
}

// hasSingleComponentFlags checks if single-component flags are set.
func hasSingleComponentFlags(info *schema.ConfigAndStacksInfo) bool {
	return info.PlanFile != "" || info.UseTerraformPlan
}

// checkTerraformFlags checks the usage of the Single-Component and Multi-Component flags.
func checkTerraformFlags(info *schema.ConfigAndStacksInfo) error {
	// Check Multi-Component flags.
	// 1. Specifying the `component` argument is not allowed with the Multi-Component flags.
	if info.ComponentFromArg != "" && hasMultiComponentFlags(info) {
		return fmt.Errorf("component `%s`: %w", info.ComponentFromArg, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	}
	// 2. `--affected` is not allowed with the other Multi-Component flags.
	if info.Affected && hasNonAffectedMultiFlags(info) {
		return errUtils.ErrInvalidTerraformFlagsWithAffectedFlag
	}

	// Single-Component and Multi-Component flags are not allowed together.
	if hasSingleComponentFlags(info) && hasMultiComponentFlags(info) {
		return errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags
	}

	return nil
}

// handleInteractiveIdentitySelection handles the case where --identity was used without a value.
func handleInteractiveIdentitySelection(info *schema.ConfigAndStacksInfo) {
	// Initialize CLI config to get auth configuration.
	// Use false to skip stack processing - only auth config is needed.
	atmosConfig, err := cfg.InitCliConfig(*info, false)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf(errWrapFormat, errUtils.ErrInitializeCLIConfig, err), "", "")
	}

	// Check if auth is configured. If not, we can't select an identity.
	if len(atmosConfig.Auth.Providers) == 0 && len(atmosConfig.Auth.Identities) == 0 {
		// User explicitly requested identity selection (--identity or --identity=)
		// but no authentication is configured. This is an error.
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: no authentication configured", errUtils.ErrNoIdentitiesAvailable), "", "")
	}

	// Create auth manager to enable identity selection.
	// Use auth.CreateAndAuthenticateManager directly to avoid import cycle.
	authManager, err := auth.CreateAndAuthenticateManager(
		cfg.IdentityFlagSelectValue,
		&atmosConfig.Auth,
		cfg.IdentityFlagSelectValue,
	)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf(errWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err), "", "")
	}

	// Get default identity with forced interactive selection.
	// GetDefaultIdentity() handles TTY and CI detection via isInteractive().
	selectedIdentity, err := authManager.GetDefaultIdentity(true)
	if err != nil {
		// Check if user explicitly aborted (Ctrl+C, ESC, etc.).
		// In this case, we want to exit immediately without showing an error.
		if errors.Is(err, errUtils.ErrUserAborted) {
			log.Debug("User aborted identity selection, exiting with SIGINT code")
			// Exit immediately with POSIX SIGINT exit code.
			// Note: We bypass error formatting as user abort is not an error condition.
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		errUtils.CheckErrorPrintAndExit(fmt.Errorf(errWrapFormat, errUtils.ErrDefaultIdentity, err), "", "")
	}

	info.Identity = selectedIdentity
}

// resolveAndPromptForArgs handles path resolution and interactive prompts for component/stack.
func resolveAndPromptForArgs(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	// Resolve path-based component arguments (e.g., ".", "./vpc", absolute paths).
	if info.NeedsPathResolution && info.ComponentFromArg != "" {
		if err := resolveComponentPath(info, cfg.TerraformComponentType); err != nil {
			return err
		}
	}
	// Handle interactive component/stack selection (skipped for multi-component ops).
	return handleInteractiveComponentStackSelection(info, cmd)
}

// handleInteractiveComponentStackSelection prompts for missing component and stack
// when running in interactive mode. Skipped for multi-component operations.
func handleInteractiveComponentStackSelection(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	// Skip if multi-component mode or help requested.
	if hasMultiComponentFlags(info) || info.NeedHelp {
		return nil
	}

	// Both provided - nothing to do.
	if info.ComponentFromArg != "" && info.Stack != "" {
		return nil
	}

	// Prompt for component if missing.
	if info.ComponentFromArg == "" {
		component, err := promptForComponent(cmd)
		if err = handlePromptError(err, "component"); err != nil {
			return err
		}
		info.ComponentFromArg = component
	}

	// Prompt for stack if missing.
	if info.Stack == "" {
		stack, err := promptForStack(cmd, info.ComponentFromArg)
		if err = handlePromptError(err, "stack"); err != nil {
			return err
		}
		info.Stack = stack
	}

	return nil
}

// handlePromptError delegates to shared.HandlePromptError.
func handlePromptError(err error, name string) error {
	return shared.HandlePromptError(err, name)
}

// promptForComponent delegates to shared.PromptForComponent.
func promptForComponent(cmd *cobra.Command) (string, error) {
	return shared.PromptForComponent(cmd)
}

// promptForStack delegates to shared.PromptForStack.
func promptForStack(cmd *cobra.Command, component string) (string, error) {
	return shared.PromptForStack(cmd, component)
}

// enableHeatmapIfRequested checks os.Args for --heatmap flag and enables performance tracking.
// This is needed because --heatmap must be detected before flag parsing occurs.
// We only enable tracking if --heatmap is present; --heatmap-mode is only relevant when --heatmap is set.
func enableHeatmapIfRequested() {
	for _, arg := range os.Args {
		if arg == "--heatmap" {
			perf.EnableTracking(true)
			return
		}
	}
}

// identityFlagCompletion provides shell completion for identity flags by fetching
// available identities from the Atmos configuration.
func identityFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

// addIdentityCompletion registers shell completion for the identity flag if present on the command.
// The identity flag may be defined directly on the command or inherited from a parent command.
func addIdentityCompletion(cmd *cobra.Command) {
	// Check both local flags and inherited flags.
	flag := cmd.Flag("identity")
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup("identity")
	}
	if flag != nil {
		if err := cmd.RegisterFlagCompletionFunc("identity", identityFlagCompletion); err != nil {
			log.Trace("Failed to register identity flag completion", "error", err)
		}
	}
}

// componentsArgCompletion delegates to shared.ComponentsArgCompletion.
func componentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return shared.ComponentsArgCompletion(cmd, args, toComplete)
}

// stackFlagCompletion delegates to shared.StackFlagCompletion.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return shared.StackFlagCompletion(cmd, args, toComplete)
}
