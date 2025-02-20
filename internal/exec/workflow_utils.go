package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	w "github.com/cloudposse/atmos/internal/tui/workflow"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteWorkflow executes an Atmos workflow.
func ExecuteWorkflow(
	atmosConfig schema.AtmosConfiguration,
	workflow string,
	workflowPath string,
	workflowDefinition *schema.WorkflowDefinition,
	dryRun bool,
	commandLineStack string,
	fromStep string,
) error {
	steps := workflowDefinition.Steps

	if len(steps) == 0 {
		return fmt.Errorf("workflow '%s' does not have any steps defined", workflow)
	}

	logFunc := u.LogDebug
	if dryRun {
		logFunc = u.LogInfo
	}

	// Check if the workflow steps have the `name` attribute
	checkAndGenerateWorkflowStepNames(workflowDefinition)

	logFunc(fmt.Sprintf("\nExecuting the workflow '%s' from '%s'\n", workflow, workflowPath))

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		err := u.PrintAsYAMLToFileDescriptor(atmosConfig, workflowDefinition)
		if err != nil {
			return err
		}
	}

	// If `--from-step` is specified, skip all the previous steps
	if fromStep != "" {
		steps = lo.DropWhile[schema.WorkflowStep](steps, func(step schema.WorkflowStep) bool {
			return step.Name != fromStep
		})

		if len(steps) == 0 {
			return fmt.Errorf("invalid '--from-step' flag. Workflow '%s' does not have a step with the name '%s'", workflow, fromStep)
		}
	}

	for stepIdx, step := range steps {
		command := strings.TrimSpace(step.Command)
		commandType := strings.TrimSpace(step.Type)

		logFunc(fmt.Sprintf("Executing workflow step: %s", command))

		if commandType == "" {
			commandType = "atmos"
		}

		var err error

		if commandType == "shell" {
			commandName := fmt.Sprintf("%s-step-%d", workflow, stepIdx)
			err = ExecuteShell(atmosConfig, command, commandName, ".", []string{}, dryRun)
		} else if commandType == "atmos" {
			args := strings.Fields(command)

			workflowStack := strings.TrimSpace(workflowDefinition.Stack)
			stepStack := strings.TrimSpace(step.Stack)
			finalStack := ""

			// The workflow `stack` attribute overrides the stack in the `command` (if specified)
			// The step `stack` attribute overrides the stack in the `command` and the workflow `stack` attribute
			// The stack defined on the command line (`atmos workflow <name> -f <file> -s <stack>`) has the highest priority,
			// it overrides all other stacks attributes
			if workflowStack != "" {
				finalStack = workflowStack
			}

			if stepStack != "" {
				finalStack = stepStack
			}

			if commandLineStack != "" {
				finalStack = commandLineStack
			}

			if finalStack != "" {
				args = append(args, []string{"-s", finalStack}...)
				logFunc(fmt.Sprintf("Stack: %s", finalStack))
			}

			err = ExecuteShellCommand(atmosConfig, "atmos", args, ".", []string{}, dryRun, "")
		} else {
			return fmt.Errorf("invalid workflow step type '%s'. Supported types are 'atmos' and 'shell'", commandType)
		}

		if err != nil {
			workflowFileName := filepath.Base(workflowPath)
			workflowFileName = strings.TrimSuffix(workflowFileName, filepath.Ext(workflowFileName))

			failedMsg := fmt.Sprintf("\nStep '%s' failed!", step.Name)

			u.LogDebug(fmt.Sprintf("\nCommand failed: %s", command))
			u.LogDebug(fmt.Sprintf("Error: %v", err))

			resumeMsg := fmt.Sprintf(
				"\nTo resume the workflow from this step, run:\natmos workflow %s -f %s --from-step %s",
				workflow,
				workflowFileName,
				step.Name,
			)

			return fmt.Errorf("%s\n%s", failedMsg, resumeMsg)
		}
	}

	return nil
}

