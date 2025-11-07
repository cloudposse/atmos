package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	attachInstance string
	attachUsePTY   bool // Experimental PTY mode flag.
)

var attachCmd = &cobra.Command{
	Use:   "attach <name>",
	Short: "Attach to a running devcontainer",
	Long: `Attach to a running devcontainer and get an interactive shell.

If the container is not running, it will be started automatically.

Experimental: Use --pty for PTY mode with masking support (not available on Windows).`,
	Example:           markdown.DevcontainerAttachUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.attach.RunE")()

		name := args[0]
		return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, attachInstance, attachUsePTY)
	},
}

func init() {
	attachCmd.Flags().StringVar(&attachInstance, "instance", "default", "Instance name for this devcontainer")
	attachCmd.Flags().BoolVar(&attachUsePTY, "pty", false, "Experimental: Use PTY mode with masking support (not available on Windows)")
	devcontainerCmd.AddCommand(attachCmd)
}
