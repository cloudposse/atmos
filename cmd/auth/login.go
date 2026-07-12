package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// loginParser handles flags for the login command.
var loginParser *flags.StandardParser

// authLoginCmd logs in using a configured identity.
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate using a configured identity",
	Long:  "Authenticate to cloud providers using an identity defined in `atmos.yaml`.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE:               executeAuthLoginCommand,
}

func init() {
	defer perf.Track(nil, "auth.login.init")()

	// Create parser with login-specific flags.
	loginParser = flags.NewStandardParser(
		flags.WithStringFlag("provider", "p", "", "Provider name to authenticate with (for SSO auto-provisioning)"),
		flags.WithStringFlag("tags", "", "", "Select an identity by tags (comma-separated, matches any): --tags=production,admin"),
	)

	// Register flags with the command.
	loginParser.RegisterFlags(authLoginCmd)

	// Bind to Viper for environment variable support.
	if err := loginParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	authCmd.AddCommand(authLoginCmd)
}

func executeAuthLoginCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	// Bind parsed flags to Viper for precedence (must be done before parsing global flags).
	v := viper.GetViper()
	if err := loginParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "auth.executeAuthLoginCommand")()

	// Create auth manager.
	authManager, err := CreateAuthManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Check if --provider flag was provided.
	providerName := v.GetString("provider")

	// Perform authentication based on whether provider or identity was specified.
	ctx := context.Background()
	var whoami *authTypes.WhoamiInfo

	if providerName != "" {
		// Provider-level authentication (e.g., for SSO auto-provisioning).
		whoami, err = authManager.AuthenticateProvider(ctx, providerName)
		if err != nil {
			return fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, providerName, err)
		}
	} else {
		// Try identity-level authentication first.
		var needsProviderFallback bool
		whoami, needsProviderFallback, err = authenticateIdentity(ctx, cmd, authManager)

		if needsProviderFallback {
			// No identities available - fall back to provider authentication.
			// This enables seamless first-login with auto_provision_identities.
			providerName, err = getProviderForFallback(authManager)
			if err != nil {
				return maybeOfferProfileFallbackOnAuthConfigError(ctx, authManager, err)
			}
			whoami, err = authManager.AuthenticateProvider(ctx, providerName)
			if err != nil {
				return fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, providerName, err)
			}
		} else if err != nil {
			return maybeOfferProfileFallbackOnAuthConfigError(ctx, authManager, err)
		}
	}

	// Display success message using Atmos theme.
	displayAuthSuccess(whoami)

	return nil
}

// authenticateIdentity handles identity-level authentication with default and interactive selection.
// Returns (WhoamiInfo, needsProviderFallback, error) where needsProviderFallback indicates whether
// to fall back to provider-level authentication (when no identities are available).
func authenticateIdentity(ctx context.Context, cmd *cobra.Command, authManager auth.AuthManager) (*authTypes.WhoamiInfo, bool, error) {
	defer perf.Track(nil, "auth.authenticateIdentity")()

	// Get identity from flag or use default.
	// Use centralized function that handles Cobra's NoOptDefVal quirk correctly.
	identityName := GetIdentityFromFlags(cmd)

	// If flag wasn't provided, check Viper for env var fallback.
	if identityName == "" {
		identityName = viper.GetString(IdentityFlagName)
	}

	// Check if user wants to interactively select identity.
	forceSelect := identityName == IdentityFlagSelectValue

	// If --tags was provided (and no explicit --identity), select by tag match instead
	// of falling through to the default-identity resolution below.
	if identityName == "" {
		if filterTags := parseCommaSeparatedNames(viper.GetString(tagsKey)); len(filterTags) > 0 {
			selected, err := selectIdentityByTags(authManager, filterTags)
			if err != nil {
				return nil, false, err
			}
			identityName = selected
		}
	}

	// If no identity specified, get the default identity (which prompts if needed).
	// If --identity flag was used without value, forceSelect will be true.
	if identityName == "" || forceSelect {
		var err error
		identityName, err = authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			// Check if we should fall back to provider-based auth.
			// This happens when no identities are available (e.g., first login with auto_provision_identities).
			if errors.Is(err, errUtils.ErrNoIdentitiesAvailable) ||
				errors.Is(err, errUtils.ErrNoDefaultIdentity) {
				return nil, true, nil
			}
			return nil, false, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDefaultIdentity, err)
		}
	}

	// Perform identity authentication.
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		// User explicitly cancelled (Ctrl+C/ESC) — surface a clean abort
		// without wrapping in ErrAuthenticationFailed.
		if errors.Is(err, errUtils.ErrUserAborted) {
			return nil, false, errUtils.ErrUserAborted
		}
		return nil, false, fmt.Errorf("%w: identity=%s: %w", errUtils.ErrAuthenticationFailed, identityName, err)
	}

	return whoami, false, nil
}

