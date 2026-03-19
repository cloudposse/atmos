package exec

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/filetype"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Error format constants.
const (
	errFlagFormat = "%w: flag: %s"
)

// Terraform compound subcommand constants.
const (
	cmdWrite     = "write"
	cmdVarfile   = "varfile"
	cmdWorkspace = "workspace"
	cmdState     = "state"
	cmdProviders = "providers"
	// Format string for compound subcommands like "state list".
	cmdFmtSpaced = "%s %s"
)

// `commonFlags` are a list of flags that Atmos understands, but the underlying tools do not (e.g., Terraform/OpenTofu, Helmfile, Packer, etc.).
// These flags get removed from the arg list after Atmos uses them, so the underlying tool does not get passed a flag it doesn't accept.
var commonFlags = []string{
	"--stack",
	"-s",
	cfg.DryRunFlag,
	cfg.SkipInitFlag,
	cfg.KubeConfigConfigFlag,
	cfg.TerraformCommandFlag,
	cfg.TerraformDirFlag,
	cfg.HelmfileCommandFlag,
	cfg.HelmfileDirFlag,
	cfg.CliConfigDirFlag,
	cfg.StackDirFlag,
	cfg.BasePathFlag,
	cfg.VendorBasePathFlag,
	cfg.GlobalOptionsFlag,
	cfg.DeployRunInitFlag,
	cfg.InitRunReconfigure,
	cfg.AutoGenerateBackendFileFlag,
	cfg.AppendUserAgentFlag,
	cfg.FromPlanFlag,
	cfg.PlanFileFlag,
	cfg.HelpFlag1,
	cfg.HelpFlag2,
	cfg.WorkflowDirFlag,
	cfg.JsonSchemaDirFlag,
	cfg.OpaDirFlag,
	cfg.CueDirFlag,
	cfg.AtmosManifestJsonSchemaFlag,
	cfg.RedirectStdErrFlag,
	cfg.LogsLevelFlag,
	cfg.LogsFileFlag,
	cfg.QueryFlag,
	cfg.SettingsListMergeStrategyFlag,
	cfg.ProcessTemplatesFlag,
	cfg.ProcessFunctionsFlag,
	cfg.SkipFlag,
	cfg.AffectedFlag,
	cfg.AllFlag,
	cfg.InitPassVars,
	cfg.PlanSkipPlanfile,
	cfg.IdentityFlag,
	cfg.ClusterNameFlag,
	cfg.ProfilerEnabledFlag,
	cfg.ProfilerHostFlag,
	cfg.ProfilerPortFlag,
	cfg.ProfilerFileFlag,
	cfg.ProfilerTypeFlag,
	cfg.HeatmapFlag,
	cfg.HeatmapModeFlag,
	cfg.AtmosProfileFlag,
}

