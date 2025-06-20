package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Display configuration for Atmos stacks and their components",
	Long:               "This command shows the configuration details for Atmos stacks and the components within those stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		checkAtmosConfig,
		exec.ProcessCommandLineArgs,
		cfg.InitCliConfig, exec.ValidateStacks,
		setCliArgsForDescribeStackCli,
		exec.NewDescribeStacksExec(),
	}),
}

type getRunnableDescribeStacksCmdProps struct {
	checkAtmosConfig       func(opts ...AtmosValidateOption)
	processCommandLineArgs func(
		componentType string,
		cmd *cobra.Command,
		args []string,
		additionalArgsAndFlags []string,
	) (schema.ConfigAndStacksInfo, error)
	initCliConfig                 func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	validateStacks                func(atmosConfig schema.AtmosConfiguration) error
	setCliArgsForDescribeStackCli func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error
	newDescribeStacksExec         exec.DescribeStacksExec
}

func getRunnableDescribeStacksCmd(
	g getRunnableDescribeStacksCmdProps,
) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		g.checkAtmosConfig()
		info, err := g.processCommandLineArgs("", cmd, args, nil)
		printErrorAndExit(err)
		atmosConfig, err := g.initCliConfig(info, true)
		printErrorAndExit(err)
		err = g.validateStacks(atmosConfig)
		printErrorAndExit(err)
		describe := &exec.DescribeStacksArgs{}
		err = setCliArgsForDescribeStackCli(cmd.Flags(), describe)
		printErrorAndExit(err)
		if cmd.Flags().Changed("pager") {
			// TODO: update this post pr:https://github.com/cloudposse/atmos/pull/1174 is merged
			atmosConfig.Settings.Terminal.Pager, err = cmd.Flags().GetString("pager")
		}

		printErrorAndExit(err)
		err = g.newDescribeStacksExec.Execute(&atmosConfig, describe)
		printErrorAndExit(err)
	}
}

func printErrorAndExit(err error) {
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}
}

func setCliArgsForDescribeStackCli(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
	flagsKeyValue := map[string]any{
		"stack":                &describe.FilterByStack,
		"format":               &describe.Format,
		"file":                 &describe.File,
		"include-empty-stacks": &describe.IncludeEmptyStacks,
		"components":           &describe.Components,
		"component-types":      &describe.ComponentTypes,
		"sections":             &describe.Sections,
		"process-templates":    &describe.ProcessTemplates,
		"process-functions":    &describe.ProcessYamlFunctions,
		"query":                &describe.Query,
		"skip":                 &describe.Skip,
	}

	var err error
	for k := range flagsKeyValue {
		if !flags.Changed(k) {
			continue
		}
		switch v := flagsKeyValue[k].(type) {
		case *string:
			*v, err = flags.GetString(k)
		case *bool:
			*v, err = flags.GetBool(k)
		case *[]string:
			*v, err = flags.GetStringSlice(k)
		default:
			panic(fmt.Sprintf("unsupported type %T for flag %s", v, k))
		}
		checkFlagError(err)
	}
	return validateFormat(describe)
}

func validateFormat(describe *exec.DescribeStacksArgs) error {
	format := describe.Format
	if format != "" && format != "yaml" && format != "json" {
		return exec.ErrInvalidFormat
	}
	if format == "" {
		format = "yaml"
	}
	describe.Format = format
	return nil
}

func init() {
	describeStacksCmd.DisableFlagParsing = false

	describeStacksCmd.PersistentFlags().String("file", "", "Write the result to file")

	describeStacksCmd.PersistentFlags().String("format", "yaml", "Specify the output format (`yaml` is default)")

	describeStacksCmd.PersistentFlags().StringP("stack", "s", "",
		"Filter by a specific stack\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)",
	)
	AddStackCompletion(describeStacksCmd)
	describeStacksCmd.PersistentFlags().String("components", "", "Filter by specific `atmos` components")

	describeStacksCmd.PersistentFlags().String("component-types", "", "Filter by specific component types. Supported component types: terraform, helmfile")

	describeStacksCmd.PersistentFlags().StringSlice("sections", nil, "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")

	describeStacksCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("include-empty-stacks", false, "Include stacks with no components in the output")

	describeStacksCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")

	describeCmd.AddCommand(describeStacksCmd)
}
