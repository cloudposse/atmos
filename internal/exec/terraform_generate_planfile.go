package exec

import (
	"errors"
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformGeneratePlanfileCmd executes `terraform generate planfile` command.
func ExecuteTerraformGeneratePlanfileCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		return err
	}

	processYamlFunctions, err := flags.GetBool("process-functions")
	if err != nil {
		return err
	}

	skip, err := flags.GetStringSlice("skip")
	if err != nil {
		return err
	}

	component := args[0]

	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "terraform"

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(atmosConfig, info, true, processTemplates, processYamlFunctions, skip)
	if err != nil {
		return err
	}

	var planFileNameFromArg string
	var planFilePath string

	planFileNameFromArg, err = flags.GetString("file")
	if err != nil {
		planFileNameFromArg = ""
	}

	if len(planFileNameFromArg) > 0 {
		planFilePath = planFileNameFromArg
	} else {
		planFilePath = constructTerraformComponentPlanfilePath(atmosConfig, info)
	}

	log.Debug("Writing the planfile", "file", planFilePath)

	err = u.WriteToFileAsJSON(planFilePath, info.ComponentVarsSection, 0o644)
	if err != nil {
		return err
	}

	return nil
}
