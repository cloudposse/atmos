package version

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// renderOutputPerm is the permission for rendered output files. Rendered files
// are ordinary project files (e.g. GitHub Actions workflows), not secrets.
const renderOutputPerm os.FileMode = 0o644

var trackRenderCmd = &cobra.Command{
	Use:   "render [track]",
	Short: "Render a template file with locked .version values",
	Long:  "Render a single template file with the .version context resolved from the lock file. Use --output to write the result and --check to verify a committed file is current.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.track.render.RunE")()

		file, _ := cmd.Flags().GetString("file")
		output, _ := cmd.Flags().GetString("output")
		check, _ := cmd.Flags().GetBool("check")
		if file == "" {
			return ErrRenderFileRequired
		}
		rendered, err := manager.RenderFile(atmosConfigPtr, trackFromArgs(cmd, args), file, renderTemplate)
		if err != nil {
			return err
		}
		if check {
			checkPath := file
			if output != "" {
				checkPath = output
			}
			current, err := os.ReadFile(checkPath)
			if err != nil {
				return err
			}
			if string(current) != rendered {
				return fmt.Errorf("%w: %s", ErrRenderDrift, checkPath)
			}
			return nil
		}
		if output != "" {
			return os.WriteFile(output, []byte(rendered), renderOutputPerm) // #nosec G306 -- rendered output is a non-sensitive project file.
		}
		return data.Write(rendered)
	},
}

// renderTemplate adapts the Atmos template engine to manager.RenderFunc.
func renderTemplate(atmosConfig *schema.AtmosConfiguration, name, content string, templateData map[string]any) (string, error) {
	return exec.ProcessTmpl(atmosConfig, name, content, templateData, false)
}

func init() {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("track", "", "", "Version track to operate on (defaults to version.track in atmos.yaml)"),
		flags.WithStringFlag("file", "", "", "Template source file to render"),
		flags.WithStringFlag("output", "", "", "Rendered output file"),
		flags.WithBoolFlag("check", "", false, "Check that rendered output matches the output file"),
	)
	parser.RegisterFlags(trackRenderCmd)
	trackCmd.AddCommand(trackRenderCmd)
}
