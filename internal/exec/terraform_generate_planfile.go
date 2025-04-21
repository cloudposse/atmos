package exec

import (
	u "github.com/cloudposse/atmos/pkg/utils"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
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

	componentWorkingDir := constructTerraformComponentWorkingDir(atmosConfig, info)

	if planFileNameFromArg != "" {
		planFilePath = planFileNameFromArg
	} else {
		planFilePath = componentWorkingDir
	}

	// Create a temporary directory for all temporary files
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-generate-planfile")
	if err != nil {
		return errors.Wrap(err, "error creating temporary directory")
	}
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			log.Warn("Error removing temporary directory", "path", path, "error", err)
		}
	}(tmpDir)

	opts := PlanFileOptions{
		ComponentPath: componentWorkingDir,
		TmpDir:        tmpDir,
	}

	planFile, err := prepareNewPlanFile(&atmosConfig, &info, opts)
	if err != nil {
		return err
	}

	// Get the JSON representation of the new plan
	planJSON, err := getTerraformPlanJSON(&atmosConfig, &info, componentWorkingDir, planFile)
	if err != nil {
		return errors.Wrap(err, "error getting JSON for planfile")
	}

	log.Debug("Writing the planfile", "file", planFilePath)

	err = u.WriteToFileAsJSON(componentWorkingDir+".planfile.json", planJSON, 0o644)
	if err != nil {
		return err
	}

	return nil
}
