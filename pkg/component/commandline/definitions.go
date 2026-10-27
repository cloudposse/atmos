package commandline

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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
	"--explain",
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
	cfg.LogsColorFlag,
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
	cfg.IdentityFlagShort,
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

// stringFlagDef associates a CLI flag with a field setter on ArgsAndFlagsInfo.
// The table-driven approach in stringFlagDefs eliminates repetitive if/else chains.
type stringFlagDef struct {
	flag    string
	setFunc func(*schema.ArgsAndFlagsInfo, string)
}

// stringFlagDefs maps string-valued CLI flags to their ArgsAndFlagsInfo setters.
// ParseFlagValue handles both "--flag value" and "--flag=value" forms for each entry.
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
	// Note: cfg.IdentityFlagShort (shorthand for --identity) is intentionally NOT added here.
	// It is an optional-value flag and is handled alongside cfg.IdentityFlag in the
	// stripping branch below so that a trailing flag (e.g., "-i -lock=false") is not
	// erroneously consumed as the identity value. This matches parseIdentityFlag's
	// behavior, which only treats the next token as a value when it does not start with '-'.
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

// boolOnlyCommonFlags is the subset of commonFlags that are boolean. In space form they
// consume the next argument only when it is exactly "true" or "false" (case-insensitive).
var boolOnlyCommonFlags = map[string]bool{
	"--explain":              true,
	cfg.DryRunFlag:           true,
	cfg.SkipInitFlag:         true,
	cfg.LogsColorFlag:        true,
	cfg.AffectedFlag:         true,
	cfg.AllFlag:              true,
	cfg.ProcessTemplatesFlag: true,
	cfg.ProcessFunctionsFlag: true,
	cfg.ProfilerEnabledFlag:  true,
	cfg.HeatmapFlag:          true,
}
