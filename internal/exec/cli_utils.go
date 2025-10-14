package exec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Error format constants.
const (
	errFlagFormat = "%w: flag: %s"
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
	cfg.ProcessTemplatesFlag,
	cfg.ProcessFunctionsFlag,
	cfg.SkipFlag,
	cfg.AffectedFlag,
	cfg.AllFlag,
	cfg.InitPassVars,
	cfg.PlanSkipPlanfile,
	cfg.IdentityFlag,
	cfg.ProfilerEnabledFlag,
	cfg.ProfilerHostFlag,
	cfg.ProfilerPortFlag,
	cfg.ProfilerFileFlag,
	cfg.ProfilerTypeFlag,
	cfg.HeatmapFlag,
	cfg.HeatmapModeFlag,
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

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil && !errors.Is(err, pflag.ErrHelp) {
		return configAndStacksInfo, err
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

	if len(inputArgsAndFlags) == 1 && inputArgsAndFlags[0] == "clean" {
		info.SubCommand = inputArgsAndFlags[0]
	}

	// For commands like `atmos terraform plan`, show the command help
	if len(inputArgsAndFlags) == 1 && inputArgsAndFlags[0] != "version" && info.SubCommand == "" {
		info.SubCommand = inputArgsAndFlags[0]
		info.NeedHelp = true
		return info, nil
	}

	if len(inputArgsAndFlags) == 1 && inputArgsAndFlags[0] == "version" {
		info.SubCommand = inputArgsAndFlags[0]
		return info, nil
	}

	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int

	for i, arg := range inputArgsAndFlags {
		if arg == cfg.GlobalOptionsFlag {
			globalOptionsFlagIndex = i + 1
		} else if strings.HasPrefix(arg+"=", cfg.GlobalOptionsFlag) {
			globalOptionsFlagIndex = i
		}

		if arg == cfg.TerraformCommandFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.TerraformCommand = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.TerraformCommandFlag) {
			terraformCommandFlagParts := strings.Split(arg, "=")
			if len(terraformCommandFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.TerraformCommand = terraformCommandFlagParts[1]
		}

		if arg == cfg.TerraformDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.TerraformDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.TerraformDirFlag) {
			terraformDirFlagParts := strings.Split(arg, "=")
			if len(terraformDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.TerraformDir = terraformDirFlagParts[1]
		}

		if arg == cfg.AppendUserAgentFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.AppendUserAgent = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.AppendUserAgentFlag) {
			appendUserAgentFlagParts := strings.Split(arg, "=")
			if len(appendUserAgentFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.AppendUserAgent = appendUserAgentFlagParts[1]
		}

		if arg == cfg.HelmfileCommandFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.HelmfileCommand = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.HelmfileCommandFlag) {
			helmfileCommandFlagParts := strings.Split(arg, "=")
			if len(helmfileCommandFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.HelmfileCommand = helmfileCommandFlagParts[1]
		}

		if arg == cfg.HelmfileDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.HelmfileDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.HelmfileDirFlag) {
			helmfileDirFlagParts := strings.Split(arg, "=")
			if len(helmfileDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.HelmfileDir = helmfileDirFlagParts[1]
		}

		if arg == cfg.CliConfigDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.ConfigDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.CliConfigDirFlag) {
			configDirFlagParts := strings.Split(arg, "=")
			if len(configDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.ConfigDir = configDirFlagParts[1]
		}

		if arg == cfg.StackDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.StacksDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.StackDirFlag) {
			stacksDirFlagParts := strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.StacksDir = stacksDirFlagParts[1]
		}

		if arg == cfg.BasePathFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.BasePath = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.BasePathFlag) {
			stacksDirFlagParts := strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.BasePath = stacksDirFlagParts[1]
		}

		if arg == cfg.VendorBasePathFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.VendorBasePath = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.VendorBasePathFlag) {
			vendorBasePathFlagParts := strings.Split(arg, "=")
			if len(vendorBasePathFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.VendorBasePath = vendorBasePathFlagParts[1]
		}

		if arg == cfg.DeployRunInitFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.DeployRunInit = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.DeployRunInitFlag) {
			deployRunInitFlagParts := strings.Split(arg, "=")
			if len(deployRunInitFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.DeployRunInit = deployRunInitFlagParts[1]
		}

		if arg == cfg.AutoGenerateBackendFileFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.AutoGenerateBackendFile = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.AutoGenerateBackendFileFlag) {
			autoGenerateBackendFileFlagParts := strings.Split(arg, "=")
			if len(autoGenerateBackendFileFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.AutoGenerateBackendFile = autoGenerateBackendFileFlagParts[1]
		}

		if arg == cfg.WorkflowDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.WorkflowsDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.WorkflowDirFlag) {
			workflowDirFlagParts := strings.Split(arg, "=")
			if len(workflowDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.WorkflowsDir = workflowDirFlagParts[1]
		}

		if arg == cfg.InitRunReconfigure {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.InitRunReconfigure = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.InitRunReconfigure) {
			initRunReconfigureParts := strings.Split(arg, "=")
			if len(initRunReconfigureParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.InitRunReconfigure = initRunReconfigureParts[1]
		}

		if arg == cfg.InitPassVars {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.InitPassVars = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.InitPassVars) {
			initPassVarsParts := strings.Split(arg, "=")
			if len(initPassVarsParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.InitPassVars = initPassVarsParts[1]
		}

		if arg == cfg.PlanSkipPlanfile {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.PlanSkipPlanfile = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.PlanSkipPlanfile) {
			planSkipPlanfileParts := strings.Split(arg, "=")
			if len(planSkipPlanfileParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.PlanSkipPlanfile = planSkipPlanfileParts[1]
		}

		if arg == cfg.JsonSchemaDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.JsonSchemaDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.JsonSchemaDirFlag) {
			jsonschemaDirFlagParts := strings.Split(arg, "=")
			if len(jsonschemaDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.JsonSchemaDir = jsonschemaDirFlagParts[1]
		}

		if arg == cfg.OpaDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.OpaDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.OpaDirFlag) {
			opaDirFlagParts := strings.Split(arg, "=")
			if len(opaDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.OpaDir = opaDirFlagParts[1]
		}

		if arg == cfg.CueDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.CueDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.CueDirFlag) {
			cueDirFlagParts := strings.Split(arg, "=")
			if len(cueDirFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.CueDir = cueDirFlagParts[1]
		}

		if arg == cfg.AtmosManifestJsonSchemaFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.AtmosManifestJsonSchema = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.AtmosManifestJsonSchemaFlag) {
			atmosManifestJsonSchemaFlagParts := strings.Split(arg, "=")
			if len(atmosManifestJsonSchemaFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.AtmosManifestJsonSchema = atmosManifestJsonSchemaFlagParts[1]
		}

		if arg == cfg.RedirectStdErrFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.RedirectStdErr = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.RedirectStdErrFlag) {
			redirectStderrParts := strings.Split(arg, "=")
			if len(redirectStderrParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.RedirectStdErr = redirectStderrParts[1]
		}

		if arg == cfg.PlanFileFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.PlanFile = inputArgsAndFlags[i+1]
			info.UseTerraformPlan = true
		} else if strings.HasPrefix(arg+"=", cfg.PlanFileFlag) {
			planFileFlagParts := strings.Split(arg, "=")
			if len(planFileFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.PlanFile = planFileFlagParts[1]
			info.UseTerraformPlan = true
		}

		if arg == cfg.LogsLevelFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.LogsLevel = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.LogsLevelFlag) {
			logsLevelFlagParts := strings.Split(arg, "=")
			if len(logsLevelFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.LogsLevel = logsLevelFlagParts[1]
		}

		if arg == cfg.LogsFileFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.LogsFile = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.LogsFileFlag) {
			logsFileFlagParts := strings.Split(arg, "=")
			if len(logsFileFlagParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.LogsFile = logsFileFlagParts[1]
		}

		if arg == cfg.SettingsListMergeStrategyFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.SettingsListMergeStrategy = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.SettingsListMergeStrategyFlag) {
			settingsListMergeStrategyParts := strings.Split(arg, "=")
			if len(settingsListMergeStrategyParts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.SettingsListMergeStrategy = settingsListMergeStrategyParts[1]
		}

		if arg == cfg.QueryFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.Query = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.QueryFlag) {
			parts := strings.Split(arg, "=")
			if len(parts) != 2 {
				return info, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
			}
			info.Query = parts[1]
		}

		if arg == cfg.IdentityFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("%w: %s", errUtils.ErrInvalidFlag, arg)
			}
			info.Identity = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.IdentityFlag) {
			parts := strings.Split(arg, "=")
			if len(parts) != 2 {
				return info, fmt.Errorf("%w: %s", errUtils.ErrInvalidFlag, arg)
			}
			info.Identity = parts[1]
		}

		if arg == cfg.FromPlanFlag {
			info.UseTerraformPlan = true
		}

		if arg == cfg.DryRunFlag {
			info.DryRun = true
		}

		if arg == cfg.SkipInitFlag {
			info.SkipInit = true
		}

		if arg == cfg.HelpFlag1 || arg == cfg.HelpFlag2 {
			info.NeedHelp = true
		}

		if arg == cfg.AffectedFlag {
			info.Affected = true
		}

		if arg == cfg.AllFlag {
			info.All = true
		}

		for _, f := range commonFlags {
			if arg == f {
				indexesToRemove = append(indexesToRemove, i)
				indexesToRemove = append(indexesToRemove, i+1)
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

	if len(additionalArgsAndFlags) > 1 {
		twoWordsCommand := false

		// Handle terraform two-words commands
		// https://developer.hashicorp.com/terraform/cli/commands
		if componentType == "terraform" {
			// Handle the custom legacy command `terraform write varfile` (NOTE: use `terraform generate varfile` instead)
			if additionalArgsAndFlags[0] == "write" && additionalArgsAndFlags[1] == "varfile" {
				info.SubCommand = "write"
				info.SubCommand2 = "varfile"
				twoWordsCommand = true
			}

			// `terraform workspace` commands
			// https://developer.hashicorp.com/terraform/cli/commands/workspace
			if additionalArgsAndFlags[0] == "workspace" &&
				u.SliceContainsString([]string{"list", "select", "new", "delete", "show"}, additionalArgsAndFlags[1]) {
				info.SubCommand = "workspace"
				info.SubCommand2 = additionalArgsAndFlags[1]
				twoWordsCommand = true
			}

			// `terraform state` commands
			// https://developer.hashicorp.com/terraform/cli/commands/state
			if additionalArgsAndFlags[0] == "state" &&
				u.SliceContainsString([]string{"list", "mv", "pull", "push", "replace-provider", "rm", "show"}, additionalArgsAndFlags[1]) {
				info.SubCommand = fmt.Sprintf("state %s", additionalArgsAndFlags[1])
				twoWordsCommand = true
			}

			// `terraform providers` commands
			// https://developer.hashicorp.com/terraform/cli/commands/providers
			if additionalArgsAndFlags[0] == "providers" &&
				u.SliceContainsString([]string{"lock", "mirror", "schema"}, additionalArgsAndFlags[1]) {
				info.SubCommand = fmt.Sprintf("providers %s", additionalArgsAndFlags[1])
				twoWordsCommand = true
			}
		}

		if twoWordsCommand {
			if len(additionalArgsAndFlags) > 2 {
				info.ComponentFromArg = additionalArgsAndFlags[2]
			} else {
				return info, fmt.Errorf("command \"%s\" requires an argument", info.SubCommand)
			}
			if len(additionalArgsAndFlags) > 3 {
				info.AdditionalArgsAndFlags = additionalArgsAndFlags[3:]
			}
		} else {
			info.SubCommand = additionalArgsAndFlags[0]
			if len(additionalArgsAndFlags) > 1 {
				secondArg := additionalArgsAndFlags[1]
				if len(secondArg) == 0 {
					return info, fmt.Errorf("invalid empty argument provided")
				}
				if strings.HasPrefix(secondArg, "--") {
					if len(secondArg) <= 2 {
						return info, fmt.Errorf("invalid option format: %s", secondArg)
					}
					info.AdditionalArgsAndFlags = []string{secondArg}
				} else {
					info.ComponentFromArg = secondArg
				}
			}
			if len(additionalArgsAndFlags) > 2 {
				info.AdditionalArgsAndFlags = additionalArgsAndFlags[2:]
			}
		}
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
