package cmd

import (
	"os"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeEditionCmd shows the active edition pin and the defaults it rolls back.
var describeEditionCmd = &cobra.Command{
	Use:   "edition",
	Short: "Display the active edition pin and the defaults it rolls back",
	Long: `This command shows whether the project is pinned to an edition (a date anchor
for defaults), where the pin came from (--edition flag, ATMOS_EDITION, or atmos.yaml),
the resolved anchor date, and every default the pin keeps at its pre-change value.

Without a pin, it reports that the project follows the latest defaults.
Use "atmos list editions" to see the journal of default changes.`,
	Example: "atmos describe edition\n" +
		"atmos describe edition --format=json\n" +
		"atmos --edition=2026-01 describe edition",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		pin, err := edition.DescribePin(atmosConfig.Edition, editionPinSource(cmd, atmosConfig.Edition))
		if err != nil {
			return err
		}

		if format == "json" {
			return data.WriteJSON(pin)
		}
		return data.WriteYAML(pin)
	},
}

// editionPinSource reports where the active pin came from, mirroring the
// precedence in pkg/config: --edition flag > ATMOS_EDITION env > atmos.yaml.
func editionPinSource(cmd *cobra.Command, pin string) string {
	if pin == "" {
		return ""
	}
	if cmd.Flags().Changed("edition") {
		return "flag"
	}
	if value, set := os.LookupEnv("ATMOS_EDITION"); set && value != "" {
		return "env"
	}
	return "config"
}

func init() {
	describeEditionCmd.DisableFlagParsing = false
	describeEditionCmd.PersistentFlags().StringP("format", "f", "yaml", "The output format: yaml or json")

	describeCmd.AddCommand(describeEditionCmd)
}
