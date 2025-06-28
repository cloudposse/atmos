package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	atmoserr "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// describeComponentCmd describes configuration for components
var describeComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Show configuration details for an Atmos component in a stack",
	Long:               `Display the configuration details for a specific Atmos component within a designated Atmos stack, including its dependencies, settings, and overrides.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		if len(args) != 1 {
			return errors.New("invalid arguments. The command requires one argument `component`")
		}

		flags := cmd.Flags()

		stack, err := flags.GetString("stack")
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		format, err := flags.GetString("format")
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		file, err := flags.GetString("file")
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		processTemplates, err := flags.GetBool("process-templates")
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		processYamlFunctions, err := flags.GetBool("process-functions")
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		query, err := flags.GetString("query")
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		skip, err := flags.GetStringSlice("skip")
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		pager, err := flags.GetString("pager")
		atmoserr.CheckErrorPrintAndExit(err, "", "")

		component := args[0]

		err = e.NewDescribeComponentExec().ExecuteDescribeComponentCmd(e.DescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     processTemplates,
			ProcessYamlFunctions: processYamlFunctions,
			Skip:                 skip,
			Query:                query,
			Pager:                pager,
			Format:               format,
			File:                 file,
		})
		atmoserr.CheckErrorPrintAndExit(err, "", "")
		return nil
	},
	ValidArgsFunction: ComponentsArgCompletion,
}

func init() {
	describeComponentCmd.DisableFlagParsing = false
	AddStackCompletion(describeComponentCmd)
	describeComponentCmd.PersistentFlags().StringP("format", "f", "yaml", "The output format")
	describeComponentCmd.PersistentFlags().String("file", "", "Write the result to the file")
	describeComponentCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")

	err := describeComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		atmoserr.CheckErrorPrintAndExit(err, "", "")
	}

	describeCmd.AddCommand(describeComponentCmd)
}
