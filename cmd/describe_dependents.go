package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type describeDependentExecCreator func(atmosConfig *schema.AtmosConfiguration) exec.DescribeDependentsExec

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:                "dependents",
	Aliases:            []string{"dependants"},
	Short:              "List Atmos components that depend on a given component",
	Long:               "This command generates a list of Atmos components within stacks that depend on the specified Atmos component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.ExactArgs(1),
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: getRunnableDescribeDependentsCmd(
		checkAtmosConfig,
		exec.ProcessCommandLineArgs,
		cfg.InitCliConfig,
		exec.NewDescribeDependentsExec),
}

func getRunnableDescribeDependentsCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	processCommandLineArgs func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error),
	initCliConfig func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error),
	newDescribeDependentsExec describeDependentExecCreator,
) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		info, err := processCommandLineArgs("terraform", cmd, args, nil)
		checkErrorAndExit(err)
		atmosConfig, err := initCliConfig(info, true)
		checkErrorAndExit(err)
		describe := &exec.DescribeDependentsExecProps{}
		err = setFlagsForDescribeDependentsCmd(cmd.Flags(), describe)
		checkErrorAndExit(err)
		if cmd.Flags().Changed("pager") {
			atmosConfig.Settings.Terminal.Pager, err = cmd.Flags().GetString("pager")
			checkErrorAndExit(err)
		}
		describe.Component = args[0]
		err = newDescribeDependentsExec(&atmosConfig).Execute(describe)
		checkErrorAndExit(err)
	}
}

func setFlagsForDescribeDependentsCmd(flags *pflag.FlagSet, describe *exec.DescribeDependentsExecProps) error {
	err := setStringFlagIfChanged(flags, "format", &describe.Format)
	if err != nil {
		return err
	}
	err = setStringFlagIfChanged(flags, "file", &describe.File)
	if err != nil {
		return err
	}
	err = setStringFlagIfChanged(flags, "stack", &describe.Stack)
	if err != nil {
		return err
	}
	err = setStringFlagIfChanged(flags, "query", &describe.Query)
	if err != nil {
		return err
	}
	if describe.Format == "" {
		describe.Format = "json"
	}
	if describe.Format != "json" && describe.Format != "yaml" {
		return ErrInvalidFormat
	}
	return nil
}

func init() {
	describeDependentsCmd.DisableFlagParsing = false

	AddStackCompletion(describeDependentsCmd)
	describeDependentsCmd.PersistentFlags().StringP("format", "f", "json", "The output format (`json` is default)")
	describeDependentsCmd.PersistentFlags().String("file", "", "Write the result to the file")
	describeDependentsCmd.PersistentFlags().String("query", "", "Filter the output using a JMESPath query")

	err := describeDependentsCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	describeCmd.AddCommand(describeDependentsCmd)
}
