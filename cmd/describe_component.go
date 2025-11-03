package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

// describeComponentParser is created once at package initialization using builder pattern.
var describeComponentParser = flags.NewStandardOptionsBuilder().
	WithStack(true).            // Required stack flag.
	WithFormat("yaml").         // Format flag with default.
	WithFile().                 // File output flag.
	WithProcessTemplates(true). // Process templates (default true).
	WithProcessFunctions(true). // Process functions (default true).
	WithSkip().                 // Skip flag.
	WithQuery().                // Query flag.
	WithProvenance().           // Provenance flag.
	Build()

// describeComponentCmd describes configuration for components.
var describeComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Show configuration details for an Atmos component in a stack",
	Long:               `Display the configuration details for a specific Atmos component within a designated Atmos stack, including its dependencies, settings, and overrides.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		// Parse flags with Viper precedence (CLI > ENV > config > defaults).
		v := viper.New()
		_ = describeComponentParser.BindFlagsToViper(cmd, v)

		// Parse command-line arguments and get strongly-typed options.
		opts, err := describeComponentParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// Extract component from positional arguments.
		positionalArgs := opts.GetPositionalArgs()
		if len(positionalArgs) == 0 {
			return cobra.ExactArgs(1)(cmd, args)
		}
		component := positionalArgs[0]

		// Execute command with strongly-typed parameters.
		err = e.NewDescribeComponentExec().ExecuteDescribeComponentCmd(e.DescribeComponentParams{
			Component:            component,
			Stack:                opts.Stack,
			ProcessTemplates:     opts.ProcessTemplates,
			ProcessYamlFunctions: opts.ProcessYamlFunctions,
			Skip:                 opts.Skip,
			Query:                opts.Query,
			Format:               opts.Format,
			File:                 opts.File,
			Provenance:           opts.Provenance,
		})
		return err
	},
	ValidArgsFunction: ComponentsArgCompletion,
}

func init() {
	describeComponentCmd.DisableFlagParsing = false
	AddStackCompletion(describeComponentCmd)
	describeComponentParser.RegisterFlags(describeComponentCmd)
	_ = describeComponentParser.BindToViper(viper.GetViper())

	describeCmd.AddCommand(describeComponentCmd)
}