// providerLister is an interface for listing providers (subset of auth.AuthManager).
type providerLister interface {
	ListProviders() []string
}

// isInteractive checks if we're running in an interactive terminal.
// Interactive mode requires stdin to be a TTY (for user input) and must not be in CI.
func isInteractive() bool {
	return term.IsTTYSupportForStdin() && !telemetry.IsCI()
}

// isInteractiveFn indirects through isInteractive so tests can force the
// non-interactive branch of getProviderForFallback deterministically — running
// the test from a real TTY would otherwise trip into promptForProvider which
// blocks on stdin. Production callers should never reassign this.
var isInteractiveFn = isInteractive

// getProviderForFallback determines which provider to use when no identities are configured.
// If only one provider exists, it is auto-selected.
// If multiple providers exist and interactive, prompts user.
// If multiple providers exist and non-interactive, returns error with helpful message.
func getProviderForFallback(authManager providerLister) (string, error) {
	defer perf.Track(nil, "auth.getProviderForFallback")()

	providers := authManager.ListProviders()

	if len(providers) == 0 {
		return "", errUtils.ErrNoProvidersAvailable
	}

	// Auto-select if only one provider.
	if len(providers) == 1 {
		return providers[0], nil
	}

	// Multiple providers - need interactive selection or error.
	if !isInteractiveFn() {
		return "", fmt.Errorf("%w: use --provider flag to specify which provider", errUtils.ErrNoDefaultProvider)
	}

	return promptForProvider("No identities configured. Select a provider:", providers)
}

// selectIdentityByTags filters authManager's identities by filterTags (any match) and
// resolves a single identity name: 0 matches is an error listing available tags, 1 match
// is returned directly, and multiple matches trigger an interactive picker (or an error
// in non-interactive contexts).
func selectIdentityByTags(authManager auth.AuthManager, filterTags []string) (string, error) {
	defer perf.Track(nil, "auth.selectIdentityByTags")()

	identities := authManager.GetIdentities()

	matches := make([]string, 0)
	for name := range identities {
		if tags.MatchesTags(identities[name].Tags, filterTags, tags.TagModeAny) {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)

	switch len(matches) {
	case 0:
		return "", buildNoIdentitiesMatchTagsError(identities, filterTags)
	case 1:
		return matches[0], nil
	default:
		if !isInteractiveFn() {
			return "", errUtils.ErrIdentitySelectionRequiresTTY
		}
		return promptForIdentity(fmt.Sprintf("Multiple identities match tags %v. Select one:", filterTags), matches)
	}
}

// buildNoIdentitiesMatchTagsError returns a helpful error listing tags that do exist,
// so the user can retry with a valid tag.
func buildNoIdentitiesMatchTagsError(identities map[string]schema.Identity, filterTags []string) error {
	tagSet := make(map[string]struct{})
	for name := range identities {
		for _, tag := range identities[name].Tags {
			tagSet[tag] = struct{}{}
		}
	}

	availableTags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		availableTags = append(availableTags, tag)
	}
	sort.Strings(availableTags)

	err := errUtils.Build(errUtils.ErrNoIdentitiesMatchTags).
		WithExplanationf("No identities match tags %v", filterTags)
	if len(availableTags) > 0 {
		err = err.WithHintf("Available tags: `%s`", strings.Join(availableTags, "`, `"))
	}
	return err.Err()
}

// promptForIdentity prompts the user to select an identity from the given list.
// Mirrors promptForProvider exactly for UI consistency.
func promptForIdentity(message string, identities []string) (string, error) {
	defer perf.Track(nil, "auth.promptForIdentity")()

	if len(identities) == 0 {
		return "", errUtils.ErrNoIdentitiesAvailable
	}

	var selectedIdentity string

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Description("Press ctrl+c or esc to exit").
				Options(huh.NewOptions(identities...)...).
				Value(&selectedIdentity),
		),
	).WithKeyMap(keyMap)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf("%w: %w", errUtils.ErrUnsupportedInputType, err)
	}

	return selectedIdentity, nil
}

// promptForProvider prompts the user to select a provider from the given list.
func promptForProvider(message string, providers []string) (string, error) {
	defer perf.Track(nil, "auth.promptForProvider")()

	if len(providers) == 0 {
		return "", errUtils.ErrNoProvidersAvailable
	}

	// Sort providers alphabetically for consistent ordering.
	sortedProviders := make([]string, len(providers))
	copy(sortedProviders, providers)
	sort.Strings(sortedProviders)

	var selectedProvider string

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Description("Press ctrl+c or esc to exit").
				Options(huh.NewOptions(sortedProviders...)...).
				Value(&selectedProvider),
		),
	).WithKeyMap(keyMap)

	if err := form.Run(); err != nil {
		// Check if user aborted (Ctrl+C, ESC, etc.).
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf("%w: %w", errUtils.ErrUnsupportedInputType, err)
	}

	return selectedProvider, nil
}
