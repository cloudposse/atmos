package gcp

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/gcp/gke"
	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

//go:embed markdown/atmos_gcp_usage.md
var gcpUsageMarkdown string

var gcpCmd = &cobra.Command{
	Use:     "gcp",
	Short:   "Run GCP-specific commands for interacting with cloud resources",
	Long:    "This command allows interaction with Google Cloud resources through native Atmos commands.",
	Example: gcpUsageMarkdown,
	Args:    cobra.NoArgs,
}

// init registers the GCP command hierarchy with the root command.
func init() {
	gcpCmd.AddCommand(gke.GkeCmd)
	internal.Register(&GCPCommandProvider{})
}

// GCPCommandProvider registers the gcp command.
type GCPCommandProvider struct{}

// GetCommand returns the root GCP command.
func (*GCPCommandProvider) GetCommand() *cobra.Command { return gcpCmd }

// GetName returns the command name.
func (*GCPCommandProvider) GetName() string { return "gcp" }

// GetGroup returns the command group.
func (*GCPCommandProvider) GetGroup() string { return "Cloud Integration" }

// GetAliases reports that the command has no aliases.
func (*GCPCommandProvider) GetAliases() []internal.CommandAlias { return nil }

// GetFlagsBuilder reports that the command has no shared flag builder.
func (*GCPCommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder reports that the command has no positional arguments.
func (*GCPCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

// GetCompatibilityFlags reports that the command has no compatibility flags.
func (*GCPCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }

// IsExperimental reports that the command is stable.
func (*GCPCommandProvider) IsExperimental() bool { return false }
