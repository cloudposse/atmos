// Package git implements the `atmos git` command group, providing Git
// repository operations (clone, pull, status, diff, commit, push) through
// the shared pkg/git service and provider registry.
//
// All commands follow the command registry pattern (CommandProvider interface)
// and use flags.NewStandardParser() for flag handling.
package git

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"

	// Self-register the CLI provider so Git subprocesses are available.
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
)

// atmosConfigPtr holds the Atmos configuration injected by root.go before
// any subcommand runs.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig is called from root.go after atmosConfig is initialized,
// making the configuration available to all git subcommands.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// gitCmd is the `atmos git` parent command.
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Manage Git repositories as Atmos artifacts",
	Long:  markdown.AtmosGitMarkdown,
}

func init() {
	defer perf.Track(nil, "git.init")()

	// Attach subcommands.
	gitCmd.AddCommand(cloneCmd)
	gitCmd.AddCommand(pullCmd)
	gitCmd.AddCommand(statusCmd)
	gitCmd.AddCommand(diffCmd)
	gitCmd.AddCommand(commitCmd)
	gitCmd.AddCommand(pushCmd)
	gitCmd.AddCommand(cleanCmd)

	// Register this command with the registry.
	internal.Register(&GitCommandProvider{})
}

// GitCommandProvider implements the CommandProvider interface for `atmos git`.
type GitCommandProvider struct{}

// GetCommand returns the git parent command (with all subcommands attached).
func (g *GitCommandProvider) GetCommand() *cobra.Command {
	return gitCmd
}

// GetName returns the command name.
func (g *GitCommandProvider) GetName() string {
	return "git"
}

// GetGroup returns the command group for help organization.
func (g *GitCommandProvider) GetGroup() string {
	return "Git"
}

// GetFlagsBuilder returns the flags builder for this command.
// The parent git command has no flags of its own.
func (g *GitCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// The parent git command has no positional arguments.
func (g *GitCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// The git command group has no compatibility flags.
func (g *GitCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases for git subcommands.
// Registers "atmos list git-repositories" as an alias for "atmos git list",
// following the same pattern as "atmos workflow list" ↔ "atmos list workflows".
func (g *GitCommandProvider) GetAliases() []internal.CommandAlias {
	return []internal.CommandAlias{
		{
			Subcommand:    "list",
			ParentCommand: "list",
			Name:          "git-repositories",
			Short:         "List configured Git repositories (alias for 'atmos git list')",
			Long:          `List managed Git repositories configured under git.repositories in atmos.yaml. This is an alias for "atmos git list".`,
			Example:       "atmos list git-repositories\natmos list git-repositories --format json",
		},
	}
}

// IsExperimental returns whether this command is experimental.
func (g *GitCommandProvider) IsExperimental() bool {
	return true
}
