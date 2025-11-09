package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// profileCmd groups profile-related subcommands.
var profileCmd = &cobra.Command{
	Use:                "profile",
	Short:              "Manage configuration profiles",
	Long:               "Discover, inspect, and manage configuration profiles. Profiles provide environment-specific configuration overrides for development, CI/CD, and production contexts without duplicating settings.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
}

func init() {
	defer perf.Track(nil, "cmd.init.profileCmd")()

	RootCmd.AddCommand(profileCmd)
}
