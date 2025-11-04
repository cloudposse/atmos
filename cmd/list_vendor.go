package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
)

var listVendorParser = flags.NewStandardOptionsBuilder().
	WithStack(false).
	WithFormat([]string{"table", "json", "yaml", "csv", "tsv"}, "table").
	WithDelimiter(",").
	Build()

// listVendorCmd lists vendor configurations.
var listVendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "List all vendor configurations",
	Long:  "List all vendor configurations in a tabular way, including component and vendor manifests.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		checkAtmosConfig(WithStackValidation(false))

		// Parse flags using StandardOptions.
		opts, err := listVendorParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// Initialize CLI config.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			return err
		}

		// Set options.
		options := &l.FilterOptions{
			FormatStr:    opts.Format,
			StackPattern: opts.Stack,
			Delimiter:    opts.Delimiter,
		}

		// Call list vendor function.
		output, err := l.FilterAndListVendor(&atmosConfig, options)
		if err != nil {
			return err
		}

		// Print output.
		fmt.Println(output)

		return nil
	},
}

func init() {
	// Register StandardOptions flags.
	listVendorParser.RegisterFlags(listVendorCmd)
	_ = listVendorParser.BindToViper(viper.GetViper())

	// Add stack completion.
	_ = listVendorCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)

	listCmd.AddCommand(listVendorCmd)
}
