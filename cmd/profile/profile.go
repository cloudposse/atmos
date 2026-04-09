package profile

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
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
	defer perf.Track(nil, "profile.init")()

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&ProfileCommandProvider{})
}

// ProfileCommandProvider implements the CommandProvider interface.
type ProfileCommandProvider struct{}

// GetCommand returns the profile command.
func (p *ProfileCommandProvider) GetCommand() *cobra.Command {
	return profileCmd
}

// GetName returns the command name.
func (p *ProfileCommandProvider) GetName() string {
	return "profile"
}

// GetGroup returns the command group for help organization.
func (p *ProfileCommandProvider) GetGroup() string {
	return "Configuration Management"
}

// GetFlagsBuilder returns the flags builder for this command.
func (p *ProfileCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (p *ProfileCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (p *ProfileCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
// Creates "atmos list profiles" as an alias for "atmos profile list".
func (p *ProfileCommandProvider) GetAliases() []internal.CommandAlias {
	return []internal.CommandAlias{
		{
			Subcommand:    "list",
			ParentCommand: "list",
			Name:          "profiles",
			Short:         "List available configuration profiles",
			Long:          `List all configured profiles across all locations. This is an alias for "atmos profile list".`,
			Example: `# List all available profiles
atmos list profiles

# List profiles in JSON format
atmos list profiles --format json

# List profiles in YAML format
atmos list profiles --format yaml`,
		},
	}
}

// IsExperimental returns whether this command is experimental.
func (p *ProfileCommandProvider) IsExperimental() bool {
	return false
}
