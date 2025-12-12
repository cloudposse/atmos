package exec

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
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
		// If file is not provided, attempt auto-discovery.
		if workflowFile == "" {
			matches, err := findWorkflowAcrossFiles(workflowName, &atmosConfig)
			if err != nil {
				return err
			}

			switch {
			case len(matches) == 0:
				return errUtils.Build(ErrWorkflowNoWorkflow).
					WithHintf("No workflow found with name `%s`", workflowName).
					WithHint("Use 'atmos describe workflows' to see all available workflows").
					WithExitCode(1).
					Err()
			case len(matches) == 1:
				// Single match - use it automatically.
				workflowFile = matches[0].File
			default:
				// Multiple matches - show interactive selector in TTY, error in CI.
				if !term.IsTTYSupportForStdin() || telemetry.IsCI() {
					// Non-interactive environment - list matching files and error.
					fileList := make([]string, len(matches))
					for i, match := range matches {
						fileList[i] = match.File
					}
					return errUtils.Build(ErrWorkflowNoWorkflow).
						WithHintf("Multiple workflow files contain workflow `%s`", workflowName).
						WithHintf("Matching files: %s", strings.Join(fileList, ", ")).
						WithHintf("Use --file flag to specify which one: atmos workflow %s --file <file>", workflowName).
						WithExitCode(1).
						Err()
				}
				// TTY mode - show interactive selector.
				workflowFile, err = promptForWorkflowFile(matches)
				if err != nil {
					if errors.Is(err, errUtils.ErrUserAborted) {
						return errUtils.ErrUserAborted
					}
					return err
				}
			}
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

	commandLineIdentity, err := flags.GetString("identity")
	if err != nil {
		return err
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
		return errUtils.Build(ErrWorkflowFileNotFound).
			WithHintf("The workflow manifest file `%s` does not exist", filepath.ToSlash(workflowPath)).
			WithExitCode(1).
			Err()
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
		return errUtils.Build(ErrInvalidWorkflowManifest).
			WithExplanationf("The workflow manifest `%s` must be a map with the top-level `workflows:` key", filepath.ToSlash(workflowPath)).
			WithHint("Add a top-level 'workflows:' key to the manifest file").
			WithExitCode(1).
			Err()
	}

	workflowConfig = workflowManifest.Workflows

	if i, ok := workflowConfig[workflowName]; !ok {
		validWorkflows := make([]string, 0, len(workflowConfig))
		for w := range workflowConfig {
			validWorkflows = append(validWorkflows, w)
		}
		// sorting so that the output is deterministic.
		sort.Strings(validWorkflows)

		return errUtils.Build(ErrWorkflowNoWorkflow).
			WithHintf("No workflow exists with name `%s`", workflowName).
			WithHintf("Available workflows in %s: %s", filepath.Base(workflowPath), u.FormatList(validWorkflows)).
			WithExitCode(1).
			Err()
	} else {
		workflowDefinition = i
	}

	err = ExecuteWorkflow(atmosConfig, workflowName, workflowPath, &workflowDefinition, dryRun, commandLineStack, fromStep, commandLineIdentity)
	if err != nil {
		return err
	}

	return nil
}