// ProcessCommandLineArgs processes command-line args.
func ProcessCommandLineArgs(
	componentType string,
	cmd *cobra.Command,
	args []string,
	additionalArgsAndFlags []string,
) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "exec.ProcessCommandLineArgs")()

	var configAndStacksInfo schema.ConfigAndStacksInfo

	log.Debug("ProcessCommandLineArgs input", "componentType", componentType, "args", args)

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil && !errors.Is(err, pflag.ErrHelp) {
		return configAndStacksInfo, err
	}

	// Check what Cobra parsed for identity flag.
	if identityFlag := cmd.Flag("identity"); identityFlag != nil {
		log.Debug("After ParseFlags", "identity.Value", identityFlag.Value.String(), "identity.Changed", identityFlag.Changed)
	}

	argsAndFlagsInfo, err := processArgsAndFlags(componentType, args)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.BasePath, err = cmd.Flags().GetString("base-path")
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.AtmosConfigFilesFromArg, err = cmd.Flags().GetStringSlice("config")
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.AtmosConfigDirsFromArg, err = cmd.Flags().GetStringSlice("config-path")
	if err != nil {
		return configAndStacksInfo, err
	}
	// Read profile flag and env var.
	// Check flag first, then fall back to ATMOS_PROFILE env var if flag not set.
	profiles, err := cmd.Flags().GetStringSlice("profile")
	if err != nil {
		return configAndStacksInfo, err
	}
	if len(profiles) == 0 {
		//nolint:forbidigo // Must use os.Getenv: profile is processed before Viper configuration loads.
		if envProfiles := os.Getenv("ATMOS_PROFILE"); envProfiles != "" {
			// Split comma-separated profiles from env var and filter out empty entries.
			raw := strings.Split(envProfiles, ",")
			for _, p := range raw {
				if v := strings.TrimSpace(p); v != "" {
					profiles = append(profiles, v)
				}
			}
		}
	}
	configAndStacksInfo.ProfilesFromArg = profiles
	finalAdditionalArgsAndFlags := argsAndFlagsInfo.AdditionalArgsAndFlags
	if len(additionalArgsAndFlags) > 0 {
		finalAdditionalArgsAndFlags = append(finalAdditionalArgsAndFlags, additionalArgsAndFlags...)
	}

	configAndStacksInfo.AdditionalArgsAndFlags = finalAdditionalArgsAndFlags
	configAndStacksInfo.SubCommand = argsAndFlagsInfo.SubCommand
	configAndStacksInfo.SubCommand2 = argsAndFlagsInfo.SubCommand2
	configAndStacksInfo.ComponentType = componentType
	configAndStacksInfo.ComponentFromArg = argsAndFlagsInfo.ComponentFromArg
	configAndStacksInfo.GlobalOptions = argsAndFlagsInfo.GlobalOptions
	configAndStacksInfo.TerraformCommand = argsAndFlagsInfo.TerraformCommand
	configAndStacksInfo.TerraformDir = argsAndFlagsInfo.TerraformDir
	configAndStacksInfo.HelmfileCommand = argsAndFlagsInfo.HelmfileCommand
	configAndStacksInfo.HelmfileDir = argsAndFlagsInfo.HelmfileDir
	configAndStacksInfo.StacksDir = argsAndFlagsInfo.StacksDir
	configAndStacksInfo.ConfigDir = argsAndFlagsInfo.ConfigDir
	configAndStacksInfo.WorkflowsDir = argsAndFlagsInfo.WorkflowsDir
	configAndStacksInfo.DeployRunInit = argsAndFlagsInfo.DeployRunInit
	configAndStacksInfo.InitRunReconfigure = argsAndFlagsInfo.InitRunReconfigure
	configAndStacksInfo.InitPassVars = argsAndFlagsInfo.InitPassVars
	configAndStacksInfo.PlanSkipPlanfile = argsAndFlagsInfo.PlanSkipPlanfile
	configAndStacksInfo.AutoGenerateBackendFile = argsAndFlagsInfo.AutoGenerateBackendFile
	configAndStacksInfo.UseTerraformPlan = argsAndFlagsInfo.UseTerraformPlan
	configAndStacksInfo.PlanFile = argsAndFlagsInfo.PlanFile
	configAndStacksInfo.DryRun = argsAndFlagsInfo.DryRun
	configAndStacksInfo.SkipInit = argsAndFlagsInfo.SkipInit
	configAndStacksInfo.NeedHelp = argsAndFlagsInfo.NeedHelp
	configAndStacksInfo.JsonSchemaDir = argsAndFlagsInfo.JsonSchemaDir
	configAndStacksInfo.AtmosManifestJsonSchema = argsAndFlagsInfo.AtmosManifestJsonSchema
	configAndStacksInfo.OpaDir = argsAndFlagsInfo.OpaDir
	configAndStacksInfo.CueDir = argsAndFlagsInfo.CueDir
	configAndStacksInfo.RedirectStdErr = argsAndFlagsInfo.RedirectStdErr
	configAndStacksInfo.LogsLevel = argsAndFlagsInfo.LogsLevel
	configAndStacksInfo.LogsFile = argsAndFlagsInfo.LogsFile
	configAndStacksInfo.SettingsListMergeStrategy = argsAndFlagsInfo.SettingsListMergeStrategy
	configAndStacksInfo.Query = argsAndFlagsInfo.Query
	configAndStacksInfo.Identity = argsAndFlagsInfo.Identity
	configAndStacksInfo.ClusterName = argsAndFlagsInfo.ClusterName
	configAndStacksInfo.NeedsPathResolution = argsAndFlagsInfo.NeedsPathResolution

	// Fallback to ATMOS_IDENTITY environment variable if identity not set via flag.
	// Use os.Getenv directly to avoid polluting viper config with temporary binding.
	// Normalize the value to handle boolean false representations (false, 0, no, off).
	if configAndStacksInfo.Identity == "" {
		if envIdentity := os.Getenv("ATMOS_IDENTITY"); envIdentity != "" { //nolint:forbidigo // Direct env var read to avoid viper config pollution
			configAndStacksInfo.Identity = cfg.NormalizeIdentityValue(envIdentity)
		}
	}

	configAndStacksInfo.Affected = argsAndFlagsInfo.Affected
	configAndStacksInfo.All = argsAndFlagsInfo.All
	configAndStacksInfo.PackerDir = argsAndFlagsInfo.PackerDir
	configAndStacksInfo.PackerCommand = argsAndFlagsInfo.PackerCommand

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err == nil && stack != "" {
		configAndStacksInfo.Stack = stack
	}

	return configAndStacksInfo, nil
}

