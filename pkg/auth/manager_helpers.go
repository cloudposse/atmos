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
		// Error means we're in non-interactive mode and couldn't determine identity.
		// Return empty string (no authentication) to maintain backward compatibility.
		// This is intentional - we gracefully handle the error instead of propagating it.
		//nolint:nilerr // Intentionally returning nil to maintain backward compatibility
		return "", nil
	}

	return defaultIdentity, nil
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
//nolint:revive // Complexity is acceptable for authentication logic with auto-detection, validation, and error handling
func CreateAndAuthenticateManager(
	identityName string,
	authConfig *schema.AuthConfig,
	selectValue string,
) (AuthManager, error) {
	log.Debug("CreateAndAuthenticateManager called", "identityName", identityName, "hasAuthConfig", authConfig != nil)

	// Check if authentication is explicitly disabled via --identity=off/false/no/0.
	// This allows users to use external identity mechanisms (e.g., Leapp).
	if identityName == cfg.IdentityFlagDisabledValue {
		log.Debug("Authentication explicitly disabled")
		return nil, nil
	}

	// Auto-detect default identity if no identity name provided.
	if identityName == "" {
		log.Debug("No identity name provided, attempting auto-detection")
		// Return nil if auth is not configured at all (backward compatible).
		if authConfig == nil || len(authConfig.Identities) == 0 {
			return nil, nil
		}

		// Try to find default identity from configuration.
		// If multiple defaults exist or no defaults exist, will prompt in interactive mode.
		// Interactive mode is detected automatically inside autoDetectDefaultIdentity.
		defaultIdentity, err := autoDetectDefaultIdentity(authConfig)
		if err != nil {
			return nil, err
		}

		// If still no identity after auto-detection, return nil (no authentication).
		// This only happens in non-interactive mode when no defaults are configured.
		if defaultIdentity == "" {
			return nil, nil
		}

		// Found or selected default identity - use it for authentication.
		identityName = defaultIdentity
	}

	// Check if auth is configured when identity is provided (either explicitly or auto-detected).
	if authConfig == nil || len(authConfig.Identities) == 0 {
		return nil, fmt.Errorf("%w: authentication requires at least one identity configured in atmos.yaml", errUtils.ErrAuthNotConfigured)
	}

	// Create a ConfigAndStacksInfo for the auth manager to populate with AuthContext.
	// This enables YAML template functions to access authenticated credentials.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := NewAuthManager(authConfig, credStore, validator, authStackInfo)
	if err != nil {
		return nil, errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Handle interactive selection if identity matches the select value.
	forceSelect := identityName == selectValue
	if forceSelect {
		identityName, err = authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return nil, err
		}
	}

	// Authenticate to populate AuthContext with credentials.
	// This is critical for YAML functions like !terraform.state and !terraform.output
	// to access cloud resources with the proper credentials.
	_, err = authManager.Authenticate(context.Background(), identityName)
	if err != nil {
		return nil, err
	}

	return authManager, nil
}
