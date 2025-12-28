package auth

import (
	"sort"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// identityFlagCompletion provides shell completion for identity flags by fetching
// available identities from the Atmos configuration.
func identityFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "auth.identityFlagCompletion")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

// AddIdentityCompletion registers shell completion for the identity flag if present on the command.
func AddIdentityCompletion(cmd *cobra.Command) {
	defer perf.Track(nil, "auth.AddIdentityCompletion")()

	if cmd.Flag("identity") != nil {
		if err := cmd.RegisterFlagCompletionFunc("identity", identityFlagCompletion); err != nil {
			log.Trace("Failed to register identity flag completion", "error", err)
		}
	}
}

// IdentityArgCompletion provides shell completion for identity positional arguments.
// It returns a list of available identities from the Atmos configuration.
func IdentityArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "auth.IdentityArgCompletion")()

	// Only complete the first positional argument.
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

// ComponentsArgCompletion provides shell completion for component arguments.
// This delegates to the cmd package's ComponentsArgCompletion.
// Note: For auth commands, we use cobra.NoFileCompletions since auth doesn't deal with components.
func ComponentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "auth.ComponentsArgCompletion")()

	// Auth commands don't use component completion - return no file completions.
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// ProviderArgCompletion provides shell completion for provider positional arguments.
// It returns a list of available providers from the Atmos configuration.
func ProviderArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "auth.ProviderArgCompletion")()

	// Only complete the first positional argument.
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var providers []string
	if atmosConfig.Auth.Providers != nil {
		for name := range atmosConfig.Auth.Providers {
			providers = append(providers, name)
		}
	}

	sort.Strings(providers)

	return providers, cobra.ShellCompDirectiveNoFileComp
}
