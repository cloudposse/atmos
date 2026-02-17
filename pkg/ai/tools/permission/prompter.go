package permission

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// CLIPrompter implements Prompter using command-line prompts.
type CLIPrompter struct {
	cache *PermissionCache
}

// NewCLIPrompter creates a new CLI prompter.
func NewCLIPrompter() *CLIPrompter {
	return &CLIPrompter{}
}

// NewCLIPrompterWithCache creates a CLI prompter with persistent cache.
func NewCLIPrompterWithCache(cache *PermissionCache) *CLIPrompter {
	return &CLIPrompter{
		cache: cache,
	}
}

// Prompt asks the user for permission via CLI.
func (p *CLIPrompter) Prompt(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	// If cache is enabled, check if we have a cached decision.
	if p.cache != nil {
		if p.cache.IsAllowed(tool.Name()) {
			return true, nil
		}
		if p.cache.IsDenied(tool.Name()) {
			return false, nil
		}
	}

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

	// Offer options for persistent permissions if cache is available.
	if p.cache != nil {
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(os.Stderr, "  [a] Always allow (save to .atmos/ai.settings.local.json)\n")
		fmt.Fprintf(os.Stderr, "  [y] Allow once\n")
		fmt.Fprintf(os.Stderr, "  [n] Deny once\n")
		fmt.Fprintf(os.Stderr, "  [d] Always deny (save to .atmos/ai.settings.local.json)\n")
		fmt.Fprintf(os.Stderr, "\nChoice (a/y/n/d): ")
	} else {
		fmt.Fprintf(os.Stderr, "\nAllow execution? (y/N): ")
	}

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	// Handle persistent cache options.
	if p.cache != nil {
		switch response {
		case "a", "always":
			// Add to allow list.
			if err := p.cache.AddAllow(tool.Name()); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Warning: Failed to save permission: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "✅ Permission saved to .atmos/ai.settings.local.json\n")
			}
			return true, nil

		case "y", "yes":
			return true, nil

		case "n", "no", "":
			return false, nil

		case "d", "deny":
			// Add to deny list.
			if err := p.cache.AddDeny(tool.Name()); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Warning: Failed to save permission: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "✅ Permission saved to .atmos/ai.settings.local.json\n")
			}
			return false, nil

		default:
			return false, nil
		}
	}

	// Fallback for simple yes/no without cache.
	return response == "y" || response == "yes", nil
}