// compoundSubcommandResult holds the result of parsing a compound terraform subcommand
// (a command with its own subcommand, e.g., "providers lock", "state list").
type compoundSubcommandResult struct {
	subCommand  string
	subCommand2 string
	argCount    int // Number of arguments consumed (1 for quoted, 2 for separate).
}

// Known terraform compound subcommands (commands that have their own subcommands).
var (
	workspaceSubcommands = []string{"list", "select", "new", "delete", "show"}
	stateSubcommands     = []string{"list", "mv", "pull", "push", "replace-provider", "rm", "show"}
	providersSubcommands = []string{"lock", "mirror", "schema"}
)

// parseCompoundSubcommand checks if the arguments represent a terraform compound subcommand
// (a command with its own subcommand, e.g., "providers lock", "state list").
// It handles both quoted forms (e.g., "providers lock") and separate forms (e.g., "providers", "lock").
// Returns nil if no compound subcommand is detected.
func parseCompoundSubcommand(args []string) *compoundSubcommandResult {
	if len(args) == 0 {
		return nil
	}

	// First, check if the first argument is a quoted compound subcommand (e.g., "providers lock").
	firstArg := args[0]
	if strings.Contains(firstArg, " ") {
		if result := parseQuotedCompoundSubcommand(firstArg); result != nil {
			return result
		}
	}

	// If not a quoted command, check for separate word forms (e.g., "providers", "lock").
	if len(args) > 1 {
		return parseSeparateCompoundSubcommand(args[0], args[1])
	}

	return nil
}

// parseQuotedCompoundSubcommand parses a quoted compound subcommand like "providers lock".
// It is only called from parseCompoundSubcommand when strings.Contains(arg, " ") is true,
// which guarantees SplitN returns exactly 2 parts.  The defensive guard below protects
// against future refactoring that could call this function with a no-space string.
func parseQuotedCompoundSubcommand(arg string) *compoundSubcommandResult {
	parts := strings.SplitN(arg, " ", 2)
	// Defensive guard: this function is only called when arg contains a space
	// (see parseCompoundSubcommand), but this check protects against callers
	// that may not enforce that invariant in the future.
	if len(parts) != 2 {
		return nil
	}
	first, second := parts[0], parts[1]

	switch first {
	case cmdWrite:
		if second == cmdVarfile {
			return &compoundSubcommandResult{subCommand: cmdWrite, subCommand2: cmdVarfile, argCount: 1}
		}
	case cmdWorkspace:
		if u.SliceContainsString(workspaceSubcommands, second) {
			return &compoundSubcommandResult{subCommand: cmdWorkspace, subCommand2: second, argCount: 1}
		}
	case cmdState:
		if u.SliceContainsString(stateSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdState, second), argCount: 1}
		}
	case cmdProviders:
		if u.SliceContainsString(providersSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdProviders, second), argCount: 1}
		}
	}

	return nil
}

// processTerraformCompoundSubcommand handles terraform compound subcommands (e.g., "providers lock").
// Returns true if a compound subcommand was processed, false otherwise.
func processTerraformCompoundSubcommand(info *schema.ArgsAndFlagsInfo, args []string) (bool, error) {
	result := parseCompoundSubcommand(args)
	if result == nil {
		return false, nil
	}

	info.SubCommand = result.subCommand
	info.SubCommand2 = result.subCommand2

	// The component argument is at index argCount.
	// For quoted commands like "providers lock", argCount=1, so component is at [1].
	// For separate commands like "providers" "lock", argCount=2, so component is at [2].
	componentArgIndex := result.argCount
	if len(args) <= componentArgIndex {
		return true, fmt.Errorf("%w: command %q requires an argument", errUtils.ErrMissingComponent, info.SubCommand)
	}

	info.ComponentFromArg = args[componentArgIndex]
	if comp.IsExplicitComponentPath(info.ComponentFromArg) {
		info.NeedsPathResolution = true
	}
	if len(args) > componentArgIndex+1 {
		info.AdditionalArgsAndFlags = args[componentArgIndex+1:]
	}

	return true, nil
}

