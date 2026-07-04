package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackSetCmd = &cobra.Command{
	Use:   "set NAME",
	Short: "Update fields of a managed version entry in atmos.yaml",
	Long:  "Update fields of an existing managed version entry (desired version, pin policy, package, provider, group) in atmos.yaml, preserving comments and formatting.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.set.RunE")()

		fields := map[string]string{}
		for flag, path := range map[string]string{
			"desired":  "desired",
			"package":  "package",
			"provider": "provider",
			"group":    "group",
			"pin":      "update.pin",
		} {
			if cmd.Flags().Changed(flag) {
				value, _ := cmd.Flags().GetString(flag)
				fields[path] = value
			}
		}
		track := manager.EffectiveTrack(atmosConfig, trackFromArgs(cmd, nil))
		file, err := manager.SetEntryFields(atmosConfig, track, args[0], fields)
		if err != nil {
			return err
		}
		return writeFormatted(cmd, crudResult{Name: args[0], Track: track, File: file})
	},
}

func init() {
	parser := flags.NewStandardParser(trackParserOptions(
		flags.WithStringFlag("desired", "", "", "Desired version: concrete, SemVer constraint, or latest"),
		flags.WithStringFlag("package", "", "", "Package coordinate"),
		flags.WithStringFlag("provider", "", "", "Provider name from version.providers"),
		flags.WithStringFlag("group", "", "", "Version group name"),
		flags.WithStringFlag("pin", "", "", "Pin policy: digest (alias: sha), or none"),
	)...)
	parser.RegisterFlags(trackSetCmd)
	trackCmd.AddCommand(trackSetCmd)
}
