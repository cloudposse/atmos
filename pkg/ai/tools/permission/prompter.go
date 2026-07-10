package permission

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/ui"
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
	ui.Writef("\n🔧 Tool Execution Request\n")
	ui.Writef("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ui.Writef("Tool: %s\n", tool.Name())
	ui.Writef("Description: %s\n", tool.Description())

	if len(params) > 0 {
		ui.Writef("\nParameters:\n")
		for key, value := range params {
			ui.Writef("  %s: %v\n", key, value)
		}
	}

	if p.cache != nil {
		ui.Writef("\nOptions:\n")
		ui.Writef("  [a] Always allow (save to .atmos/ai.settings.local.json)\n")
		ui.Writef("  [y] Allow once\n")
		ui.Writef("  [n] Deny once\n")
		ui.Writef("  [d] Always deny (save to .atmos/ai.settings.local.json)\n")
		ui.Write("\nChoice (a/y/n/d): ")
	} else {
		ui.Write("\nAllow execution? (y/N): ")
	}
}

// handleCachedResponse processes a response when cache is available.
func (p *CLIPrompter) handleCachedResponse(response, toolName string) bool {
	switch response {
	case "a", "always":
		if err := p.cache.AddAllow(toolName); err != nil {
			ui.Warningf("Failed to save permission: %v", err)
		} else {
			ui.Successf("Permission saved to .atmos/ai.settings.local.json")
		}
		return true
	case "y", "yes":
		return true
	case "d", "deny":
		if err := p.cache.AddDeny(toolName); err != nil {
			ui.Warningf("Failed to save permission: %v", err)
		} else {
			ui.Successf("Permission saved to .atmos/ai.settings.local.json")
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