// processSingleCommand handles parsing for single-word commands (non-two-word commands).
// It sets the subcommand, component, and additional args on the info struct.
func processSingleCommand(info *schema.ArgsAndFlagsInfo, args []string) error {
	info.SubCommand = args[0]
	if len(args) <= 1 {
		return nil
	}

	secondArg := args[1]
	if len(secondArg) == 0 {
		return fmt.Errorf("%w: invalid empty argument provided", errUtils.ErrInvalidArguments)
	}

	if strings.HasPrefix(secondArg, "--") {
		if len(secondArg) <= 2 {
			return fmt.Errorf("%w: invalid option format: %s", errUtils.ErrInvalidArguments, secondArg)
		}
		info.AdditionalArgsAndFlags = args[1:]
		return nil
	}

	info.ComponentFromArg = secondArg
	// Check if argument is an explicit path that needs resolution.
	// Only resolve as a filesystem path if the argument explicitly indicates a path:
	// - "." (current directory).
	// - Starts with "./" or "../" (relative path).
	// - Starts with "/" (absolute path).
	// Otherwise, treat it as a component name (even if it contains slashes).
	if comp.IsExplicitComponentPath(secondArg) {
		info.NeedsPathResolution = true
	}

	if len(args) > 2 {
		info.AdditionalArgsAndFlags = args[2:]
	}

	return nil
}

// parseSeparateCompoundSubcommand parses a compound subcommand passed as separate arguments.
func parseSeparateCompoundSubcommand(first, second string) *compoundSubcommandResult {
	switch first {
	case cmdWrite:
		if second == cmdVarfile {
			return &compoundSubcommandResult{subCommand: cmdWrite, subCommand2: cmdVarfile, argCount: 2}
		}
	case cmdWorkspace:
		if u.SliceContainsString(workspaceSubcommands, second) {
			return &compoundSubcommandResult{subCommand: cmdWorkspace, subCommand2: second, argCount: 2}
		}
	case cmdState:
		if u.SliceContainsString(stateSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdState, second), argCount: 2}
		}
	case cmdProviders:
		if u.SliceContainsString(providersSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdProviders, second), argCount: 2}
		}
	}

	return nil
}

// stringFlagDef associates a CLI flag with a field setter on ArgsAndFlagsInfo.
// The table-driven approach in stringFlagDefs eliminates repetitive if/else chains.
type stringFlagDef struct {
	flag    string
	setFunc func(*schema.ArgsAndFlagsInfo, string)
}

// stringFlagDefs maps string-valued CLI flags to their ArgsAndFlagsInfo setters.
// parseFlagValue handles both "--flag value" and "--flag=value" forms for each entry.
// Flags with special semantics (--identity, --from-plan) are handled separately.
var stringFlagDefs = []stringFlagDef{
	{cfg.TerraformCommandFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.TerraformCommand = v }},
	{cfg.TerraformDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.TerraformDir = v }},
	{cfg.AppendUserAgentFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.AppendUserAgent = v }},
	{cfg.HelmfileCommandFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.HelmfileCommand = v }},
	{cfg.HelmfileDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.HelmfileDir = v }},
	{cfg.CliConfigDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.ConfigDir = v }},
	{cfg.StackDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.StacksDir = v }},
	{cfg.BasePathFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.BasePath = v }},
	{cfg.VendorBasePathFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.VendorBasePath = v }},
	{cfg.DeployRunInitFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.DeployRunInit = v }},
	{cfg.AutoGenerateBackendFileFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.AutoGenerateBackendFile = v }},
	{cfg.WorkflowDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.WorkflowsDir = v }},
	{cfg.InitRunReconfigure, func(info *schema.ArgsAndFlagsInfo, v string) { info.InitRunReconfigure = v }},
	{cfg.InitPassVars, func(info *schema.ArgsAndFlagsInfo, v string) { info.InitPassVars = v }},
	{cfg.PlanSkipPlanfile, func(info *schema.ArgsAndFlagsInfo, v string) { info.PlanSkipPlanfile = v }},
	{cfg.JsonSchemaDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.JsonSchemaDir = v }},
	{cfg.OpaDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.OpaDir = v }},
	{cfg.CueDirFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.CueDir = v }},
	{cfg.AtmosManifestJsonSchemaFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.AtmosManifestJsonSchema = v }},
	{cfg.RedirectStdErrFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.RedirectStdErr = v }},
	{cfg.LogsLevelFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.LogsLevel = v }},
	{cfg.LogsFileFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.LogsFile = v }},
	{cfg.SettingsListMergeStrategyFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.SettingsListMergeStrategy = v }},
	{cfg.QueryFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.Query = v }},
	{cfg.ClusterNameFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.ClusterName = v }},
	// --planfile sets two fields; UseTerraformPlan is always true when a planfile is given.
	{cfg.PlanFileFlag, func(info *schema.ArgsAndFlagsInfo, v string) { info.PlanFile = v; info.UseTerraformPlan = true }},
}

