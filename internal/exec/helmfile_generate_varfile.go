package exec

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteHelmfileGenerateVarfileCmd executes `helmfile generate varfile` command.
func ExecuteHelmfileGenerateVarfileCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteHelmfileGenerateVarfileCmd")()

	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
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

	info, err = ProcessStacks(&atmosConfig, info, true, true, true, nil, nil)
	if err != nil {
		return err
	}

	// JIT source provisioning: vendor remote component if source is configured.
	if provSource.HasSource(info.ComponentSection) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := provSource.AutoProvisionSource(ctx, &atmosConfig, cfg.HelmfileComponentType, info.ComponentSection, info.AuthContext); err != nil {
			return err
		}
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
