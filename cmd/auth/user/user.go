package user

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthUserCmd groups user-related auth commands.
var AuthUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage cloud provider credentials in the local keychain",
	Long: `Store and manage user credentials in the local system keychain.
These credentials are used by Atmos to authenticate with cloud providers
(e.g. AWS IAM). Currently, only AWS IAM user credentials are supported.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	defer perf.Track(nil, "auth.user.init")()

	// Subcommands are added in their respective init() functions.
}