// valueTakingCommonFlags is the subset of commonFlags whose space-separated form ("--flag value")
// consumes the next argument as its value.  Boolean-only flags (e.g., --dry-run, --skip-init,
// --affected, --all, --process-templates, --heatmap, --profiler-enabled) are NOT included because
// they never consume i+1; blindly stripping i+1 for them would silently drop an unrelated
// flag (e.g., --refresh=false) that the user intended to pass to the underlying tool.
//
// The set is derived from stringFlagDefs (all of which are value-taking) plus the remaining
// value-taking entries in commonFlags that are handled outside stringFlagDefs.
var valueTakingCommonFlags = func() map[string]bool {
	set := make(map[string]bool, len(stringFlagDefs)+16)
	for _, def := range stringFlagDefs {
		set[def.flag] = true
	}
	// --stack / -s take a stack-name value.
	set["--stack"] = true
	set["-s"] = true
	// --global-options takes a string value in space form.
	set[cfg.GlobalOptionsFlag] = true
	// --kubeconfig-path takes a path value.
	set[cfg.KubeConfigConfigFlag] = true
	// Profiler string flags.
	set[cfg.ProfilerHostFlag] = true
	set[cfg.ProfilerPortFlag] = true
	set[cfg.ProfilerFileFlag] = true
	set[cfg.ProfilerTypeFlag] = true
	// --heatmap-mode and --profile take string values.
	set[cfg.HeatmapModeFlag] = true
	set[cfg.AtmosProfileFlag] = true
	// --skip is a StringSlice flag that can be used in space form ("--skip funcname").
	set[cfg.SkipFlag] = true
	return set
}()

// parseFlagValue extracts the value for a CLI flag from the current argument.
// It handles both space-separated ("--flag value") and equals-separated ("--flag=value") forms.
// Returns ("", false, nil) when arg does not match flag.
// Returns (value, true, nil) on success.
// Returns ("", false, err) when the flag matches but its value is missing or malformed.
//
// SplitN(arg, "=", 2) is used so that values containing "=" (e.g., --query=.tags[?env==prod])
// are handled correctly — only the first "=" is treated as the separator.
func parseFlagValue(flag, arg string, args []string, index int) (string, bool, error) {
	if arg == flag {
		if index+1 >= len(args) {
			return "", false, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
		}
		return args[index+1], true, nil
	}
	if strings.HasPrefix(arg, flag+"=") {
		// SplitN(..., 2) keeps any additional "=" in the value intact.
		parts := strings.SplitN(arg, "=", 2)
		return parts[1], true, nil
	}
	return "", false, nil
}

// parseIdentityFlag handles --identity which supports optional and empty values.
//
//   - --identity         → __SELECT__ (interactive selection).
//   - --identity value   → use value.
//   - --identity=value   → use value.
//   - --identity=        → __SELECT__ (interactive selection).
//
// SplitN(arg, "=", 2) is used so that identity values containing "=" (e.g., ARN-like
// strings with key=value parameters) are handled correctly.
func parseIdentityFlag(info *schema.ArgsAndFlagsInfo, arg string, args []string, index int) {
	if arg == cfg.IdentityFlag {
		// Has value: --identity <value> (next arg exists and is not another flag).
		if len(args) > index+1 && !strings.HasPrefix(args[index+1], "-") {
			info.Identity = args[index+1]
		} else {
			// No value: --identity (interactive selection).
			info.Identity = cfg.IdentityFlagSelectValue
		}
		return
	}
	if strings.HasPrefix(arg, cfg.IdentityFlag+"=") {
		// SplitN(..., 2) keeps any additional "=" in the value intact.
		parts := strings.SplitN(arg, "=", 2)
		if parts[1] == "" {
			// Empty value: --identity= (interactive selection).
			info.Identity = cfg.IdentityFlagSelectValue
		} else {
			info.Identity = parts[1]
		}
	}
}

