package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/describe"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeComponentParser is created once at package initialization using builder pattern.
var describeComponentParser = flags.NewStandardOptionsBuilder().
	WithStack(true).                              // Required stack flag.
	WithFormat([]string{"json", "yaml"}, "yaml"). // Format flag with valid values and default.
	WithFile().                                   // File output flag.
	WithProcessTemplates(true).                   // Process templates (default true).
	WithProcessFunctions(true).                   // Process functions (default true).
	WithSkip().                                   // Skip flag.
	WithQuery().                                  // Query flag.
	WithProvenance().                             // Provenance flag.
	WithPositionalArgs(describe.NewComponentPositionalArgsBuilder().
		WithComponent(true). // Required component argument.
		Build()).
	Build()

// describeComponentCmd describes configuration for components.
var describeComponentCmd = &cobra.Command{
	Use:   "component <component>",
	Short: "Show configuration details for an Atmos component in a stack",
	Long:  `Display the configuration details for a specific Atmos component within a designated Atmos stack, including its dependencies, settings, and overrides.`,
	// Positional args are validated by the StandardParser using the builder pattern.
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		v := viper.New()
		_ = describeComponentParser.BindFlagsToViper(cmd, v)

		// Parse command-line arguments and get strongly-typed options.
		// Component is extracted by builder pattern into opts.Component field.
		opts, err := describeComponentParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}

		component := opts.Component

		// Load atmos configuration to get auth config.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
			ComponentFromArg: component,
			Stack:            opts.Stack,
		}, false)
		if err != nil {
			return errors.Join(errUtils.ErrFailedToInitConfig, err)
		}

		// Get identity from flag and create AuthManager if provided.
		identityName := GetIdentityFromFlags(cmd, os.Args)
		authManager, err := CreateAuthManagerFromIdentity(identityName, &atmosConfig.Auth)
		if err != nil {
			return err
		}

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
			AuthManager:          authManager,
		})
		return err
	},
	ValidArgsFunction: ComponentsArgCompletion,
}

func init() {
	AddStackCompletion(describeComponentCmd)
	describeComponentParser.RegisterFlags(describeComponentCmd)
	_ = describeComponentParser.BindToViper(viper.GetViper())

	describeCmd.AddCommand(describeComponentCmd)
}
