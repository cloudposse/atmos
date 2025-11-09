package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var shellParser *flags.StandardParser

// ShellOptions contains parsed flags for the shell command.
type ShellOptions struct {
	Instance string
	Identity string
	UsePTY   bool
	New      bool
	Replace  bool
	Rm       bool
	NoPull   bool
}

var shellCmd = &cobra.Command{
	Use:   "shell [name]",
	Short: "Launch a shell in a devcontainer (alias for 'start --attach')",
	Long: `Launch a shell in a devcontainer.

This is a convenience command that combines start and attach operations:
- Starts the container if it's stopped
- Creates the container if it doesn't exist
- Attaches to the container with an interactive shell

If no devcontainer name is provided, you will be prompted to select one interactively.

This command is consistent with other Atmos shell commands (terraform shell, auth shell)
and provides the quickest way to get into a devcontainer environment.

Experimental: Use --pty for PTY mode with masking support (not available on Windows).

## Instance Management

- --new: Always create a new instance with auto-generated numbered name based on --instance value (e.g., default-1, default-2, or alice-1 with --instance alice)
- --replace: Destroy and recreate the instance specified by --instance flag (default "default")
- --rm: Automatically remove the container when you exit the shell (similar to 'docker run --rm')

## Using Authenticated Identities

Launch a devcontainer with Atmos-managed credentials:

  atmos devcontainer shell <name> --identity <identity-name>

Inside the container, cloud provider SDKs automatically use the authenticated identity.`,
	Example:           markdown.DevcontainerShellUsageMarkdown,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.shell.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := shellParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseShellOptions(cmd, v, args)
		if err != nil {
			return err
		}

		// Get devcontainer name from args or prompt user.
		name, err := getDevcontainerName(args)
		if err != nil {
			return err
		}

		// Handle --replace: destroy and recreate the instance.
		if opts.Replace {
			if err := e.ExecuteDevcontainerRebuild(atmosConfigPtr, name, opts.Instance, opts.Identity, opts.NoPull); err != nil {
				return err
			}
			// Attach to the newly created container.
			err := e.ExecuteDevcontainerAttach(atmosConfigPtr, name, opts.Instance, opts.UsePTY)

			// If --rm flag is set, remove the container after exit.
			if opts.Rm {
				if rmErr := e.ExecuteDevcontainerRemove(atmosConfigPtr, name, opts.Instance, true); rmErr != nil {
					if err != nil {
						return err
					}
					return rmErr
				}
			}

			return err
		}

		// Handle --new: create a new instance with auto-generated name.
		if opts.New {
			newInstance, err := e.GenerateNewDevcontainerInstance(atmosConfigPtr, name, opts.Instance)
			if err != nil {
				return err
			}
			opts.Instance = newInstance
		}

		// Start the container (creates if necessary).
		if err := e.ExecuteDevcontainerStart(atmosConfigPtr, name, opts.Instance, opts.Identity); err != nil {
			return err
		}

		// Attach to the container.
		err = e.ExecuteDevcontainerAttach(atmosConfigPtr, name, opts.Instance, opts.UsePTY)

		// If --rm flag is set, remove the container after exit.
		if opts.Rm {
			// Remove the container (force=true to remove even if running).
			if rmErr := e.ExecuteDevcontainerRemove(atmosConfigPtr, name, opts.Instance, true); rmErr != nil {
				// If attach failed, return attach error; otherwise return remove error.
				if err != nil {
					return err
				}
				return rmErr
			}
		}

		return err
	},
}

// parseShellOptions parses command flags into ShellOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseShellOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*ShellOptions, error) {
	return &ShellOptions{
		Instance: v.GetString("instance"),
		Identity: v.GetString("identity"),
		UsePTY:   v.GetBool("pty"),
		New:      v.GetBool("new"),
		Replace:  v.GetBool("replace"),
		Rm:       v.GetBool("rm"),
		NoPull:   v.GetBool("no-pull"),
	}, nil
}

func init() {
	// Create parser with shell-specific flags using functional options.
	shellParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithStringFlag("identity", "i", "", "Authenticate with specified identity"),
		flags.WithBoolFlag("pty", "", false, "Experimental: Use PTY mode with masking support (not available on Windows)"),
		flags.WithBoolFlag("new", "", false, "Create a new instance with auto-generated name"),
		flags.WithBoolFlag("replace", "", false, "Destroy and recreate the current instance"),
		flags.WithBoolFlag("rm", "", false, "Automatically remove the container when the shell exits"),
		flags.WithBoolFlag("no-pull", "", false, "Skip pulling the image when using --replace (use cached image)"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("identity", "ATMOS_DEVCONTAINER_IDENTITY"),
		flags.WithEnvVars("pty", "ATMOS_DEVCONTAINER_PTY"),
		flags.WithEnvVars("new", "ATMOS_DEVCONTAINER_NEW"),
		flags.WithEnvVars("replace", "ATMOS_DEVCONTAINER_REPLACE"),
		flags.WithEnvVars("rm", "ATMOS_DEVCONTAINER_RM"),
		flags.WithEnvVars("no-pull", "ATMOS_DEVCONTAINER_NO_PULL"),
	)

	// Register flags using the standard RegisterFlags method.
	shellParser.RegisterFlags(shellCmd)

	// Bind flags to Viper for environment variable support.
	if err := shellParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Mark flags as mutually exclusive.
	shellCmd.MarkFlagsMutuallyExclusive("new", "replace")

	devcontainerCmd.AddCommand(shellCmd)
}
