package workdir

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var describeParser *flags.StandardParser

var describeCmd = &cobra.Command{
	Use:   "describe <component>",
	Short: "Describe workdir as stack manifest",
	Long: `Output the workdir configuration as a valid Atmos stack manifest snippet.

The output can be used to see or copy the workdir configuration in a format
that matches the stack manifest structure.`,
	Example: `  atmos terraform workdir describe vpc --stack dev`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "workdir.describe.RunE")()

		component := args[0]

		v := viper.GetViper()
		stack := v.GetString("stack")

		if stack == "" {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithExplanation("Stack is required").
				WithHint("Use --stack to specify the stack").
				Err()
		}

		// Initialize config with global flags (--base-path, --config, etc.).
		configInfo := buildConfigAndStacksInfo(cmd, v)
		atmosConfig, err := cfg.InitCliConfig(configInfo, false)
		if err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to load atmos configuration").
				Err()
		}

		// Get workdir manifest.
		manifest, err := workdirManager.DescribeWorkdir(&atmosConfig, component, stack)
		if err != nil {
			return err
		}

		// Output the manifest.
		fmt.Print(manifest)
		return nil
	},
}

func init() {
	describeCmd.DisableFlagParsing = false

	// Create parser with functional options.
	describeParser = flags.NewStandardParser(
		flags.WithStackFlag(),
	)

	// Register flags with the command.
	describeParser.RegisterFlags(describeCmd)

	// Bind flags to Viper.
	if err := describeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
