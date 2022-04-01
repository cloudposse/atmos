package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	g "github.com/cloudposse/atmos/pkg/globals"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path"
	"path/filepath"
)

// ExecuteWorkflow executes a workflow
func ExecuteWorkflow(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `workflow name` and one flag `file name`")
	}

	err := c.InitConfig()
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	workflowFile, err := flags.GetString("file")
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
		return errors.New(fmt.Sprintf("File '%s' does not exist", workflowPath))
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
		return errors.New(fmt.Sprintf("the file '%s' does not have the '%s' workflow defined", workflowPath, workflow))
	} else {
		workflowDefinition = i
	}

	color.Cyan("\nExecuting the workflow '%s' from '%s'\n", workflow, workflowPath)
	fmt.Println()

	err = u.PrintAsYAML(workflowDefinition)
	if err != nil {
		return err
	}

	err = executeWorkflowSteps(workflowDefinition, commandLineStack)
	if err != nil {
		return err
	}

	fmt.Println()
	return nil
}
