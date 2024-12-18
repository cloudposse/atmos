package cmd

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	u "github.com/cloudposse/atmos/pkg/utils"
)

//go:embed markdown/workflow.md
var workflowMarkdown string

// ErrorMessage represents a structured error message
type ErrorMessage struct {
	Title      string
	Details    string
	Suggestion string
}

// renderError renders an error message using the markdown renderer
func renderError(msg ErrorMessage) error {
	renderer, err := markdown.NewRenderer(
		markdown.WithWidth(80),
	)
	if err != nil {
		return fmt.Errorf("failed to create markdown renderer: %w", err)
	}

	rendered, err := renderer.RenderError(msg.Title, msg.Details, msg.Suggestion)
	if err != nil {
		return fmt.Errorf("failed to render error message: %w", err)
	}

	fmt.Print(rendered)
	return nil
}

// getMarkdownSection returns a section from the markdown file
func getMarkdownSection(title string) (details, suggestion string) {
	sections := markdown.ParseMarkdownSections(workflowMarkdown)
	if section, ok := sections[title]; ok {
		parts := markdown.SplitMarkdownContent(section)
		if len(parts) >= 2 {
			return parts[0], parts[1]
		}
		return section, ""
	}
	return "", ""
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
			details, suggestion := getMarkdownSection("Invalid Command")
			err := renderError(ErrorMessage{
				Title:      "Invalid Command",
				Details:    details,
				Suggestion: suggestion,
			})
			if err != nil {
				u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			}
			return
		}

		// Get the --file flag value
		workflowFile, _ := cmd.Flags().GetString("file")
		if workflowFile == "" {
			details, suggestion := getMarkdownSection("Missing Required Flag")
			err := renderError(ErrorMessage{
				Title:      "Missing Required Flag",
				Details:    details,
				Suggestion: suggestion,
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
				details, suggestion := getMarkdownSection("Workflow File Not Found")
				err := renderError(ErrorMessage{
					Title:      "Workflow File Not Found",
					Details:    details,
					Suggestion: suggestion,
				})
				if err != nil {
					u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
				}
			} else if strings.Contains(err.Error(), "does not have the") {
				details, suggestion := getMarkdownSection("Invalid Workflow")
				err := renderError(ErrorMessage{
					Title:      "Invalid Workflow",
					Details:    details,
					Suggestion: suggestion,
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
