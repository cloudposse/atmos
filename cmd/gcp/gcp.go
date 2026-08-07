package gcp

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/gcp/gke"
	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

var gcpCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Run GCP-specific commands for interacting with cloud resources",
	Long:  "This command allows interaction with Google Cloud resources through native Atmos commands.",
	Args:  cobra.NoArgs,
}

func init() {
	gcpCmd.AddCommand(gke.GkeCmd)
	internal.Register(&GCPCommandProvider{})
}

// GCPCommandProvider registers the gcp command.
type GCPCommandProvider struct{}

func (*GCPCommandProvider) GetCommand() *cobra.Command                                 { return gcpCmd }
func (*GCPCommandProvider) GetName() string                                            { return "gcp" }
func (*GCPCommandProvider) GetGroup() string                                           { return "Cloud Integration" }
func (*GCPCommandProvider) GetAliases() []internal.CommandAlias                        { return nil }
func (*GCPCommandProvider) GetFlagsBuilder() flags.Builder                             { return nil }
func (*GCPCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder     { return nil }
func (*GCPCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }
func (*GCPCommandProvider) IsExperimental() bool                                       { return false }
