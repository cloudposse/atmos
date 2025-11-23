package terraform

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	h "github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

func runHooks(event h.HookEvent, cmd_ *cobra.Command, args []string) error {
	// Process args (split on --) and get config
	var argsAfterDoubleDash []string
	finalArgs := append([]string{cmd_.Name()}, args...)

	doubleDashIndex := lo.IndexOf(finalArgs, "--")
	if doubleDashIndex > 0 {
		finalArgs = lo.Slice(finalArgs, 0, doubleDashIndex)
		argsAfterDoubleDash = lo.Slice(finalArgs, doubleDashIndex+1, len(finalArgs))
	}

	info, err := e.ProcessCommandLineArgs("terraform", cmd_, finalArgs, argsAfterDoubleDash)
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

func terraformRun(parentCmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	// Get compatibility flags for this subcommand.
	subCommand := actualCmd.Name()
	compatFlags := GetCompatFlagsForCommand(subCommand)

	// Create translator to separate Atmos flags from terraform pass-through flags.
	translator := compat.NewCompatibilityFlagTranslator(compatFlags)

	// Create unified parser that handles Atmos flags + pass-through separation.
	// The registry is nil because flags are already registered on the command;
	// the parser will use the command's existing flags.
	parser := flags.NewAtmosFlagParser(actualCmd, viper.GetViper(), translator, nil)

	// Parse args using the unified parser.
	parsedConfig, err := parser.Parse(args)
	if err != nil {
		return err
	}

	// Build info from parsed config.
	info, err := e.ProcessCommandLineArgs(cfg.TerraformComponentType, parentCmd, parsedConfig.PositionalArgs, parsedConfig.SeparatedArgs)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		err := actualCmd.Usage()
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Get flag values from Viper (respects precedence: flag > env > config > default).
	v := viper.GetViper()
	info.ProcessTemplates = v.GetBool("process-templates")
	info.ProcessFunctions = v.GetBool("process-functions")
	info.Skip = v.GetStringSlice("skip")
	info.Components = v.GetStringSlice("components")
	info.DryRun = v.GetBool("dry-run")

	// Handle --identity flag for interactive selection.
	// ProcessCommandLineArgs already parsed the identity value correctly via processArgsAndFlags.
	// We only need to handle the special case where --identity was used without a value (interactive selection).
	// Note: We cannot use flags.GetString("identity") here because Cobra's NoOptDefVal behavior
	// with positional args causes it to return "__SELECT__" even when a value was provided
	// (e.g., "atmos terraform plan vpc --identity asd" treats "asd" as positional, not flag value).
	if info.Identity == cfg.IdentityFlagSelectValue {
		handleInteractiveIdentitySelection(&info)
	}
	// Otherwise, info.Identity already has the correct value from ProcessCommandLineArgs
	// (either from --identity <value>, ATMOS_IDENTITY env var, or empty string).
	// Check Terraform Single-Component and Multi-Component flags
	err = checkTerraformFlags(&info)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	// Execute `atmos terraform <sub-command> --affected` or `atmos terraform <sub-command> --affected --stack <stack>`
	if info.Affected {
		// Add these flags because `atmos describe affected` needs them, but `atmos terraform --affected` does not define them
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

		err = e.ExecuteTerraformAffected(&a, &info)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Execute `atmos terraform <sub-command>` with the filters if any of the following flags are specified:
	// `--all`
	// `--components c1,c2`
	// `--query <yq-expression>`
	// `--stack` (and the `component` argument is not passed)
	if info.All || len(info.Components) > 0 || info.Query != "" || (info.Stack != "" && info.ComponentFromArg == "") {
		err = e.ExecuteTerraformQuery(&info)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Execute `atmos terraform <sub-command> <component> --stack <stack>`
	err = e.ExecuteTerraform(info)
	// For plan-diff, ExecuteTerraform will call OsExit directly if there are differences
	// So if we get here, it means there were no differences or there was an error
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			// Print the error message but return the error to be handled by main.go
			errUtils.CheckErrorAndPrint(err, "", "")
			return err
		}
		// For other errors, continue with existing behavior
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	return nil
}

// checkTerraformFlags checks the usage of the Single-Component and Multi-Component flags.
func checkTerraformFlags(info *schema.ConfigAndStacksInfo) error {
	// Check Multi-Component flags
	// 1. Specifying the `component` argument is not allowed with the Multi-Component flags
	if info.ComponentFromArg != "" && (info.All || info.Affected || len(info.Components) > 0 || info.Query != "") {
		return fmt.Errorf("component `%s`: %w", info.ComponentFromArg, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	}
	// 2. `--affected` is not allowed with the other Multi-Component flags
	if info.Affected && (info.All || len(info.Components) > 0 || info.Query != "") {
		return errUtils.ErrInvalidTerraformFlagsWithAffectedFlag
	}

	// Single-Component and Multi-Component flags are not allowed together
	singleComponentFlagPassed := info.PlanFile != "" || info.UseTerraformPlan
	multiComponentFlagPassed := info.Affected || info.All || len(info.Components) > 0 || info.Query != ""
	if singleComponentFlagPassed && multiComponentFlagPassed {
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
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err), "", "")
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
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err), "", "")
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
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrDefaultIdentity, err), "", "")
	}

	info.Identity = selectedIdentity
}

