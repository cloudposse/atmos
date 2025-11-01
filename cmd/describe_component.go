package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
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
			return errUtils.Build(errUtils.ErrInvalidComponentArgument).
				WithHint("Provide exactly one argument: the component name").
				WithHint("Example: atmos describe component vpc -s prod").
				WithExitCode(2).
				Err()
		}

		flags := cmd.Flags()

		stack, err := flags.GetString("stack")
		if err != nil {
			return err
		}

		format, err := flags.GetString("format")
		if err != nil {
			return err
		}

		file, err := flags.GetString("file")
		if err != nil {
			return err
		}

		processTemplates, err := flags.GetBool("process-templates")
		if err != nil {
			return err
		}

		processYamlFunctions, err := flags.GetBool("process-functions")
		if err != nil {
			return err
		}

		query, err := flags.GetString("query")
		if err != nil {
			return err
		}

		skip, err := flags.GetStringSlice("skip")
		if err != nil {
			return err
		}

		provenance, err := flags.GetBool("provenance")
		if err != nil {
			return err
		}

		component := args[0]

		err = e.NewDescribeComponentExec().ExecuteDescribeComponentCmd(e.DescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     processTemplates,
			ProcessYamlFunctions: processYamlFunctions,
			Skip:                 skip,
			Query:                query,
			Format:               format,
			File:                 file,
			Provenance:           provenance,
		})
		return err
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
	describeComponentCmd.PersistentFlags().Bool("provenance", false, "Enable provenance tracking to show where configuration values originated")

	err := describeComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	describeCmd.AddCommand(describeComponentCmd)
}
