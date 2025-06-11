package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:                "dependents",
	Aliases:            []string{"dependants"},
	Short:              "List Atmos components that depend on a given component",
	Long:               "This command generates a list of Atmos components within stacks that depend on the specified Atmos component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.ExactArgs(1),
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		var err error
		info, err := exec.ProcessCommandLineArgs("", cmd, args, nil)
		checkErrorAndExit(err)
		info.CliArgs = []string{"describe", "dependents"}
		atmosConfig, err := cfg.InitCliConfig(info, true)
		checkErrorAndExit(err)
		err = exec.ValidateStacks(atmosConfig)
		checkErrorAndExit(err)

		describe := &exec.DescribeDependentsExecProps{}
		if err := setFlagsForDescribeDependentsCmd(cmd.Flags(), describe); err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}

		if cmd.Flags().Changed("pager") {
			// TODO: update this post pr:https://github.com/cloudposse/atmos/pull/1174 is merged
			atmosConfig.Settings.Terminal.Pager, err = cmd.Flags().GetString("pager")
		}
		describe.Component = args[0]
		checkErrorAndExit(err)
		err = exec.NewDescribeDependentsExec(&atmosConfig).Execute(describe)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func setFlagsForDescribeDependentsCmd(flags *pflag.FlagSet, describe *exec.DescribeDependentsExecProps) error {
	if err := setStringFlagIfChanged(flags, "format", &describe.Format); err != nil {
		return err
	}
	if err := setStringFlagIfChanged(flags, "file", &describe.File); err != nil {
		return err
	}
	if err := setStringFlagIfChanged(flags, "stack", &describe.Stack); err != nil {
		return err
	}
	if err := setStringFlagIfChanged(flags, "query", &describe.Query); err != nil {
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
