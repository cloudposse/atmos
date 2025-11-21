package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var rebuildParser *flags.StandardParser

// RebuildOptions contains parsed flags for the rebuild command.
type RebuildOptions struct {
	Instance string
	Attach   bool
	NoPull   bool
	Identity string
}

var rebuildCmd = &cobra.Command{
	Use:   "rebuild <name>",
	Short: "Rebuild a devcontainer",
	Long: `Rebuild a devcontainer from scratch.

This command stops and removes the existing container, pulls the latest image
(unless --no-pull is specified), and creates a new container with the current
configuration. This is useful when you've updated the devcontainer.json or
need to start fresh.`,
	Example:           markdown.DevcontainerRebuildUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.rebuild.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := rebuildParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseRebuildOptions(cmd, v, args)
		if err != nil {
			return err
		}

		// Handle identity selection if __SELECT__ sentinel value is used.
		// This happens when user passes --identity without a value.
		if opts.Identity == cfg.IdentityFlagSelectValue || opts.Identity == "" {
			// If user explicitly requested selection but auth is not configured, show helpful error.
			if opts.Identity == cfg.IdentityFlagSelectValue && !isAuthConfigured(&atmosConfigPtr.Auth) {
				return errUtils.Build(errUtils.ErrAuthNotConfigured).
					WithExplanation("Authentication requires at least one identity configured in atmos.yaml").
					WithHint("Configure authentication in atmos.yaml under the 'auth' section").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/auth/auth-identity-configure/").
					Err()
			}

			// If auth is configured, create manager to access GetDefaultIdentity.
			if isAuthConfigured(&atmosConfigPtr.Auth) {
				authMgr, err := createUnauthenticatedAuthManager(&atmosConfigPtr.Auth)
				if err != nil {
					return err
				}
				// forceSelect=true when user explicitly used --identity flag without value.
				forceSelect := opts.Identity == cfg.IdentityFlagSelectValue
				selectedIdentity, err := authMgr.GetDefaultIdentity(forceSelect)
				if err != nil {
					return err
				}
				opts.Identity = selectedIdentity
			}
		}

		name := args[0]
		mgr := devcontainer.NewManager()
		if err := mgr.Rebuild(atmosConfigPtr, name, opts.Instance, opts.Identity, opts.NoPull); err != nil {
			return err
		}

		// If --attach flag is set, attach to the container after rebuilding.
		if opts.Attach {
			return mgr.Attach(atmosConfigPtr, name, opts.Instance, false)
		}

		return nil
	},
}

// parseRebuildOptions parses command flags into RebuildOptions.
//
// Constructs a RebuildOptions populated from flags and environment values read via viper.
// The cmd and args parameters are unused and retained for API consistency.
func parseRebuildOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*RebuildOptions, error) {
	return &RebuildOptions{
		Instance: v.GetString("instance"),
		Attach:   v.GetBool("attach"),
		NoPull:   v.GetBool("no-pull"),
		Identity: v.GetString("identity"),
	}, nil
}

// init registers the rebuild subcommand and its flags, and wires the flag parser to the command.
// It configures the rebuildParser with instance, attach, no-pull, and identity flags (plus their environment variable mappings) and adds rebuildCmd to devcontainerCmd.
func init() {
	// Create parser with rebuild-specific flags using functional options.
	rebuildParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("attach", "", false, "Attach to the container after rebuilding"),
		flags.WithBoolFlag("no-pull", "", false, "Don't pull the latest image before rebuilding"),
		flags.WithIdentityFlag(),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("attach", "ATMOS_DEVCONTAINER_ATTACH"),
		flags.WithEnvVars("no-pull", "ATMOS_DEVCONTAINER_NO_PULL"),
	)

	initCommandWithFlags(rebuildCmd, rebuildParser)
	devcontainerCmd.AddCommand(rebuildCmd)
}
