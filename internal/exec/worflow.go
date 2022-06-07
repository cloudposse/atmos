package exec

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"

	c "github.com/cloudposse/atmos/pkg/config"
	g "github.com/cloudposse/atmos/pkg/globals"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// ExecuteWorkflow executes a workflow
func ExecuteWorkflow(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `workflow name`")
	}

	// InitConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	err := c.InitConfig()
	if err != nil {
		return err
	}

	// ProcessConfig processes all the ENV vars and command line arguments
	// Even if all workflow steps of type `atmos` process the ENV vars by calling InitConfig/ProcessConfig,
	// we need call it from `atmos workflow` command to take into account the `ATMOS_WORKFLOWS_BASE_PATH` ENV var
	var configAndStacksInfo c.ConfigAndStacksInfo
	err = c.ProcessConfig(configAndStacksInfo, false)
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

	var workflowPath string
	if u.IsPathAbsolute(workflowFile) {
		workflowPath = workflowFile
	} else {
		workflowPath = path.Join(c.Config.BasePath, c.Config.Workflows.BasePath, workflowFile)
	}

	// If the file is specified without an extension, use the default extension
	ext := filepath.Ext(workflowPath)
	if ext == "" {
		ext = g.DefaultStackConfigFileExtension
		workflowPath = workflowPath + ext
	}

	if !u.FileExists(workflowPath) {
		return fmt.Errorf("file '%s' does not exist", workflowPath)
	}

	fileContent, err := ioutil.ReadFile(workflowPath)
	if err != nil {
		return err
	}

	var yamlContent c.WorkflowFile
	var workflowConfig c.WorkflowConfig
	var workflowDefinition c.WorkflowDefinition

	if err = yaml.Unmarshal(fileContent, &yamlContent); err != nil {
		return err
	}

	if i, ok := yamlContent["workflows"]; !ok {
		return errors.New("a workflow file must be a map with top-level 'workflows:' key")
	} else {
		workflowConfig = i
	}

	workflow := args[0]

	if i, ok := workflowConfig[workflow]; !ok {
		return fmt.Errorf("the file '%s' does not have the '%s' workflow defined", workflowPath, workflow)
	} else {
		workflowDefinition = i
	}

	u.PrintInfo(fmt.Sprintf("\nExecuting the workflow '%s' from '%s'\n", workflow, workflowPath))
	fmt.Println()

	err = u.PrintAsYAML(workflowDefinition)
	if err != nil {
		return err
	}

	err = executeWorkflowSteps(workflowDefinition, dryRun, commandLineStack)
	if err != nil {
		return err
	}

	fmt.Println()
	return nil
}
