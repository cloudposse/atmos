package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

//go:embed markdown/atmos_list_profiles_usage.md
var listProfilesUsageMarkdown string

// listProfilesCmd is an alias for "atmos profile list".
var listProfilesCmd = &cobra.Command{
	Use:                "profiles",
	Short:              "List available configuration profiles",
	Long:               `List all configured profiles across all locations. This is an alias for "atmos profile list".`,
	Example:            listProfilesUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               executeProfileListCommand, // Reuse the profile list command.
}

func init() {
	defer perf.Track(nil, "cmd.init.listProfilesCmd")()

	// Format flag (same as profile list).
	listProfilesCmd.Flags().StringP("format", "f", "table", "Output format: table, json, yaml")

	// Register flag completion functions.
	if err := listProfilesCmd.RegisterFlagCompletionFunc("format", profileFormatFlagCompletion); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	listCmd.AddCommand(listProfilesCmd)
}
