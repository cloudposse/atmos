package permission

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// CLIPrompter implements Prompter using command-line prompts.
type CLIPrompter struct{}

// NewCLIPrompter creates a new CLI prompter.
func NewCLIPrompter() *CLIPrompter {
	return &CLIPrompter{}
}

// Prompt asks the user for permission via CLI.
func (p *CLIPrompter) Prompt(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	fmt.Fprintf(os.Stderr, "\n🔧 Tool Execution Request\n")
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "Tool: %s\n", tool.Name())
	fmt.Fprintf(os.Stderr, "Description: %s\n", tool.Description())

	if len(params) > 0 {
		fmt.Fprintf(os.Stderr, "\nParameters:\n")
		for key, value := range params {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", key, value)
		}
	}

	fmt.Fprintf(os.Stderr, "\nAllow execution? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}
