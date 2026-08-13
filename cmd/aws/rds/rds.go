package rds

import "github.com/spf13/cobra"

// RdsCmd is the parent command for AWS RDS IAM authentication subcommands.
var RdsCmd = &cobra.Command{
	Use:   "rds",
	Short: "Run AWS RDS commands for IAM database authentication",
	Long: `Generate short-lived RDS IAM database authentication tokens using Atmos identities.

These commands let you connect to Amazon RDS and Aurora databases with IAM authentication
instead of a static password, without requiring the AWS CLI.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}
