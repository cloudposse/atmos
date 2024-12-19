package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ErrorMessage represents a structured error message
type ErrorMessage struct {
	Title      string
	Details    string
	Suggestion string
}

// String returns the markdown representation of the error message
func (e ErrorMessage) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# ❌ %s\n\n", e.Title))

	if e.Details != "" {
		sb.WriteString(fmt.Sprintf("## Details\n%s\n\n", e.Details))
	}

	if e.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("## Suggestion\n%s\n\n", e.Suggestion))
	}

	return sb.String()
}

// renderError renders an error message using the markdown renderer
func renderError(msg ErrorMessage) error {
	renderer, err := markdown.NewRenderer(
		markdown.WithWidth(80),
	)
	if err != nil {
		return fmt.Errorf("failed to create markdown renderer: %w", err)
	}

	rendered, err := renderer.Render(msg.String())
	if err != nil {
		return fmt.Errorf("failed to render error message: %w", err)
	}

	fmt.Print(rendered)
	return nil
}

// workflowCmd executes a workflow
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Execute a workflow",
	Long:  `This command executes a workflow: atmos workflow <name> --file <file>`,
	Example: "atmos workflow\n" +
		"atmos workflow <name> --file <file>\n" +
		"atmos workflow <name> --file <file> --stack <stack>\n" +
		"atmos workflow <name> --file <file> --from-step <step-name>\n\n" +
		"To resume the workflow from this step, run:\n" +
		"atmos workflow deploy-infra --file workflow1 --from-step deploy-vpc\n\n" +
		"For more details refer to https://atmos.tools/cli/commands/workflow/",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are provided, start the workflow UI
		if len(args) == 0 {
			err := e.ExecuteWorkflowCmd(cmd, args)
			if err != nil {
				u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			}
			return
		}

		// Check if the workflow name is "list" (invalid command)
		if args[0] == "list" {
			err := renderError(ErrorMessage{
				Title:      "Invalid Command",
				Details:    "The command `atmos workflow list` is not valid.",
				Suggestion: "Use `atmos workflow --file <file>` to execute a workflow, or run `atmos workflow` without arguments to use the interactive UI.",
			})
			if err != nil {
				u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			}
			return
		}

		// Get the --file flag value
		workflowFile, _ := cmd.Flags().GetString("file")
		if workflowFile == "" {
			err := renderError(ErrorMessage{
				Title:      "Missing Required Flag",
				Details:    "The `--file` flag is required to specify a workflow manifest.",
				Suggestion: "Example:\n`atmos workflow deploy-infra --file workflow1`",
			})
			if err != nil {
				u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			}
			return
		}

		// Execute the workflow command
		err := e.ExecuteWorkflowCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			// Format common error messages
			if strings.Contains(err.Error(), "does not exist") {
				err := renderError(ErrorMessage{
					Title:      "Workflow File Not Found",
					Details:    fmt.Sprintf("The workflow manifest file '%s' could not be found.", workflowFile),
					Suggestion: "Check if the file exists in the workflows directory and the path is correct.",
				})
				if err != nil {
					u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
				}
			} else if strings.Contains(err.Error(), "does not have the") {
				err := renderError(ErrorMessage{
					Title:      "Invalid Workflow",
					Details:    err.Error(),
					Suggestion: fmt.Sprintf("Check the available workflows in '%s' and make sure you're using the correct workflow name.", workflowFile),
				})
				if err != nil {
					u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
				}
			} else {
				// For other errors, use the standard error handler
				u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			}
			return
		}
	},
}

func init() {
	workflowCmd.DisableFlagParsing = false
	workflowCmd.PersistentFlags().StringP("file", "f", "", "atmos workflow <name> --file <file>")
	workflowCmd.PersistentFlags().Bool("dry-run", false, "atmos workflow <name> --file <file> --dry-run")
	workflowCmd.PersistentFlags().StringP("stack", "s", "", "atmos workflow <name> --file <file> --stack <stack>")
	workflowCmd.PersistentFlags().String("from-step", "", "atmos workflow <name> --file <file> --from-step <step-name>")

	RootCmd.AddCommand(workflowCmd)
}
