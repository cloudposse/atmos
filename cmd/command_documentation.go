package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// Keep the full URL visible, including in plain output and terminal recordings.
func printCommandDocumentation(w io.Writer, cmd *cobra.Command) {
	defer perf.Track(nil, "cmd.printCommandDocumentation")()
	renderer := ui.NewRenderer(w)
	styles := theme.GetCurrentStyles()
	label := renderer.NewStyle().Foreground(styles.Muted.GetForeground())
	link := renderer.NewStyle().Foreground(styles.Link.GetForeground())
	fmt.Fprintf(w, "\n  %s %s\n", label.Render("Documentation:"), link.Render(internal.ResolveCommandMetadata(cmd).DocumentationURL))
}
