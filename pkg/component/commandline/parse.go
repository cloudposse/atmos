package commandline

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Parse processes command-line args.
func Parse(request Request) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "commandline.Parse")()

	var info schema.ConfigAndStacksInfo
	log.Debug("ProcessCommandLineArgs input", "componentType", request.ComponentType, "args", request.Args)
	cmd := request.Command
	cmd.DisableFlagParsing = false
	beforeValue, beforeSet := getExplicitIdentityFlagValue(cmd)
	if err := cmd.ParseFlags(request.Args); err != nil && !errors.Is(err, pflag.ErrHelp) {
		return info, err
	}
	// Check what Cobra parsed for identity flag.
	if flag := cmd.Flag("identity"); flag != nil {
		log.Debug("After ParseFlags", "identity.Value", flag.Value.String(), "identity.Changed", flag.Changed)
	}
	parsed, err := ParseArguments(request.ComponentType, request.Args)
	if err != nil {
		return info, err
	}
	if err = readConfigurationFlags(cmd, &info); err != nil {
		return info, err
	}
	copyCommandFields(&info, &parsed, request.ComponentType)
	copyProcessingFields(&info, &parsed)
	if len(request.AdditionalArgsAndFlags) > 0 {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, request.AdditionalArgsAndFlags...)
	}
	afterValue, afterSet := getExplicitIdentityFlagValue(cmd)
	info.Identity = resolveIdentityValue(parsed.Identity, beforeValue, beforeSet, afterValue, afterSet)
	applyParsedFlagFallbacks(cmd, &info)
	return info, nil
}

func readConfigurationFlags(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) error {
	flags := cmd.Flags()
	var err error
	info.BasePath, err = flags.GetString("base-path")
	if err != nil {
		return err
	}
	info.AtmosConfigFilesFromArg, err = flags.GetStringSlice("config")
	if err != nil {
		return err
	}
	info.AtmosConfigDirsFromArg, err = flags.GetStringSlice("config-path")
	if err != nil {
		return err
	}
	info.ProfilesFromArg, err = readProfiles(flags)
	return err
}

func readProfiles(flags *pflag.FlagSet) ([]string, error) {
	// Check the profile flag first, then fall back to ATMOS_PROFILE.
	profiles, err := flags.GetStringSlice("profile")
	if err != nil || len(profiles) > 0 {
		return profiles, err
	}
	//nolint:forbidigo // Must use os.Getenv: profile is processed before Viper configuration loads.
	envProfiles := os.Getenv("ATMOS_PROFILE")
	// Split comma-separated profiles from env var and filter out empty entries.
	for _, profile := range strings.Split(envProfiles, ",") {
		if value := strings.TrimSpace(profile); value != "" {
			profiles = append(profiles, value)
		}
	}
	return profiles, nil
}

func copyCommandFields(info *schema.ConfigAndStacksInfo, parsed *schema.ArgsAndFlagsInfo, componentType string) {
	info.AdditionalArgsAndFlags = parsed.AdditionalArgsAndFlags
	info.SubCommand = parsed.SubCommand
	info.SubCommand2 = parsed.SubCommand2
	info.ComponentType = componentType
	info.ComponentFromArg = parsed.ComponentFromArg
	info.GlobalOptions = parsed.GlobalOptions
	info.TerraformCommand = parsed.TerraformCommand
	info.TerraformDir = parsed.TerraformDir
	info.HelmfileCommand = parsed.HelmfileCommand
	info.HelmfileDir = parsed.HelmfileDir
	info.StacksDir = parsed.StacksDir
	info.ConfigDir = parsed.ConfigDir
	info.WorkflowsDir = parsed.WorkflowsDir
	info.DeployRunInit = parsed.DeployRunInit
	info.InitRunReconfigure = parsed.InitRunReconfigure
	info.InitPassVars = parsed.InitPassVars
}

func copyProcessingFields(info *schema.ConfigAndStacksInfo, parsed *schema.ArgsAndFlagsInfo) {
	info.PlanSkipPlanfile = parsed.PlanSkipPlanfile
	info.AutoGenerateBackendFile = parsed.AutoGenerateBackendFile
	info.UseTerraformPlan = parsed.UseTerraformPlan
	info.PlanFile = parsed.PlanFile
	info.DryRun = parsed.DryRun
	info.SkipInit = parsed.SkipInit
	info.NeedHelp = parsed.NeedHelp
	info.JsonSchemaDir = parsed.JsonSchemaDir
	info.AtmosManifestJsonSchema = parsed.AtmosManifestJsonSchema
	info.OpaDir = parsed.OpaDir
	info.CueDir = parsed.CueDir
	info.RedirectStdErr = parsed.RedirectStdErr
	info.LogsLevel = parsed.LogsLevel
	info.LogsFile = parsed.LogsFile
	info.LogsColor = parsed.LogsColor
	info.SettingsListMergeStrategy = parsed.SettingsListMergeStrategy
	info.Query = parsed.Query
	info.ClusterName = parsed.ClusterName
	info.NeedsPathResolution = parsed.NeedsPathResolution
	info.Affected = parsed.Affected
	info.All = parsed.All
	info.PackerDir = parsed.PackerDir
	info.PackerCommand = parsed.PackerCommand
}

func applyParsedFlagFallbacks(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) {
	// Cobra strips flags from the args passed to RunE. Recover a directly parsed
	// merge strategy when the raw-argument scan did not see it.
	if info.SettingsListMergeStrategy == "" {
		if flag := cmd.Flag("settings-list-merge-strategy"); flag != nil && flag.Changed {
			info.SettingsListMergeStrategy = flag.Value.String()
		}
	}
	applyIdentityEnvironment(info)

	flags := cmd.Flags()
	if stack, err := flags.GetString("stack"); err == nil && stack != "" {
		info.Stack = stack
	}
	// PreRunE can receive args with flags already stripped. Recover Cobra's
	// dry-run value just like the stack value, without overriding a true value.
	if !info.DryRun {
		if dryRun, err := flags.GetBool("dry-run"); err == nil && dryRun {
			info.DryRun = true
		}
	}
}

func applyIdentityEnvironment(info *schema.ConfigAndStacksInfo) {
	// Fallback to ATMOS_IDENTITY only when no flag selected an identity. Read the
	// environment directly to avoid polluting Viper with a temporary binding.
	// Normalize false/0/no/off consistently with explicitly passed identities.
	if info.Identity == "" {
		if envIdentity := os.Getenv("ATMOS_IDENTITY"); envIdentity != "" { //nolint:forbidigo // Direct env var read to avoid viper config pollution.
			info.Identity = cfg.NormalizeIdentityValue(envIdentity)
		}
	}
}
