package exec

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteWorkflowCmd executes an Atmos workflow
func ExecuteWorkflowCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument <workflow name>")
	}

	info, err := processCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	workflowFile, err := flags.GetString("file")
	if err != nil {
		return err
	}

	dryRun, err := flags.GetBool("dry-run")
	if err != nil {
		return err
	}

	commandLineStack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	fromStep, err := flags.GetString("from-step")
	if err != nil {
		return err
	}

	var workflowPath string
	if u.IsPathAbsolute(workflowFile) {
		workflowPath = workflowFile
	} else {
		workflowPath = path.Join(cliConfig.BasePath, cliConfig.Workflows.BasePath, workflowFile)
	}

	// If the workflow file is specified without an extension, use the default extension
	ext := filepath.Ext(workflowPath)
	if ext == "" {
		ext = cfg.DefaultStackConfigFileExtension
		workflowPath = workflowPath + ext
	}

	if !u.FileExists(workflowPath) {
		return fmt.Errorf("workflow manifest '%s' does not exist", workflowPath)
	}

	fileContent, err := os.ReadFile(workflowPath)
	if err != nil {
		return err
	}

	var yamlContent schema.WorkflowFile
	var workflowConfig schema.WorkflowConfig
	var workflowDefinition schema.WorkflowDefinition

	if err = yaml.Unmarshal(fileContent, &yamlContent); err != nil {
		return err
	}

	if i, ok := yamlContent["workflows"]; !ok {
		return fmt.Errorf("the workflow manifest '%s' must be a map with top-level 'workflows:' key", workflowPath)
	} else {
		workflowConfig = i
	}

	workflow := args[0]

	if i, ok := workflowConfig[workflow]; !ok {
		return fmt.Errorf("the workflow manifest '%s' does not have the '%s' workflow defined", workflowPath, workflow)
	} else {
		workflowDefinition = i
	}

	err = ExecuteWorkflow(cliConfig, workflow, workflowPath, &workflowDefinition, dryRun, commandLineStack, fromStep)
	if err != nil {
		return err
	}

	return nil
}