// parseFromPlanFlag handles --from-plan which has optional value semantics.
//
//   - --from-plan           → UseTerraformPlan = true, PlanFile unchanged.
//   - --from-plan <path>    → UseTerraformPlan = true, PlanFile = path.
//   - --from-plan=<path>    → UseTerraformPlan = true, PlanFile = path.
//   - --from-plan=          → UseTerraformPlan = true, PlanFile unchanged.
func parseFromPlanFlag(info *schema.ArgsAndFlagsInfo, arg string, args []string, index int) {
	if arg == cfg.FromPlanFlag {
		info.UseTerraformPlan = true
		// Check if next argument is the planfile path (not another flag).
		if len(args) > index+1 && !strings.HasPrefix(args[index+1], "-") {
			info.PlanFile = args[index+1]
		}
		return
	}
	if strings.HasPrefix(arg, cfg.FromPlanFlag+"=") {
		info.UseTerraformPlan = true
		planFilePath := strings.TrimPrefix(arg, cfg.FromPlanFlag+"=")
		if planFilePath != "" {
			info.PlanFile = planFilePath
		}
	}
}

// processArgsAndFlags processes args and flags from the provided CLI arguments/flags
//
// Deprecated: use Cobra command flag parser instead.
// Post https://github.com/cloudposse/atmos/pull/1174 we can use the API provided by this PR for better handling of flags.
func processArgsAndFlags(
	componentType string,
	inputArgsAndFlags []string,
) (schema.ArgsAndFlagsInfo, error) {
	var info schema.ArgsAndFlagsInfo
	var additionalArgsAndFlags []string
	var globalOptions []string
	var indexesToRemove []int

	// For commands with only the subcommand (e.g., `atmos terraform plan`),
	// just set the subcommand and return - don't auto-show help.
	// Interactive prompts will handle missing args if available,
	// otherwise validation will show appropriate errors.
	// Exception: Don't return early if the single argument contains a space,
	// as it might be a quoted compound subcommand like "providers lock" that needs
	// further processing.
	if len(inputArgsAndFlags) == 1 && info.SubCommand == "" && !strings.Contains(inputArgsAndFlags[0], " ") {
		info.SubCommand = inputArgsAndFlags[0]
		return info, nil
	}

	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int

	for i, arg := range inputArgsAndFlags {
		// GlobalOptions: track its position for second-pass collection.
		// Note: the old code used strings.HasPrefix(arg+"=", cfg.GlobalOptionsFlag), which had the
		// same false-positive bug as the string flags; the corrected form checks arg starts with
		// flag+"=" to avoid matching flags that share a common prefix.
		if arg == cfg.GlobalOptionsFlag {
			globalOptionsFlagIndex = i + 1
		} else if strings.HasPrefix(arg, cfg.GlobalOptionsFlag+"=") {
			globalOptionsFlagIndex = i
		}

		// Standard string-valued flags: table-driven to eliminate repetitive if/else chains.
		for _, def := range stringFlagDefs {
			val, found, err := parseFlagValue(def.flag, arg, inputArgsAndFlags, i)
			if err != nil {
				return info, err
			}
			if found {
				def.setFunc(&info, val)
				break
			}
		}

		// --identity has special optional/empty-value semantics.
		parseIdentityFlag(&info, arg, inputArgsAndFlags, i)

		// --from-plan has optional value semantics (path may or may not follow).
		parseFromPlanFlag(&info, arg, inputArgsAndFlags, i)

		// Boolean flags — set fields on ArgsAndFlagsInfo when recognized.
		// Note: cfg.ProcessTemplatesFlag and cfg.ProcessFunctionsFlag are intentionally absent here.
		// Those flags are consumed exclusively by Cobra (via viper.GetBool) in the cmd/terraform/*
		// layer and assigned to configAndStacksInfo via cmd/terraform/utils.go.  They are listed in
		// commonFlags solely so they get stripped from pass-through args that reach the underlying
		// tool (terraform/tofu/helmfile).  No ArgsAndFlagsInfo field exists for them.
		switch arg {
		case cfg.DryRunFlag:
			info.DryRun = true
		case cfg.SkipInitFlag:
			info.SkipInit = true
		case cfg.HelpFlag1, cfg.HelpFlag2:
			info.NeedHelp = true
		case cfg.AffectedFlag:
			info.Affected = true
		case cfg.AllFlag:
			info.All = true
		}

		// Collect indices of atmos-specific flags to strip from pass-through args.
		for _, f := range commonFlags {
			if arg == f {
				indexesToRemove = append(indexesToRemove, i)
				// Optional-value flags (--from-plan, --identity): only strip i+1 when the next
				// arg was actually consumed as the value (i.e., it exists and does not start with '-').
				if f == cfg.FromPlanFlag || f == cfg.IdentityFlag {
					if len(inputArgsAndFlags) > i+1 && !strings.HasPrefix(inputArgsAndFlags[i+1], "-") {
						indexesToRemove = append(indexesToRemove, i+1)
					}
				} else if valueTakingCommonFlags[f] {
					// Value-taking flags always strip i+1 (the value was consumed during parsing).
					indexesToRemove = append(indexesToRemove, i+1)
				}
				// Boolean-only flags are not in valueTakingCommonFlags and do not strip i+1.
			} else if strings.HasPrefix(arg, f+"=") {
				indexesToRemove = append(indexesToRemove, i)
			}
		}
	}

	for i, arg := range inputArgsAndFlags {
		if !u.SliceContainsInt(indexesToRemove, i) {
			additionalArgsAndFlags = append(additionalArgsAndFlags, arg)
		}

		if globalOptionsFlagIndex > 0 && i == globalOptionsFlagIndex {
			if strings.HasPrefix(arg, cfg.GlobalOptionsFlag+"=") {
				parts := strings.SplitN(arg, "=", 2)
				globalOptions = strings.Split(parts[1], " ")
			} else {
				globalOptions = strings.Split(arg, " ")
			}
		}
	}

	info.GlobalOptions = globalOptions

	if info.NeedHelp {
		if len(additionalArgsAndFlags) > 0 {
			info.SubCommand = additionalArgsAndFlags[0]
		}
		return info, nil
	}

	if len(additionalArgsAndFlags) == 1 && info.SubCommand == "" {
		info.SubCommand = additionalArgsAndFlags[0]
	}

	if len(additionalArgsAndFlags) == 0 {
		return info, nil
	}

	// Handle terraform compound subcommands (e.g., "providers lock", "state list", "workspace select").
	// These are terraform commands that have their own subcommands.
	// https://developer.hashicorp.com/terraform/cli/commands
	if componentType == "terraform" {
		processed, err := processTerraformCompoundSubcommand(&info, additionalArgsAndFlags)
		if err != nil {
			return info, err
		}
		if processed {
			return info, nil
		}
	}

	// Not a compound subcommand, process as single command.
	if err := processSingleCommand(&info, additionalArgsAndFlags); err != nil {
		return info, err
	}

	return info, nil
}

// getCliVars parses command-line arguments and extracts all -var arguments,
// returning them as a map of variables with proper type conversion.
// This function processes JSON values and returns them as parsed objects.
// Example: ["-var", "name=test", "-var", "tags={\"env\":\"prod\",\"team\":\"devops\"}"]
// Returns: map[string]any{"name": "test", "tags": map[string]any{"env": "prod", "team": "devops"}}.
func getCliVars(args []string) (map[string]any, error) {
	variables := make(map[string]any)
	for i := 0; i < len(args); i++ {
		if args[i] == "-var" && i+1 < len(args) {
			kv := args[i+1]
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				varName := parts[0]
				part2 := parts[1]
				var varValue any
				if filetype.IsJSON(part2) {
					v, err := u.ConvertFromJSON(part2)
					if err != nil {
						return nil, err
					}
					varValue = v
				} else {
					varValue = strings.TrimSpace(part2)
				}

				variables[varName] = varValue
			}
			i++
		}
	}
	return variables, nil
}
