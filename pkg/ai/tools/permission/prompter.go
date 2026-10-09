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

// checkCachedForTool resolves the cached decision for a tool. Command-scoped tools
// (ScopedTool) are looked up by their exact CacheKey; all other tools by name using
// the legacy matching rules.
func (p *CLIPrompter) checkCachedForTool(tool Tool) (bool, bool) {
	scoped, ok := tool.(ScopedTool)
	if !ok {
		return p.checkCachedPermission(tool.Name())
	}
	if p.cache == nil {
		return false, false
	}
	key := scoped.CacheKey()
	if p.cache.IsAllowedExact(key) {
		return true, true
	}
	if p.cache.IsDeniedExact(key) {
		return false, true
	}
	return false, false
}

// HasCachedDecision reports whether a stored allow/deny decision answers for the
// tool, in which case Prompt returns without showing anything.
func (p *CLIPrompter) HasCachedDecision(tool Tool) bool {
	_, found := p.checkCachedForTool(tool)
	return found
}

// cacheKeyFor returns the key under which decisions for the tool are stored.
func cacheKeyFor(tool Tool) string {
	if scoped, ok := tool.(ScopedTool); ok {
		return scoped.CacheKey()
	}
	return tool.Name()
}

// settingsPathHint names the file where "always" decisions are stored.
const settingsPathHint = ".atmos/ai.settings.local.json"

// Receipt verbs printed after the user decides.
const (
	receiptAllowed = "Allowed"
	receiptDenied  = "Denied"
	// The receiptGutter constant is the number of columns reserved for the status icon and its space.
	receiptGutter = 2
)

// handleCachedResponse processes a response when cache is available. It reports whether
// the request is allowed and whether the decision was persisted to the cache.
func (p *CLIPrompter) handleCachedResponse(response, toolName string) (allowed, saved bool) {
	switch response {
	case "a", "always":
		return true, p.persist(p.cache.AddAllow(toolName))
	case "y", "yes":
		return true, false
	case "d", "deny":
		return false, p.persist(p.cache.AddDeny(toolName))
	default:
		return false, false
	}
}

// persist reports whether a cache write succeeded, warning the user when it did not.
func (p *CLIPrompter) persist(err error) bool {
	if err != nil {
		ui.Warningf("Failed to save permission: %v", err)
		return false
	}
	return true
}

// printReceipt prints one status line recording the decision, followed by a blank
// line that separates it from whatever the AI prints next.
func printReceipt(tool Tool, params map[string]interface{}, allowed, saved bool) {
	verb := receiptDenied
	if allowed {
		verb = receiptAllowed
	}
	suffix := ""
	if saved {
		suffix = " (saved to " + settingsPathHint + ")"
	}
	budget := requestWidth() + formGutter - receiptGutter - len(verb) - len(" ") - len(suffix)
	line := verb + " " + summarizeRequest(tool, params, budget) + suffix

	if allowed {
		ui.Success(line)
	} else {
		ui.Warning(line)
	}
	ui.Writeln("")
}

// Prompt asks the user for permission via CLI.
func (p *CLIPrompter) Prompt(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	if decision, found := p.checkCachedForTool(tool); found {
		return decision, nil
	}

	// Prompts require a TTY; fail loudly instead of silently defaulting to deny.
	// Checked before anything is printed so non-interactive logs stay free of a dangling request.
	if !terminal.New().IsTTY(terminal.Stdin) {
		return false, errUtils.ErrInteractiveNotAvailable
	}

	if p.cache != nil {
		return p.promptWithCache(tool, params)
	}

	return p.promptWithoutCache(tool, params)
}

// requestNote builds the note field that shows the request inside the form, so huh
// erases it together with the choices once the user answers.
func requestNote(tool Tool, params map[string]interface{}) *huh.Note {
	title, body := renderRequestParts(tool, params, requestWidth())
	return huh.NewNote().Title(title).Description(escapeNoteMarkup(body))
}

// requestTheme returns the Atmos huh theme tuned for the request block: the note title
// sits flush above its body, and the vertical breathing room is owned by the note card
// (above the block) rather than doubled up by the choices below it.
func requestTheme() *huh.Theme {
	t := uiutils.NewAtmosHuhTheme()
	for _, styles := range []*huh.FieldStyles{&t.Focused, &t.Blurred} {
		styles.NoteTitle = styles.NoteTitle.MarginBottom(0)
		styles.Card = styles.Card.MarginTop(1)
		styles.Base = styles.Base.MarginTop(0)
	}
	return t
}

// runRequestForm runs the form and maps huh errors onto Atmos errors.
func runRequestForm(form *huh.Form) error {
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errUtils.ErrUserAborted
		}
		return fmt.Errorf("%w: %w", errUtils.ErrAIPermissionPromptFailed, err)
	}
	return nil
}

// alwaysAllowLabel names the "always allow" choice for the tool, saying what it covers.
func alwaysAllowLabel(tool Tool) string {
	if _, scoped := tool.(ScopedTool); scoped {
		return "Always allow this exact request"
	}
	return "Always allow " + prettyToolName(tool.Name())
}

// alwaysDenyLabel names the "always deny" choice for the tool, saying what it covers.
func alwaysDenyLabel(tool Tool) string {
	if _, scoped := tool.(ScopedTool); scoped {
		return "Always deny this exact request"
	}
	return "Always deny " + prettyToolName(tool.Name())
}

// promptWithCache presents the four cached-permission choices via a huh select
// (safest first) and dispatches the selection through handleCachedResponse.
func (p *CLIPrompter) promptWithCache(tool Tool, params map[string]interface{}) (bool, error) {
	response := choiceAllowOnce

	form := huh.NewForm(
		huh.NewGroup(
			requestNote(tool, params),
			huh.NewSelect[string]().
				Title("Allow execution?").
				Description("\"Always\" choices are saved to "+settingsPathHint).
				Options(
					huh.NewOption("Allow once", choiceAllowOnce),
					huh.NewOption(alwaysAllowLabel(tool), choiceAlwaysAllow),
					huh.NewOption("Deny once", choiceDenyOnce),
					huh.NewOption(alwaysDenyLabel(tool), choiceAlwaysDeny),
				).
				Value(&response),
		),
	).WithTheme(requestTheme())

	if err := runRequestForm(form); err != nil {
		return false, err
	}

	allowed, saved := p.handleCachedResponse(response, cacheKeyFor(tool))
	printReceipt(tool, params, allowed, saved)
	return allowed, nil
}

// promptWithoutCache presents a simple allow/deny confirmation via huh.
func (p *CLIPrompter) promptWithoutCache(tool Tool, params map[string]interface{}) (bool, error) {
	allowed := true

	form := huh.NewForm(
		huh.NewGroup(
			requestNote(tool, params),
			uiutils.NewAtmosConfirm().
				Title("Allow execution?").
				Affirmative("Allow").
				Negative("Deny").
				Value(&allowed),
		),
	).WithTheme(requestTheme())

	if err := runRequestForm(form); err != nil {
		return false, err
	}

	printReceipt(tool, params, allowed, false)
	return allowed, nil
}
