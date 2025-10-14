package exec

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

//go:embed examples/workflow_invalid_manifest.md
var workflowInvalidManifestExample string

//go:embed examples/workflow_not_found.md
var workflowNotFoundExample string

//go:embed examples/workflow_file_not_found.md
var workflowFileNotFoundExample string

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
		err := errUtils.Build(ErrWorkflowFileNotFound).
			WithExplanationf("The workflow manifest file `%s` does not exist.", filepath.ToSlash(workflowPath)).
			WithExampleFile(workflowFileNotFoundExample).
			WithHint("Use `atmos list workflows` to see available workflows").
			WithHintf("Verify the workflow file exists at: %s", workflowPath).
			WithHintf("Check `workflows.base_path` in `atmos.yaml`: %s", atmosConfig.Workflows.BasePath).
			WithContext("file", workflowPath).
			WithContext("base_path", atmosConfig.Workflows.BasePath).
			WithExitCode(2).
			Err()
		errUtils.CheckErrorAndPrint(err, "", "")
		return err
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
		err := errUtils.Build(ErrInvalidWorkflowManifest).
			WithExplanationf("The workflow manifest `%s` must be a map with the top-level `workflows:` key.", filepath.ToSlash(workflowPath)).
			WithExampleFile(workflowInvalidManifestExample).
			WithHint("Check the YAML structure of your workflow file").
			WithHintf("Valid format requires a top-level `workflows:` key containing workflow definitions").
			WithContext("file", workflowPath).
			WithExitCode(2).
			Err()
		errUtils.CheckErrorAndPrint(err, "", "")
		return err
	}

	workflowConfig = workflowManifest.Workflows

	if i, ok := workflowConfig[workflowName]; !ok {
		validWorkflows := make([]string, 0, len(workflowConfig))
		for w := range workflowConfig {
			validWorkflows = append(validWorkflows, w)
		}
		// sorting so that the output is deterministic.
		sort.Strings(validWorkflows)

		explanation := fmt.Sprintf("The workflow `%s` does not exist.\n\n## Available workflows:\n\n%s",
			workflowName,
			FormatList(validWorkflows))

		err := errUtils.Build(ErrWorkflowNoWorkflow).
			WithExplanation(explanation).
			WithExampleFile(workflowNotFoundExample).
			WithHintf("Use `atmos describe workflows` to see detailed workflow definitions").
			WithHintf("Run a workflow: `atmos workflow <name> -f %s`", workflowPath).
			WithContext("requested_workflow", workflowName).
			WithContext("file", workflowPath).
			WithContext("available_count", fmt.Sprintf("%d", len(validWorkflows))).
			WithExitCode(2).
			Err()
		errUtils.CheckErrorAndPrint(err, "", "")
		return err
	} else {
		workflowDefinition = i
	}

	err = ExecuteWorkflow(atmosConfig, workflowName, workflowPath, &workflowDefinition, dryRun, commandLineStack, fromStep)
	if err != nil {
		return err
	}

	return nil
}
