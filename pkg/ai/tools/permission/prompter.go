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

// checkCachedPermission checks if a cached permission decision exists.
// Returns the cached decision and true if found, or false if no cached decision exists.
func (p *CLIPrompter) checkCachedPermission(toolName string) (bool, bool) {
	if p.cache == nil {
		return false, false
	}
	if p.cache.IsAllowed(toolName) {
		return true, true
	}
	if p.cache.IsDenied(toolName) {
		return false, true
	}
	return false, false
}

// displayPrompt shows the tool execution request and prompt options.
func (p *CLIPrompter) displayPrompt(tool Tool, params map[string]interface{}) {
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
}

// handleCachedResponse processes a response when cache is available.
func (p *CLIPrompter) handleCachedResponse(response, toolName string) bool {
	switch response {
	case "a", "always":
		if err := p.cache.AddAllow(toolName); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Failed to save permission: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "✅ Permission saved to .atmos/ai.settings.local.json\n")
		}
		return true
	case "y", "yes":
		return true
	case "d", "deny":
		if err := p.cache.AddDeny(toolName); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Failed to save permission: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "✅ Permission saved to .atmos/ai.settings.local.json\n")
		}
		return false
	default:
		return false
	}
}

// Prompt asks the user for permission via CLI.
func (p *CLIPrompter) Prompt(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	if decision, found := p.checkCachedPermission(tool.Name()); found {
		return decision, nil
	}

	p.displayPrompt(tool, params)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	if p.cache != nil {
		return p.handleCachedResponse(response, tool.Name()), nil
	}

	return response == "y" || response == "yes", nil
}
