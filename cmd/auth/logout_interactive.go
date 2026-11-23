package auth

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// logoutOption represents a logout choice in the interactive menu.
type logoutOption struct {
	label  string
	typ    string // "identity", "provider", "all"
	target string
}

// buildLogoutOptions creates the list of logout options from identities and providers.
// This is a pure function that can be easily tested. Identities and providers are
// each sorted by name so the menu order is the same on every run (Go map
// iteration order is random).
func buildLogoutOptions(identities map[string]schema.Identity, providers map[string]schema.Provider) []logoutOption {
	defer perf.Track(nil, "auth.buildLogoutOptions")()

	var options []logoutOption

	// Add identity options.
	for _, name := range sortedKeys(identities) {
		options = append(options, logoutOption{
			label:  fmt.Sprintf("Identity: %s", name),
			typ:    "identity",
			target: name,
		})
	}

	// Add provider options.
	for _, name := range sortedKeys(providers) {
		options = append(options, logoutOption{
			label:  fmt.Sprintf("Provider: %s (removes all identities)", name),
			typ:    "provider",
			target: name,
		})
	}

	// Add "all" option.
	options = append(options, logoutOption{
		label:  "All identities (complete logout)",
		typ:    "all",
		target: "",
	})

	return options
}

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// canonicalIdentityKey returns the configured identity key that matches name,
// exactly or else case-insensitively (identity keys are stored lower-cased while
// users may type the original casing), or name unchanged when nothing matches.
func canonicalIdentityKey(identities map[string]schema.Identity, name string) string {
	if _, exists := identities[name]; exists {
		return name
	}
	for _, key := range sortedKeys(identities) {
		if strings.EqualFold(key, name) {
			return key
		}
	}
	return name
}

// executeLogoutOption executes the selected logout action.
// This is a pure routing function that can be easily tested.
func executeLogoutOption(ctx context.Context, authManager auth.AuthManager, option logoutOption, dryRun bool, deleteKeychain bool, force bool) error {
	defer perf.Track(nil, "auth.executeLogoutOption")()

	switch option.typ {
	case "identity":
		return performIdentityLogout(ctx, authManager, option.target, dryRun, deleteKeychain, force)
	case "provider":
		return performProviderLogout(ctx, authManager, option.target, dryRun, deleteKeychain, force)
	case "all":
		return performLogoutAll(ctx, authManager, dryRun, deleteKeychain, force)
	default:
		return errUtils.ErrInvalidLogoutOption
	}
}

// performInteractiveLogout prompts user to choose what to logout.
// Without an interactive terminal (no TTY, CI, or --interactive=false) it fails
// instead of showing the menu: a menu that cannot be answered would read EOF and
// log out whichever option happened to come first.
func performInteractiveLogout(ctx context.Context, authManager auth.AuthManager, dryRun bool, deleteKeychain bool, force bool) error {
	defer perf.Track(nil, "auth.performInteractiveLogout")()

	identities := authManager.GetIdentities()
	providers := authManager.GetProviders()

	if len(identities) == 0 {
		ui.MarkdownMessagef("**No identities configured** in atmos.yaml\n")
		return nil
	}

	if !isInteractiveFn() {
		return logoutSelectionUnavailableError(identities)
	}

	// Build options list.
	options := buildLogoutOptions(identities, providers)

	// Create select options for huh.
	var huhOptions []huh.Option[logoutOption]
	for _, opt := range options {
		huhOptions = append(huhOptions, huh.NewOption(opt.label, opt))
	}

	var selected logoutOption
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[logoutOption]().
				Title("Choose what to logout from:").
				Options(huhOptions...).
				Value(&selected),
		),
	).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		return err
	}

	// Perform the selected logout action.
	return executeLogoutOption(ctx, authManager, selected, dryRun, deleteKeychain, force)
}

// logoutSelectionUnavailableError explains that `atmos auth logout` needs a
// selector that cannot be shown, and how to name the target instead.
func logoutSelectionUnavailableError(identities map[string]schema.Identity) error {
	return errUtils.Build(errUtils.ErrLogoutSelectionRequiresTTY).
		WithExplanation("`atmos auth logout` without a target asks what to log out of, but this session cannot show a prompt (it requires stdin and stderr to be terminals outside CI).").
		WithHint("Pass the identity to log out: `atmos auth logout <identity>` or `atmos auth logout --identity=<name>`").
		WithHint("Use `--provider=<name>`, `--tags=<tags>`, or `--all` to log out of more than one identity").
		WithContext("available_identities", joinSorted(identities)).
		Err()
}

// joinSorted returns the sorted keys of m joined with commas.
func joinSorted[V any](m map[string]V) string {
	return strings.Join(sortedKeys(m), ",")
}

// performSelectIdentityLogout handles a bare `--identity`: the user asked to pick
// an identity, so the identity selector is shown (not the full logout menu) and
// the chosen identity is logged out. Without a terminal the selector returns the
// same TTY-required error that other bare `--identity` commands return.
func performSelectIdentityLogout(ctx context.Context, authManager auth.AuthManager, dryRun bool, deleteKeychain bool, force bool) error {
	defer perf.Track(nil, "auth.performSelectIdentityLogout")()

	identityName, err := authManager.GetDefaultIdentity(true)
	if err != nil {
		return err
	}
	return performIdentityLogout(ctx, authManager, identityName, dryRun, deleteKeychain, force)
}
