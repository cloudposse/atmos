package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteWorkflowCmd executes an Atmos workflow.
func ExecuteWorkflowCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteWorkflowCmd")()

	var workflowName string
	var workflowFile string
	var fromStep string

	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	// If the `workflow` argument is not passed, start the workflow UI
	if len(args) != 1 {
		workflowFile, workflowName, fromStep, err = ExecuteWorkflowUI(atmosConfig)
		if err != nil {
			return err
		}
		if workflowFile == "" || workflowName == "" {
			return nil
		}
	}

	if workflowName == "" {
		workflowName = args[0]
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
		workflowPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath, workflowFile)
	}

	// If the workflow file is specified without an extension, use the default extension
	ext := filepath.Ext(workflowPath)
	if ext == "" {
		ext = u.DefaultStackConfigFileExtension
		workflowPath = workflowPath + ext
	}

	if !u.FileExists(workflowPath) {
		errUtils.CheckErrorPrintAndExit(
			ErrWorkflowFileNotFound,
			WorkflowErrTitle,
			fmt.Sprintf("\n## Explanation\nThe workflow manifest file `%s` does not exist.", filepath.ToSlash(workflowPath)),
		)
		return ErrWorkflowFileNotFound
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
		errUtils.CheckErrorPrintAndExit(
			ErrInvalidWorkflowManifest,
			WorkflowErrTitle,
			fmt.Sprintf("\n## Explanation\nThe workflow manifest `%s` must be a map with the top-level `workflows:` key.", filepath.ToSlash(workflowPath)),
		)
		return ErrInvalidWorkflowManifest
	}

	workflowConfig = workflowManifest.Workflows

	if i, ok := workflowConfig[workflowName]; !ok {
		validWorkflows := make([]string, 0, len(workflowConfig))
		for w := range workflowConfig {
			validWorkflows = append(validWorkflows, w)
		}
		// sorting so that the output is deterministic
		sort.Strings(validWorkflows)
		errUtils.CheckErrorPrintAndExit(
			ErrWorkflowNoWorkflow,
			"Workflow Error",
			fmt.Sprintf("\n## Explanation\nNo workflow exists with the name `%s`\n### Available workflows:\n%s", workflowName, FormatList(validWorkflows)),
		)
		return ErrWorkflowNoWorkflow
	} else {
		workflowDefinition = i
	}

	err = ExecuteWorkflow(atmosConfig, workflowName, workflowPath, &workflowDefinition, dryRun, commandLineStack, fromStep)
	if err != nil {
		return err
	}

	return nil
}
