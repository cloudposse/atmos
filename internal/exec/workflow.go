package exec

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteWorkflowCmd executes an Atmos workflow
func ExecuteWorkflowCmd(cmd *cobra.Command, args []string) error {
	var workflow string
	var workflowFile string
	var fromStep string

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

	// If the `workflow` argument is not passed, start the workflow UI
	if len(args) != 1 {
		workflowFile, workflow, fromStep, err = ExecuteWorkflowUI(cliConfig)
		if err != nil {
			return err
		}
		if workflowFile == "" || workflow == "" {
			return nil
		}
	}

	if workflow == "" {
		workflow = args[0]
	}

	flags := cmd.Flags()

	if workflowFile == "" {
		workflowFile, err = flags.GetString("file")
		if err != nil {
			return err
		}
		if workflowFile == "" {
			return errors.New("'--file' flag is required to specify a workflow manifest")
		}
	}

	dryRun, err := flags.GetBool("dry-run")
	if err != nil {
		return err
	}

	commandLineStack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	if fromStep == "" {
		fromStep, err = flags.GetString("from-step")
		if err != nil {
			return err
		}
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
		ext = u.DefaultStackConfigFileExtension
		workflowPath = workflowPath + ext
	}

	if !u.FileExists(workflowPath) {
		return fmt.Errorf("the workflow manifest file '%s' does not exist", workflowPath)
	}

	fileContent, err := os.ReadFile(workflowPath)
	if err != nil {
		return err
	}

	var workflowManifest schema.WorkflowManifest
	var workflowConfig schema.WorkflowConfig
	var workflowDefinition schema.WorkflowDefinition

	workflowManifest, err = u.UnmarshalYAML[schema.WorkflowManifest](string(fileContent))
	if err != nil {
		return err
	}

	if workflowManifest.Workflows == nil {
		return fmt.Errorf("the workflow manifest '%s' must be a map with the top-level 'workflows:' key", workflowPath)
	}

	workflowConfig = workflowManifest.Workflows

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
