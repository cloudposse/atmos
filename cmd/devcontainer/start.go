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

var startParser *flags.StandardParser

// StartOptions contains parsed flags for the start command.
type StartOptions struct {
	Instance string
	Attach   bool
	Identity string
}

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a devcontainer",
	Long: `Start a devcontainer by name.

If the container doesn't exist, it will be created. If it exists but is stopped,
it will be started. Use --instance to manage multiple instances of the same devcontainer.

Use --identity to launch the container with Atmos-managed credentials.`,
	Example:           markdown.DevcontainerStartUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.start.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := startParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseStartOptions(cmd, v, args)
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

		mgr := devcontainer.NewManager()
		name := args[0]
		if err := mgr.Start(atmosConfigPtr, name, opts.Instance, opts.Identity); err != nil {
			return err
		}

		// If --attach flag is set, attach to the container after starting.
		if opts.Attach {
			return mgr.Attach(atmosConfigPtr, name, opts.Instance, false)
		}

		return nil
	},
}

// parseStartOptions parses command flags into StartOptions.
//
// parseStartOptions builds a StartOptions value by reading the relevant flag values from v.
// The args parameter is unused and kept for consistency with other parse* functions.
// It returns the populated StartOptions and a nil error.
func parseStartOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*StartOptions, error) {
	return &StartOptions{
		Instance: v.GetString("instance"),
		Attach:   v.GetBool("attach"),
		Identity: v.GetString("identity"),
	}, nil
}

// init initializes the start subcommand by creating its flag parser (instance, attach, identity),
// binding environment variables, registering the flags with the start command, and adding the
// start command to the devcontainer command tree.
func init() {
	// Create parser with start-specific flags using functional options.
	startParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("attach", "", false, "Attach to the container after starting"),
		flags.WithIdentityFlag(),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("attach", "ATMOS_DEVCONTAINER_ATTACH"),
	)

	initCommandWithFlags(startCmd, startParser)
	devcontainerCmd.AddCommand(startCmd)
}