package exec

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteHelmfileGenerateVarfileCmd executes `helmfile generate varfile` command.
func ExecuteHelmfileGenerateVarfileCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteHelmfileGenerateVarfileCmd")()

	if len(args) != 1 {
		return errUtils.Build(errUtils.ErrInvalidArguments).
			WithHint("Provide exactly one argument: the component name").
			WithHint("Example: atmos helmfile generate varfile <component> -s <stack>").
			WithExitCode(2).
			Err()
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	component := args[0]

	info, err := ProcessCommandLineArgs("helmfile", cmd, args, nil)
	if err != nil {
		return err
	}

	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "helmfile"
	info.CliArgs = []string{"helmfile", "generate", "varfile"}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(&atmosConfig, info, true, true, true, nil)
	if err != nil {
		return err
	}

	var varFileNameFromArg string
	var varFilePath string

	varFileNameFromArg, err = flags.GetString("file")
	if err != nil {
		varFileNameFromArg = ""
	}

	if len(varFileNameFromArg) > 0 {
		varFilePath = varFileNameFromArg
	} else {
		varFilePath = constructHelmfileComponentVarfilePath(&atmosConfig, &info)
	}

	// Print the component variables
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack)

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		err = u.PrintAsYAMLToFileDescriptor(&atmosConfig, info.ComponentVarsSection)
		if err != nil {
			return err
		}
	}

	// Write the variables to file
	log.Debug("Writing the variables to file", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	return nil
}
