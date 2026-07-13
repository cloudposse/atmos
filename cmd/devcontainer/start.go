package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var startParser *flags.StandardFlagParser

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
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.start.RunE")()

		// Guard against nil atmosConfigPtr to prevent panics in tests or programmatic usage.
		if atmosConfigPtr == nil {
			return errUtils.Build(errUtils.ErrAtmosConfigIsNil).
				WithExplanation("Atmos configuration was not initialized for the devcontainer command").
				WithHint("Ensure cmd.Execute() has been called before invoking devcontainer subcommands").
				Err()
		}

		parsed, err := startParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}
		opts := parseStartOptions(parsed)

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
		name := parsed.PositionalArgs[0]
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
// ParseStartOptions reads parsed flags into a StartOptions value.
func parseStartOptions(parsed *flags.ParsedConfig) *StartOptions {
	return &StartOptions{
		Instance: flags.GetString(parsed.Flags, "instance"),
		Attach:   flags.GetBool(parsed.Flags, "attach"),
		Identity: flags.GetString(parsed.Flags, "identity"),
	}
}

// init initializes the start subcommand by creating its flag parser (instance, attach, identity),
// binding environment variables, registering the flags with the start command, and adding the
// start command to the devcontainer command tree.
func init() {
	// Create parser with start-specific flags using functional options.
	var usage string
	startParser, usage = newDevcontainerParser(
		true,
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("attach", "", false, "Attach to the container after starting"),
		flags.WithIdentityFlag(),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("attach", "ATMOS_DEVCONTAINER_ATTACH"),
	)
	startCmd.Use = "start " + usage

	initCommandWithFlags(startCmd, startParser)
	devcontainerCmd.AddCommand(startCmd)
}
