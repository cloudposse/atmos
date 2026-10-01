package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

// sourceDryRun resolves an optional flag supplied by the parent command, including
// its declared environment variable. Command groups without the flag opt out.
func sourceDryRun(cmd *cobra.Command) bool {
	flag := cmd.Flag("dry-run")
	if flag == nil || flag.Value.Type() != "bool" {
		return false
	}
	_, dryRun := flags.NewStandardParser(flags.WithDryRunFlag()).IsBoolFlagExplicitlySet(cmd, "dry-run")
	return dryRun
}
