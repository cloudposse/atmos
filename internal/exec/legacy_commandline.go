package exec

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/component/commandline"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessCommandLineArgs delegates to its owning package.
//
// Deprecated: Use commandline.ProcessCommandLineArgs.
func ProcessCommandLineArgs(
	componentType string,
	cmd *cobra.Command,
	args []string,
	additionalArgsAndFlags []string,
) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "exec.ProcessCommandLineArgs")()

	return commandline.Parse(commandline.Request{ComponentType: componentType, Command: cmd, Args: args, AdditionalArgsAndFlags: additionalArgsAndFlags})
}

func getCliVars(args []string) (map[string]any, error) { return commandline.ParseVariables(args) }
func processArgsAndFlags(componentType string, args []string) (schema.ArgsAndFlagsInfo, error) {
	return commandline.ParseArguments(componentType, args)
}
