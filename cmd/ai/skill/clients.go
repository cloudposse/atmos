package skill

import (
	"errors"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	term "github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

// resolveSkillClients resolves which AI clients `atmos ai skill install`/
// `uninstall` should target. An explicit --client/--all-clients flag always
// wins; otherwise this is "auto" mode: only ever act on what
// marketplace.DetectClients actually finds -- interactively, that means
// pre-checking the detected clients in a picker the user can adjust;
// non-interactively (skipPrompt, no TTY, or CI), it means using exactly the
// detected list, which may be empty. Auto mode never silently falls back to
// every supported client just because nothing was detected -- that's what
// --all-clients is for. Mirrors cmd/mcp/client.resolveInstallClients.
func resolveSkillClients(basePath string, v *viper.Viper, skipPrompt bool, title string) ([]string, error) {
	clients := v.GetStringSlice("client")
	if len(clients) > 0 {
		return clients, nil
	}
	if v.GetBool("all-clients") {
		return append([]string(nil), marketplace.SupportedClients...), nil
	}

	detected := marketplace.DetectClients(basePath)
	if skipPrompt || !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		if len(detected) > 0 {
			ui.Infof("Auto-detected AI clients: %s", strings.Join(detected, ", "))
		}
		return detected, nil
	}
	return promptForSkillClients(detected, title)
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
