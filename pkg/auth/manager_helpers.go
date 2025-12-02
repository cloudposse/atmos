package auth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// autoDetectDefaultIdentity attempts to find and return a default identity from configuration.
// Returns empty string if no default identity is found (not an error condition).
// If multiple defaults exist and allowInteractive is true, prompts user to select.
func autoDetectDefaultIdentity(authConfig *schema.AuthConfig) (string, error) {
	// Create a temporary manager to call GetDefaultIdentity which handles:
	// - Global defaults from atmos.yaml
	// - Stack-level defaults from stack configs
	// - Multiple defaults (prompts in interactive mode, errors in CI)
	// - No defaults (returns error which we handle gracefully)
	tempStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	tempManager, err := NewAuthManager(authConfig, credStore, validator, tempStackInfo)
	if err != nil {
		return "", errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Try to get default identity using GetDefaultIdentity(forceSelect=false).
	//
	// Behavior depends on terminal mode (detected by isInteractive()):
	//
	// Interactive mode (TTY available):
	//   - Exactly one default: Returns the default identity
	//   - Multiple defaults: Prompts user to choose from ONLY the defaults
	//   - No defaults: Prompts user to choose from ALL identities
	//
	// Non-interactive mode (CI/no TTY):
	//   - Exactly one default: Returns the default identity
	//   - Multiple defaults: Returns error (can't prompt)
	//   - No defaults: Returns error (can't prompt)
	//
	// Note: We call GetDefaultIdentity(false) NOT GetDefaultIdentity(true) because:
	// - forceSelect=false makes it aware of default identities
	// - forceSelect=true would always show ALL identities, ignoring defaults
	// - When multiple defaults exist, we want to show ONLY those defaults
	defaultIdentity, err := tempManager.GetDefaultIdentity(false)
	if err != nil {
		// Special case: If user explicitly aborted (Ctrl+C), propagate the error immediately.
		// This allows the caller to exit cleanly without continuing execution.
		if errors.Is(err, errUtils.ErrUserAborted) {
			return "", err
		}

		// For other errors (no default identity in CI mode, etc.), return empty string.
		// This maintains backward compatibility where no authentication is performed
		// when a default identity cannot be determined automatically.
		// We intentionally return nil error here to maintain backward compatibility.
		return "", nil
	}

	return defaultIdentity, nil
}

// shouldDisableAuth checks if authentication is explicitly disabled.
func shouldDisableAuth(identityName string) bool {
	return identityName == cfg.IdentityFlagDisabledValue
}

// isAuthConfigured checks if auth configuration is present and has identities.
func isAuthConfigured(authConfig *schema.AuthConfig) bool {
	return authConfig != nil && len(authConfig.Identities) > 0
}

// resolveIdentityName resolves the final identity name to use for authentication.
// Returns empty string if no authentication should be performed.
// Returns error if identity resolution fails.
func resolveIdentityName(identityName string, authConfig *schema.AuthConfig) (string, error) {
	// If identity already specified (not empty, not disabled), use it as-is.
	if identityName != "" && identityName != cfg.IdentityFlagDisabledValue {
		return identityName, nil
	}

	// If no auth configured, return empty (no authentication).
	if !isAuthConfigured(authConfig) {
		return "", nil
	}

	// Try to auto-detect default identity.
	defaultIdentity, err := autoDetectDefaultIdentity(authConfig)
	if err != nil {
		return "", err
	}

	return defaultIdentity, nil
}

// createAuthManagerInstance creates a new AuthManager instance with the given configuration.
func createAuthManagerInstance(authConfig *schema.AuthConfig) (AuthManager, error) {
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := NewAuthManager(authConfig, credStore, validator, authStackInfo)
	if err != nil {
		return nil, errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	return authManager, nil
}

// authenticateWithIdentity authenticates using the provided identity name.
// Handles interactive selection if identity matches selectValue.
func authenticateWithIdentity(authManager AuthManager, identityName string, selectValue string) error {
	// Handle interactive selection if identity matches the select value.
	forceSelect := identityName == selectValue
	if forceSelect {
		selectedIdentity, err := authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return err
		}
		identityName = selectedIdentity
	}

	// Authenticate to populate AuthContext with credentials.
	_, err := authManager.Authenticate(context.Background(), identityName)
	return err
}

// CreateAndAuthenticateManager creates and authenticates an AuthManager from an identity name.
// If identityName is empty, attempts to auto-detect a default identity from configuration.
// Returns nil AuthManager only if no identity is specified AND no default identity is configured,
// or if authentication is explicitly disabled.
// Returns error if authentication fails or if identity is specified but auth is not configured.
//
// This helper is used by both CLI commands and internal execution logic to ensure
// consistent authentication behavior across the codebase.
//
// Identity resolution behavior:
//   - If identityName is cfg.IdentityFlagDisabledValue ("__DISABLED__"), returns nil (authentication explicitly disabled)
//   - If identityName is empty and no auth configured, returns nil (no authentication)
//   - If identityName is empty and auth configured, attempts auto-detection of default identity
//   - If identityName is selectValue ("__SELECT__"), prompts for identity selection
//   - Otherwise, uses the provided identityName
//
// Auto-detection behavior when identityName is empty:
//   - If auth is not configured (no identities), returns nil (no authentication)
//   - If auth is configured, checks for default identity in both global atmos.yaml and stack configs
//   - If exactly ONE default identity exists, authenticates with it automatically
//   - If MULTIPLE defaults exist:
//   - Interactive mode (TTY): prompts user to select one from ONLY the defaults
//   - Non-interactive mode (CI): returns nil (no authentication)
//   - If NO defaults exist:
//   - Interactive mode: prompts user to select from all available identities
//   - Non-interactive mode (CI): returns nil (no authentication)
//
// Interactive selection behavior:
//   - When triggered (via selectValue OR no defaults in interactive mode), prompts user ONCE
//   - Selected identity is cached in AuthManager for the entire command execution
//   - All YAML functions use the same selected identity (no repeated prompts)
//
// Parameters:
//   - identityName: The identity to authenticate (can be "__SELECT__" for interactive selection,
//     "__DISABLED__" to disable auth, or empty for auto-detection)
//   - authConfig: The auth configuration from atmos.yaml and stack configs
//   - selectValue: The special value that triggers interactive identity selection (typically "__SELECT__")
//
// Returns:
//   - AuthManager with populated AuthContext after successful authentication
//   - nil if authentication disabled, no identity specified, or no default identity configured (in CI mode)
//   - error if authentication fails or auth is not configured when identity is specified
//
// Note: This function does not scan stack configs for default identities.
// Use CreateAndAuthenticateManagerWithAtmosConfig if you need stack-level default identity resolution.
func CreateAndAuthenticateManager(
	identityName string,
	authConfig *schema.AuthConfig,
	selectValue string,
) (AuthManager, error) {
	// Delegate to the full implementation with nil atmosConfig.
	// This maintains backward compatibility while allowing stack auth scanning when atmosConfig is provided.
	return CreateAndAuthenticateManagerWithAtmosConfig(identityName, authConfig, selectValue, nil)
}

// CreateAndAuthenticateManagerWithAtmosConfig creates and authenticates an AuthManager from an identity name.
// This is the full implementation that supports scanning stack configs for default identities.
//
// When atmosConfig is provided and identityName is empty:
//   - Scans stack configuration files for auth identity defaults
//   - Merges stack-level defaults with atmos.yaml defaults
//   - Stack defaults have lower priority than atmos.yaml defaults
//
// This solves the chicken-and-egg problem where:
//   - We need to know the default identity to authenticate
//   - But stack configs are only loaded after authentication is configured
//   - Stack-level defaults (auth.identities.*.default: true) would otherwise be ignored
//
// Parameters:
//   - identityName: The identity to authenticate (can be "__SELECT__" for interactive selection,
//     "__DISABLED__" to disable auth, or empty for auto-detection)
//   - authConfig: The auth configuration from atmos.yaml and stack configs
//   - selectValue: The special value that triggers interactive identity selection (typically "__SELECT__")
//   - atmosConfig: The full atmos configuration (optional, enables stack auth scanning)
//
// Returns:
//   - AuthManager with populated AuthContext after successful authentication
//   - nil if authentication disabled, no identity specified, or no default identity configured (in CI mode)
//   - error if authentication fails or auth is not configured when identity is specified
func CreateAndAuthenticateManagerWithAtmosConfig(
	identityName string,
	authConfig *schema.AuthConfig,
	selectValue string,
	atmosConfig *schema.AtmosConfiguration,
) (AuthManager, error) {
	defer perf.Track(atmosConfig, "auth.CreateAndAuthenticateManagerWithAtmosConfig")()

	log.Debug("CreateAndAuthenticateManager called", "identityName", identityName, "hasAuthConfig", authConfig != nil)

	// Check if authentication is explicitly disabled.
	if shouldDisableAuth(identityName) {
		log.Debug("Authentication explicitly disabled")
		return nil, nil
	}

	// If no identity specified and auth is configured, scan stack configs for defaults.
	// This solves the chicken-and-egg problem where stack-level defaults are not yet loaded.
	if identityName == "" && isAuthConfigured(authConfig) && atmosConfig != nil {
		scanAndMergeStackAuthDefaults(authConfig, atmosConfig)
	}

	// Resolve the identity name to use (may auto-detect or return empty).
	resolvedIdentity, err := resolveIdentityName(identityName, authConfig)
	if err != nil {
		return nil, err
	}

	// If no identity resolved, return nil (no authentication).
	if resolvedIdentity == "" {
		log.Debug("No identity resolved, returning nil")
		return nil, nil
	}

	// Validate auth is configured when we have an identity to use.
	if !isAuthConfigured(authConfig) {
		return nil, fmt.Errorf("%w: authentication requires at least one identity configured in atmos.yaml", errUtils.ErrAuthNotConfigured)
	}

	// Create AuthManager instance.
	authManager, err := createAuthManagerInstance(authConfig)
	if err != nil {
		return nil, err
	}

	// Authenticate with the resolved identity.
	if err := authenticateWithIdentity(authManager, resolvedIdentity, selectValue); err != nil {
		return nil, err
	}

	return authManager, nil
}

// scanAndMergeStackAuthDefaults scans stack configs for auth defaults and merges them into authConfig.
// This is a helper function that handles the stack auth scanning logic.
// Stack defaults take precedence over atmos.yaml defaults (following Atmos inheritance model).
func scanAndMergeStackAuthDefaults(authConfig *schema.AuthConfig, atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "auth.scanAndMergeStackAuthDefaults")()

	// Always scan stack configs - stack defaults take precedence over atmos.yaml.
	// This follows the Atmos inheritance model where more specific config overrides global.
	log.Debug("Scanning stack configs for auth identity defaults")
	stackDefaults, err := cfg.ScanStackAuthDefaults(atmosConfig)
	if err != nil {
		log.Debug("Failed to scan stack auth defaults", "error", err)
		return
	}

	if len(stackDefaults) == 0 {
		log.Debug("No default identities found in stack configs")
		return
	}

	// Merge stack defaults into auth config (stack takes precedence over atmos.yaml).
	cfg.MergeStackAuthDefaults(authConfig, stackDefaults)
	log.Debug("Merged stack auth defaults", "count", len(stackDefaults))
}
