package permission

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Cached-permission choice values, fed to the huh select and mapped by
// handleCachedResponse into an allow/deny decision.
const (
	choiceAlwaysAllow = "a"
	choiceAllowOnce   = "y"
	choiceDenyOnce    = "n"
	choiceAlwaysDeny  = "d"
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

	// Prompts require a TTY; fail loudly instead of silently defaulting to deny.
	if !terminal.New().IsTTY(terminal.Stdin) {
		return false, errUtils.ErrInteractiveNotAvailable
	}

	if p.cache != nil {
		return p.promptWithCache(tool.Name())
	}

	return p.promptWithoutCache()
}

// promptWithCache presents the four cached-permission choices via a huh select
// and dispatches the selection through handleCachedResponse.
func (p *CLIPrompter) promptWithCache(toolName string) (bool, error) {
	var response string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Allow execution?").
				Options(
					huh.NewOption("Always allow (save to .atmos/ai.settings.local.json)", choiceAlwaysAllow),
					huh.NewOption("Allow once", choiceAllowOnce),
					huh.NewOption("Deny once", choiceDenyOnce),
					huh.NewOption("Always deny (save to .atmos/ai.settings.local.json)", choiceAlwaysDeny),
				).
				Value(&response),
		),
	).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errUtils.ErrUserAborted
		}
		return false, fmt.Errorf("permission prompt failed: %w", err)
	}

	return p.handleCachedResponse(response, toolName), nil
}

// promptWithoutCache presents a simple allow/deny confirmation via huh.
func (p *CLIPrompter) promptWithoutCache() (bool, error) {
	var allowed bool

	confirm := uiutils.NewAtmosConfirm().
		Title("Allow execution?").
		Affirmative("Allow").
		Negative("Deny").
		Value(&allowed).
		WithTheme(uiutils.NewAtmosHuhTheme())

	if err := confirm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errUtils.ErrUserAborted
		}
		return false, fmt.Errorf("permission prompt failed: %w", err)
	}

	return allowed, nil
}