// enableHeatmapIfRequested checks os.Args for --heatmap flag and enables performance tracking.
// This is needed for commands with DisableFlagParsing=true (terraform, helmfile, packer)
// where Cobra doesn't parse the flags, so PersistentPreRun can't detect them.
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

// setCustomHelp sets up a custom help function for a terraform subcommand that includes
// compatibility flags documentation in the help output.
func setCustomHelp(cmd *cobra.Command, descriptions []CompatFlagDescription) {
	originalHelp := cmd.HelpFunc()

	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		// Call original help first.
		originalHelp(c, args)

		// Append compatibility flags section.
		if len(descriptions) > 0 {
			fmt.Print(FormatCompatFlagsHelp(descriptions))
		}
	})
}

// componentsArgCompletion provides shell completion for component positional arguments.
func componentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		output, err := listTerraformComponents(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// stackFlagCompletion provides shell completion for the --stack flag.
// If a component was provided as the first positional argument, it filters stacks
// to only those containing that component.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component.
	if len(args) > 0 && args[0] != "" {
		output, err := listStacksForComponent(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks.
	output, err := listAllStacks()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// listTerraformComponents lists all terraform components.
func listTerraformComponents(cmd *cobra.Command) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %w", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %w", err)
	}

	// Collect unique component names from all stacks.
	componentSet := make(map[string]struct{})
	for _, stackData := range stacksMap {
		if stackMap, ok := stackData.(map[string]any); ok {
			if components, ok := stackMap["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					for componentName := range terraform {
						componentSet[componentName] = struct{}{}
					}
				}
			}
		}
	}

	components := make([]string, 0, len(componentSet))
	for name := range componentSet {
		components = append(components, name)
	}
	sort.Strings(components)
	return components, nil
}

// listStacksForComponent returns stacks that contain the specified component.
func listStacksForComponent(component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %w", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %w", err)
	}

	// Filter stacks that contain the specified component.
	var stacks []string
	for stackName, stackData := range stacksMap {
		if stackMap, ok := stackData.(map[string]any); ok {
			if components, ok := stackMap["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					if _, hasComponent := terraform[component]; hasComponent {
						stacks = append(stacks, stackName)
					}
				}
			}
		}
	}
	sort.Strings(stacks)
	return stacks, nil
}

// listAllStacks returns all stacks.
func listAllStacks() ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %w", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %w", err)
	}

	stacks := make([]string, 0, len(stacksMap))
	for stackName := range stacksMap {
		stacks = append(stacks, stackName)
	}
	sort.Strings(stacks)
	return stacks, nil
}
