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
	logger "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteWorkflow executes an Atmos workflow
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
		u.PrintErrorMarkdownAndExit(
			"Workflow Error",
			fmt.Errorf("Workflow `%s` does not have any steps defined", workflow),
			fmt.Sprintf("\nPlease add steps to your workflow definition"),
		)
		return nil // This line will never be reached due to PrintErrorMarkdownAndExit
	}

	// Check if the workflow steps have the `name` attribute
	checkAndGenerateWorkflowStepNames(workflowDefinition)

	// Create a logger instance
	l, err := logger.NewLoggerFromCliConfig(atmosConfig)
	if err != nil {
		return err
	}

	l.Debug(fmt.Sprintf("Executing the workflow: workflow=%s file=%s", workflow, workflowPath))

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		err := u.PrintAsYAMLToFileDescriptor(&atmosConfig, workflowDefinition)
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
			stepNames := lo.Map(workflowDefinition.Steps, func(step schema.WorkflowStep, _ int) string { return step.Name })
			u.PrintErrorMarkdownAndExit(
				"Invalid Step",
				fmt.Errorf("Invalid `--from-step` flag. Workflow `%s` does not have a step with the name `%s`", workflow, fromStep),
				fmt.Sprintf("\n## Available Steps\n%s", formatList(stepNames)),
			)
			return nil // This line will never be reached due to PrintErrorMarkdownAndExit
		}
	}

	for stepIdx, step := range steps {
		command := strings.TrimSpace(step.Command)
		commandType := strings.TrimSpace(step.Type)

		l.Debug(fmt.Sprintf("Executing workflow step: step=%d command=%s", stepIdx, command))

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
				l.Debug(fmt.Sprintf("Stack: stack=%s", finalStack))
			}

			err = ExecuteShellCommand(atmosConfig, "atmos", args, ".", []string{}, dryRun, "")
		} else {
			u.PrintErrorMarkdownAndExit(
				"Invalid Step Type",
				fmt.Errorf("Invalid workflow step type `%s`", commandType),
				fmt.Sprintf("\n## Available Step Types\n%s", formatList([]string{"atmos", "shell"})),
			)
			return nil // This line will never be reached due to PrintErrorMarkdownAndExit
		}

		if err != nil {
			l.Debug(fmt.Sprintf("Workflow failed: workflow=%s path=%s step=%s command=%s error=%v",
				workflow, workflowPath, step.Name, command, err))

			workflowFileName := filepath.Base(workflowPath)
			workflowFileName = strings.TrimSuffix(workflowFileName, filepath.Ext(workflowFileName))

			resumeCommand := fmt.Sprintf(
				"atmos workflow %s -f %s --from-step %s",
				workflow,
				workflowFileName,
				step.Name,
			)

			failedCmd := command
			if commandType == "atmos" {
				failedCmd = "atmos " + command
			}

			u.PrintErrorMarkdownAndExit(
				fmt.Sprintf("Step '%s' Failed", step.Name),
				fmt.Errorf("Failed command:\n```\n%s\n```\n", failedCmd),
				fmt.Sprintf("To resume the workflow from this step, run:\n```\n%s\n```", resumeCommand),
			)
			return nil // This line will never be reached due to PrintErrorMarkdownAndExit
		}
	}

	return nil
}

// formatList formats a list of strings into a markdown bullet list.
func formatList(items []string) string {
	var result strings.Builder
	for _, item := range items {
		result.WriteString(fmt.Sprintf("- `%s`\n", item))
	}
	return result.String()
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
