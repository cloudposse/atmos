package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Keep the full URL visible, including in plain output and terminal recordings.
func printCommandDocumentation(w io.Writer, cmd *cobra.Command) {
	defer perf.Track(nil, "cmd.printCommandDocumentation")()
	fmt.Fprintf(w, "\nDocumentation: %s\n", internal.ResolveCommandMetadata(cmd).DocumentationURL)
}
