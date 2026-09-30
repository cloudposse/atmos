package cmd

import "github.com/spf13/cobra"

// sourceDryRun reads an optional flag supplied by the parent command. Avoid
// global Viper state: other source command groups may not expose dry-run at all.
func sourceDryRun(cmd *cobra.Command) bool {
	flag := cmd.Flag("dry-run")
	return flag != nil && flag.Value.Type() == "bool" && flag.Value.String() == "true"
}
