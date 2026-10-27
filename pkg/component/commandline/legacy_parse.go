package commandline

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessCommandLineArgs parses a component command.
//
// Deprecated: Use Parse with a Request.
func ProcessCommandLineArgs(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "commandline.ProcessCommandLineArgs")()

	return Parse(Request{ComponentType: componentType, Command: cmd, Args: args, AdditionalArgsAndFlags: additionalArgsAndFlags})
}
