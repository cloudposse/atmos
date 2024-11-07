package exec

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	w "github.com/cloudposse/atmos/internal/tui/workflow"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteWorkflow executes an Atmos workflow
func ExecuteWorkflow(
	cliConfig schema.CliConfiguration,
	workflow string,
	workflowPath string,
	workflowDefinition *schema.WorkflowDefinition,
	dryRun bool,
	commandLineStack string,
	fromStep string,
) error {
	var steps = workflowDefinition.Steps

	if len(steps) == 0 {
		return fmt.Errorf("workflow '%s' does not have any steps defined", workflow)
	}

	logFunc := u.LogDebug
	if dryRun {
		logFunc = u.LogInfo
	}

	// Check if the workflow steps have the `name` attribute
	checkAndGenerateWorkflowStepNames(workflowDefinition)

	logFunc(cliConfig, fmt.Sprintf("\nExecuting the workflow '%s' from '%s'\n", workflow, workflowPath))

	if cliConfig.Logs.Level == u.LogLevelTrace || cliConfig.Logs.Level == u.LogLevelDebug {
		err := u.PrintAsYAMLToFileDescriptor(cliConfig, workflowDefinition)
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
		var command = strings.TrimSpace(step.Command)
		var commandType = strings.TrimSpace(step.Type)

		logFunc(cliConfig, fmt.Sprintf("Executing workflow step: %s", command))

		if commandType == "" {
			commandType = "atmos"
		}

		var err error
		if commandType == "shell" {
			commandName := fmt.Sprintf("%s-step-%d", workflow, stepIdx)
			err = ExecuteShell(cliConfig, command, commandName, ".", []string{}, dryRun)
		} else if commandType == "atmos" {
			args := strings.Fields(command)

			var workflowStack = strings.TrimSpace(workflowDefinition.Stack)
			var stepStack = strings.TrimSpace(step.Stack)
			var finalStack = ""

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
				logFunc(cliConfig, fmt.Sprintf("Stack: %s", finalStack))
			}

			err = ExecuteShellCommand(cliConfig, "atmos", args, ".", []string{}, dryRun, "")
		} else {
			return fmt.Errorf("invalid workflow step type '%s'. Supported types are 'atmos' and 'shell'", commandType)
		}

		if err != nil {
			workflowFileName := filepath.Base(workflowPath)
			workflowFileName = strings.TrimSuffix(workflowFileName, filepath.Ext(workflowFileName))

			failedMsg := color.New(color.FgRed).Sprintf("\nStep '%s' failed!", step.Name)
			cmdMsg := color.New(color.FgYellow).Sprintf("Failed command: %s", command)
			resumeMsg := color.New(color.FgGreen).Sprintf(
				"\nTo resume the workflow from this step, run:\ncd %s && atmos workflow %s -f %s --from-step %s",
				filepath.Dir(workflowPath),
				workflow,
				workflowFileName,
				step.Name,
			)

			return fmt.Errorf("%s\n%s\n%s\nError: %v", failedMsg, cmdMsg, resumeMsg, err)
		}
	}

	return nil
}

// ExecuteDescribeWorkflows executes `atmos describe workflows` command
func ExecuteDescribeWorkflows(
	cliConfig schema.CliConfiguration,
) ([]schema.DescribeWorkflowsItem, map[string][]string, map[string]schema.WorkflowManifest, error) {

	listResult := []schema.DescribeWorkflowsItem{}
	mapResult := make(map[string][]string)
	allResult := make(map[string]schema.WorkflowManifest)

	if cliConfig.Workflows.BasePath == "" {
		return nil, nil, nil, errors.New("'workflows.base_path' must be configured in 'atmos.yaml'")
	}

	// If `workflows.base_path` is a relative path, join it with `stacks.base_path`
	var workflowsDir string
	if u.IsPathAbsolute(cliConfig.Workflows.BasePath) {
		workflowsDir = cliConfig.Workflows.BasePath
	} else {
		workflowsDir = path.Join(cliConfig.BasePath, cliConfig.Workflows.BasePath)
	}

	isDirectory, err := u.IsDirectory(workflowsDir)
	if err != nil || !isDirectory {
		return nil, nil, nil, fmt.Errorf("the workflow directory '%s' does not exist. Review 'workflows.base_path' in 'atmos.yaml'", workflowsDir)
	}

	files, err := u.GetAllYamlFilesInDir(workflowsDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error reading the directory '%s' defined in 'workflows.base_path' in 'atmos.yaml': %v",
			cliConfig.Workflows.BasePath, err)
	}

	for _, f := range files {
		var workflowPath string
		if u.IsPathAbsolute(cliConfig.Workflows.BasePath) {
			workflowPath = path.Join(cliConfig.Workflows.BasePath, f)
		} else {
			workflowPath = path.Join(cliConfig.BasePath, cliConfig.Workflows.BasePath, f)
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
	var steps = workflowDefinition.Steps

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

func ExecuteWorkflowUI(cliConfig schema.CliConfiguration) (string, string, string, error) {
	_, _, allWorkflows, err := ExecuteDescribeWorkflows(cliConfig)
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
		color.New(color.FgCyan),
	)
	fmt.Println()

	return selectedWorkflowFile, selectedWorkflow, selectedWorkflowStep, nil
}