// ExecuteDescribeWorkflows executes `atmos describe workflows` command.
func ExecuteDescribeWorkflows(
	atmosConfig schema.AtmosConfiguration,
) ([]schema.DescribeWorkflowsItem, map[string][]string, map[string]schema.WorkflowManifest, error) {
	listResult := []schema.DescribeWorkflowsItem{}
	mapResult := make(map[string][]string)
	allResult := make(map[string]schema.WorkflowManifest)

	if atmosConfig.Workflows.BasePath == "" {
		return nil, nil, nil, errors.New("'workflows.base_path' must be configured in 'atmos.yaml'")
	}

	// If `workflows.base_path` is a relative path, join it with `stacks.base_path`
	var workflowsDir string
	if u.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
		workflowsDir = atmosConfig.Workflows.BasePath
	} else {
		workflowsDir = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath)
	}

	isDirectory, err := u.IsDirectory(workflowsDir)
	if err != nil || !isDirectory {
		return nil, nil, nil, fmt.Errorf("the workflow directory '%s' does not exist. Review 'workflows.base_path' in 'atmos.yaml'", workflowsDir)
	}

	files, err := u.GetAllYamlFilesInDir(workflowsDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error reading the directory '%s' defined in 'workflows.base_path' in 'atmos.yaml': %v",
			atmosConfig.Workflows.BasePath, err)
	}

	for _, f := range files {
		var workflowPath string
		if u.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
			workflowPath = filepath.Join(atmosConfig.Workflows.BasePath, f)
		} else {
			workflowPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath, f)
		}

		fileContent, err := os.ReadFile(workflowPath)
		if err != nil {
			return nil, nil, nil, err
		}

		workflowManifest, err := u.UnmarshalYAML[schema.WorkflowManifest](string(fileContent))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error parsing the workflow manifest '%s': %v", f, err)
		}

		if workflowManifest.Workflows == nil {
			return nil, nil, nil, fmt.Errorf("the workflow manifest '%s' must be a map with the top-level 'workflows:' key", workflowPath)
		}

		workflowConfig := workflowManifest.Workflows
		allWorkflowsInFile := lo.Keys(workflowConfig)
		sort.Strings(allWorkflowsInFile)

		// Check if the workflow steps have the `name` attribute
		lo.ForEach(allWorkflowsInFile, func(item string, _ int) {
			workflowDefinition := workflowConfig[item]
			checkAndGenerateWorkflowStepNames(&workflowDefinition)
		})

		mapResult[f] = allWorkflowsInFile
		allResult[f] = workflowManifest
	}

	for k, v := range mapResult {
		for _, w := range v {
			listResult = append(listResult, schema.DescribeWorkflowsItem{
				File:     k,
				Workflow: w,
			})
		}
	}

	return listResult, mapResult, allResult, nil
}

func checkAndGenerateWorkflowStepNames(workflowDefinition *schema.WorkflowDefinition) {
	steps := workflowDefinition.Steps

	if steps == nil {
		return
	}

	// Check if the steps have the `name` attribute.
	// If not, generate a friendly name consisting of a prefix of `step` and followed by the index of the
	// step (the index starts with 1, so the first generated step name would be `step1`)
	for index, step := range steps {
		if step.Name == "" {
			// When iterating through a slice with a range loop, if elements need to be changed,
			// changing the returned value from the range is not changing the original slice element.
			// That return value is a copy of the element.
			// So doing changes to it will not affect the original elements.
			// We need to access the element with the index returned from the range iterator and change it there.
			// https://medium.com/@nsspathirana/common-mistakes-with-go-slices-95f2e9b362a9
			steps[index].Name = fmt.Sprintf("step%d", index+1)
		}
	}
}

func ExecuteWorkflowUI(atmosConfig schema.AtmosConfiguration) (string, string, string, error) {
	_, _, allWorkflows, err := ExecuteDescribeWorkflows(atmosConfig)
	if err != nil {
		return "", "", "", err
	}

	// Start the UI
	app, err := w.Execute(allWorkflows)

	fmt.Println()

	if err != nil {
		return "", "", "", err
	}

	selectedWorkflowFile := app.GetSelectedWorkflowFile()
	selectedWorkflow := app.GetSelectedWorkflow()
	selectedWorkflowStep := app.GetSelectedWorkflowStep()

	// If the user quit the UI, exit
	if app.ExitStatusQuit() || selectedWorkflowFile == "" || selectedWorkflow == "" {
		return "", "", "", nil
	}

	fmt.Println()
	u.PrintMessageInColor(fmt.Sprintf(
		"Executing command:\n"+os.Args[0]+" workflow %s --file %s --from-step \"%s\"\n", selectedWorkflow, selectedWorkflowFile, selectedWorkflowStep),
		theme.Colors.Info,
	)
	fmt.Println()

	return selectedWorkflowFile, selectedWorkflow, selectedWorkflowStep, nil
}
