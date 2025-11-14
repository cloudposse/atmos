package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
)

var vendorParser *flags.StandardParser

// VendorOptions contains parsed flags for the vendor command.
type VendorOptions struct {
	global.Flags
	Format    string
	Stack     string
	Delimiter string
}

// vendorCmd lists vendor configurations.
var vendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "List all vendor configurations",
	Long:  "List all vendor configurations in a tabular way, including component and vendor manifests.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkAtmosConfig(true); err != nil {
			return err
		} // Skip stack validation for vendor

		// Parse flags using StandardParser with Viper precedence
		v := viper.GetViper()
		if err := vendorParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &VendorOptions{
			Flags:     flags.ParseGlobalFlags(cmd, v),
			Format:    v.GetString("format"),
			Stack:     v.GetString("stack"),
			Delimiter: v.GetString("delimiter"),
		}

		output, err := listVendorWithOptions(opts)
		if err != nil {
			return err
		}

		fmt.Println(output)
		return nil
	},
}

func init() {
	// Create parser with vendor-specific flags using functional options
	vendorParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithStringFlag("stack", "s", "", "Filter by stack name or pattern"),
		flags.WithStringFlag("delimiter", "d", "", "Delimiter for CSV/TSV output"),
		flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("delimiter", "ATMOS_LIST_DELIMITER"),
	)

	// Register flags
	vendorParser.RegisterFlags(vendorCmd)

	// Add stack completion
	addStackCompletion(vendorCmd)

	// Bind flags to Viper for environment variable support
	if err := vendorParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listVendorWithOptions(opts *VendorOptions) (string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return "", err
	}

	options := &l.FilterOptions{
		FormatStr:    opts.Format,
		StackPattern: opts.Stack,
		Delimiter:    opts.Delimiter,
	}

	return l.FilterAndListVendor(&atmosConfig, options)
}
