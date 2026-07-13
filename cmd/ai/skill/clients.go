package skill

import (
	"errors"
	"sort"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	term "github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

// scopeFlag is the shared --scope flag name, referenced from install.go,
// uninstall.go, and the scope-resolution helpers below.
const scopeFlag = "scope"

// resolveSkillClients resolves which AI clients `atmos ai skill install`
// should target (uninstall uses resolveUninstallClients instead, since it
// must consider every scope in play rather than one). An explicit
// --client/--all-clients flag always wins; otherwise this is "auto" mode:
// only ever act on what marketplace.DetectClients actually finds --
// interactively, that means pre-checking the detected clients in a picker
// the user can adjust; non-interactively (skipPrompt, no TTY, or CI), it
// means using exactly the detected list, which may be empty. Auto mode
// never silently falls back to every supported client just because nothing
// was detected -- that's what --all-clients is for. Mirrors
// cmd/mcp/client.resolveInstallClients.
func resolveSkillClients(basePath string, v *viper.Viper, skipPrompt bool, scope string) ([]string, error) {
	clients := v.GetStringSlice("client")
	if len(clients) > 0 {
		return clients, nil
	}
	if v.GetBool("all-clients") {
		return append([]string(nil), marketplace.SupportedClients...), nil
	}

	detected := marketplace.DetectClients(basePath, "", scope)
	if skipPrompt || !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		if len(detected) > 0 {
			ui.Infof("Auto-detected AI clients: %s", marketplace.BacktickJoin(detected))
		}
		return detected, nil
	}
	title := "Install skill into which clients?"
	if scope == marketplace.ScopeUser {
		title += " (user scope)"
	}
	return promptForSkillClients(detected, title)
}

// resolveSkillScope resolves the install/uninstall distribution scope
// (project versus user). An explicit --scope or --global flag always wins;
// otherwise, when skipPrompt is set or there's no real TTY/CI, it silently
// falls back to the flag's default value ("project"). Only when running
// interactively with none of those true does it prompt the user to choose.
// Mirrors cmd/mcp/client.resolveInstallScope; skipPrompt mirrors the same
// argument on resolveSkillClients: install.go passes its --yes value
// (--force there just means "reinstall", not "skip prompts"), uninstall.go
// passes its --force value (its only skip-prompt flag, since it has no
// separate --yes).
func resolveSkillScope(cmd *cobra.Command, v *viper.Viper, skipPrompt bool) (string, error) {
	if scope, ok := explicitSkillScope(cmd, v); ok {
		return scope, nil
	}
	if skipPrompt || !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		return v.GetString(scopeFlag), nil
	}

	title := "Install scope?"
	if cmd != nil && cmd.Name() == "uninstall" {
		title = "Uninstall scope?"
	}
	return promptForSkillScope(title)
}

// resolveUninstallScopes resolves which distribution scope(s) `atmos ai
// skill uninstall` should clean up client copies from. An explicit
// --scope/--global always wins and is used alone -- the user asked for
// exactly that scope. Otherwise (skipPrompt, no TTY, or CI) there is no
// reliable signal for which scope the skill was actually distributed to, and
// guessing a single one wrong for a destructive cleanup is unsafe: it can
// leave orphaned copies at the real scope, or -- worse -- a coincidental
// signal directory at the guessed scope (e.g. this repo's own project-level
// .claude/) can make cleanup target unrelated real files. So both project
// and user scope are checked; removeClientCopies no-ops for whichever scope
// nothing was actually distributed to. When running interactively (no
// force, real TTY, not CI), the existing single-choice scope picker still
// applies, since the user can just tell us which one they meant.
func resolveUninstallScopes(cmd *cobra.Command, v *viper.Viper, skipPrompt bool) ([]string, error) {
	if scope, ok := explicitSkillScope(cmd, v); ok {
		return []string{scope}, nil
	}
	if skipPrompt || !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		return []string{marketplace.ScopeProject, marketplace.ScopeUser}, nil
	}
	scope, err := promptForSkillScope("Uninstall scope?")
	if err != nil {
		return nil, err
	}
	return []string{scope}, nil
}

// resolveUninstallClients resolves which AI clients `atmos ai skill
// uninstall` should clean up distributed skill copies from, across every
// scope in scopes (see resolveUninstallScopes) -- an explicit
// --client/--all-clients flag still always wins, same as resolveSkillClients.
func resolveUninstallClients(basePath string, v *viper.Viper, skipPrompt bool, scopes []string) ([]string, error) {
	clients := v.GetStringSlice("client")
	if len(clients) > 0 {
		return clients, nil
	}
	if v.GetBool("all-clients") {
		return append([]string(nil), marketplace.SupportedClients...), nil
	}

	seen := make(map[string]bool)
	var detected []string
	for _, scope := range scopes {
		for _, client := range marketplace.DetectClients(basePath, "", scope) {
			if !seen[client] {
				seen[client] = true
				detected = append(detected, client)
			}
		}
	}

	if skipPrompt || !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		if len(detected) > 0 {
			ui.Infof("Auto-detected AI clients: %s", marketplace.BacktickJoin(detected))
		}
		return detected, nil
	}
	return promptForSkillClients(detected, "Remove skill from which clients?")
}

// explicitSkillScope returns the scope requested via an explicitly-set
// --scope or --global flag, and whether either was actually set (as opposed
// to just holding its zero-value default).
func explicitSkillScope(cmd *cobra.Command, v *viper.Viper) (string, bool) {
	if cmd == nil {
		return "", false
	}
	if cmd.Flags().Changed(scopeFlag) {
		return v.GetString(scopeFlag), true
	}
	if cmd.Flags().Changed("global") {
		if v.GetBool("global") {
			return marketplace.ScopeUser, true
		}
		return v.GetString(scopeFlag), true
	}
	return "", false
}

// promptForSkillScope shows an interactive single-choice picker for install/
// uninstall scope, mirroring promptForSkillClients's form setup and cancel
// handling but as a single-select rather than a multi-select (matching
// cmd/mcp/client.promptForScope).
func promptForSkillScope(title string) (string, error) {
	scope := marketplace.ScopeProject
	options := []huh.Option[string]{
		huh.NewOption("project (this repo only)", marketplace.ScopeProject),
		huh.NewOption("user (all your projects)", marketplace.ScopeUser),
	}
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "cancel"),
	)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Value(&scope),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", err
	}
	return scope, nil
}

// promptForSkillClients shows a huh.MultiSelect picker of every supported AI
// client, pre-checked with defaultClients (the auto-detected set).
func promptForSkillClients(defaultClients []string, title string) ([]string, error) {
	selected := append([]string(nil), defaultClients...)
	selectedByClient := make(map[string]bool, len(selected))
	for _, client := range selected {
		selectedByClient[client] = true
	}
	options := make([]huh.Option[string], 0, len(marketplace.SupportedClients))
	for _, client := range marketplace.SupportedClients {
		options = append(options, huh.NewOption(client, client).Selected(selectedByClient[client]))
	}
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "cancel"),
	)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(title).
				Description("Space toggles, enter confirms.").
				Options(options...).
				Value(&selected),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, err
	}
	sort.Strings(selected)
	return selected, nil
}
