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

	// Check if --stack flag is provided to determine if stacks should be processed.
	// Workflows only require stacks configuration when --stack is explicitly passed.
	flags := cmd.Flags()
	commandLineStack, err := flags.GetString("stack")
	if err != nil {
		return err
	}
	commandLineTags, err := flags.GetStringSlice("tags")
	if err != nil {
		return err
	}
	commandLineLabels, err := flags.GetString("labels")
	if err != nil {
		return err
	}
	processStacks := commandLineStack != ""

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
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
				return errUtils.Build(errUtils.ErrWorkflowNoWorkflow).
					WithExplanationf("No workflow found with name `%s`", workflowName).
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
					// Sort for deterministic output (important for tests and snapshots).
					sort.Strings(fileList)
					return errUtils.Build(errUtils.ErrWorkflowNoWorkflow).
						WithExplanationf("Multiple workflow files contain workflow `%s`", workflowName).
						WithExplanationf("Matching files: %s", strings.Join(fileList, ", ")).
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

	workflowPath := ResolveWorkflowFilePath(&atmosConfig, workflowFile)

	workflowConfig, err := LoadWorkflowConfig(workflowPath)
	if err != nil {
		return err
	}

	workflowDefinition, ok := workflowConfig[workflowName]
	if !ok {
		validWorkflows := make([]string, 0, len(workflowConfig))
		for w := range workflowConfig {
			validWorkflows = append(validWorkflows, w)
		}
		// sorting so that the output is deterministic.
		sort.Strings(validWorkflows)

		return errUtils.Build(errUtils.ErrWorkflowNoWorkflow).
			WithExplanationf("No workflow exists with name `%s`", workflowName).
			WithHintf("Available workflows in %s:\n\n%s", filepath.Base(workflowPath), u.FormatList(validWorkflows)).
			WithExitCode(1).
			Err()
	}

	err = ExecuteWorkflow(atmosConfig, workflowName, workflowPath, &workflowDefinition, dryRun, commandLineStack, fromStep, commandLineIdentity,
		workflowCommandFilters{tags: commandLineTags, labels: commandLineLabels})
	if err != nil {
		return err
	}

	return nil
}

// ResolveWorkflowFilePath resolves a workflow file reference (as given to the CLI's `-f`/
// `--file` flag, or a `dependencies.workflows[].file` entry) to an absolute path, applying the
// same base-path-join and default-extension rules the `atmos workflow` command itself uses.
func ResolveWorkflowFilePath(atmosConfig *schema.AtmosConfiguration, file string) string {
	defer perf.Track(atmosConfig, "exec.ResolveWorkflowFilePath")()

	var workflowPath string
	if u.IsPathAbsolute(file) {
		workflowPath = file
	} else {
		workflowPath = filepath.Join(getWorkflowsDirToUse(atmosConfig), file)
	}

	// If the workflow file is specified without an extension, use the default extension.
	if filepath.Ext(workflowPath) == "" {
		workflowPath += u.DefaultStackConfigFileExtension
	}
	return workflowPath
}

// LoadWorkflowConfig reads and parses a workflow manifest file at the given (already-resolved,
// see ResolveWorkflowFilePath) path into its WorkflowConfig map (workflow name -> definition,
// for every workflow defined in that file).
func LoadWorkflowConfig(workflowPath string) (schema.WorkflowConfig, error) {
	defer perf.Track(nil, "exec.LoadWorkflowConfig")()

	if !u.FileExists(workflowPath) {
		return nil, errUtils.Build(errUtils.ErrWorkflowFileNotFound).
			WithExplanationf("The workflow manifest file `%s` does not exist", filepath.ToSlash(displayPath(workflowPath))).
			WithExitCode(1).
			Err()
	}

	fileContent, err := os.ReadFile(workflowPath)
	if err != nil {
		return nil, err
	}

	workflowManifest, err := u.UnmarshalYAML[schema.WorkflowManifest](string(fileContent))
	if err != nil {
		return nil, err
	}

	if workflowManifest.Workflows == nil {
		return nil, errUtils.Build(errUtils.ErrInvalidWorkflowManifest).
			WithExplanationf("The workflow manifest `%s` must be a map with the top-level `workflows:` key", filepath.ToSlash(displayPath(workflowPath))).
			WithHint("Add a top-level 'workflows:' key to the manifest file").
			WithExitCode(1).
			Err()
	}

	return workflowManifest.Workflows, nil
}
