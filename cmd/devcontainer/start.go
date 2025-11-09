package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	e "github.com/cloudposse/atmos/internal/exec"
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

		name := args[0]
		if err := e.ExecuteDevcontainerStart(atmosConfigPtr, name, opts.Instance, opts.Identity); err != nil {
			return err
		}

		// If --attach flag is set, attach to the container after starting.
		if opts.Attach {
			return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, opts.Instance, false)
		}

		return nil
	},
}

// parseStartOptions parses command flags into StartOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseStartOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*StartOptions, error) {
	return &StartOptions{
		Instance: v.GetString("instance"),
		Attach:   v.GetBool("attach"),
		Identity: v.GetString("identity"),
	}, nil
}

func init() {
	// Create parser with start-specific flags using functional options.
	startParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("attach", "", false, "Attach to the container after starting"),
		flags.WithStringFlag("identity", "i", "", "Authenticate with specified identity"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("attach", "ATMOS_DEVCONTAINER_ATTACH"),
		flags.WithEnvVars("identity", "ATMOS_DEVCONTAINER_IDENTITY"),
	)

	// Register flags using the standard RegisterFlags method.
	startParser.RegisterFlags(startCmd)

	// Bind flags to Viper for environment variable support.
	if err := startParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	devcontainerCmd.AddCommand(startCmd)
}
