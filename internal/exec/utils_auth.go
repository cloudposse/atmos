// utils_auth.go contains exec-layer orchestration for authentication.
//
// These functions live in internal/exec (not pkg/auth) because they depend on
// ExecuteDescribeComponent to fetch component-specific auth config from stack
// manifests before delegating to pkg/auth primitives. Moving them to pkg/auth
// would create a circular import (pkg/auth → internal/exec → pkg/auth).
//
// The actual auth logic (manager creation, identity resolution, credential
// storage) is implemented in pkg/auth. This file only orchestrates the
// component config discovery and merging that must happen in the exec layer.
package exec

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// componentConfigFetcher is a function type for fetching component configuration.
// This allows dependency injection for testing.
type componentConfigFetcher func(params *ExecuteDescribeComponentParams) (map[string]any, error)

// authManagerCreator is a function type for creating and authenticating an AuthManager.
// This allows dependency injection for testing.
type authManagerCreator func(identity string, authConfig *schema.AuthConfig, selectValue string, atmosConfig *schema.AtmosConfiguration) (auth.AuthManager, error)

// defaultComponentConfigFetcher is the default implementation that calls ExecuteDescribeComponent.
var defaultComponentConfigFetcher componentConfigFetcher = ExecuteDescribeComponent

// defaultAuthManagerCreator is the default implementation that calls auth.CreateAndAuthenticateManagerWithAtmosConfig.
var defaultAuthManagerCreator authManagerCreator = auth.CreateAndAuthenticateManagerWithAtmosConfig

// createAndAuthenticateAuthManager creates an AuthManager by merging global auth config with
// component-specific auth config, then authenticating using the identity from info.Identity.
// If authentication succeeds and identity was auto-detected, the resolved identity is stored
// back into info.Identity so subsequent operations (hooks, nested calls) can reuse it.
func createAndAuthenticateAuthManager(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	return createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, defaultComponentConfigFetcher, defaultAuthManagerCreator)
}

// createAndAuthenticateAuthManagerWithDeps is the implementation with injectable dependencies for testing.
func createAndAuthenticateAuthManagerWithDeps(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	configFetcher componentConfigFetcher,
	authCreator authManagerCreator,
) (auth.AuthManager, error) {
	defer perf.Track(atmosConfig, "exec.createAndAuthenticateAuthManager")()

	// Get merged auth config (global + component-specific if available).
	mergedAuthConfig, err := getMergedAuthConfigWithFetcher(atmosConfig, info, configFetcher)
	if err != nil {
		// Propagate known sentinel errors directly (e.g., ErrInvalidComponent) to preserve
		// their error message format. Only wrap unexpected errors with ErrInvalidAuthConfig.
		if errors.Is(err, errUtils.ErrInvalidComponent) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	// Create and authenticate AuthManager from --identity flag if specified.
	// Uses merged auth config that includes both global and component-specific identities/defaults.
	// This enables YAML template functions like !terraform.state to use authenticated credentials.
	authManager, err := authCreator(info.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// If AuthManager was created and identity was auto-detected (info.Identity was empty),
	// store the authenticated identity back into info.Identity so that hooks and nested
	// operations can reuse it without re-prompting for credentials.
	storeAutoDetectedIdentity(authManager, info)

	return authManager, nil
}

// getMergedAuthConfig merges global auth config with component-specific auth config.
// If stack and component are specified, it fetches component config and merges its auth section.
// Otherwise, returns a copy of the global auth config.
func getMergedAuthConfig(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
	return getMergedAuthConfigWithFetcher(atmosConfig, info, defaultComponentConfigFetcher)
}

// getMergedAuthConfigWithFetcher is the implementation with injectable component config fetcher for testing.
func getMergedAuthConfigWithFetcher(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	configFetcher componentConfigFetcher,
) (*schema.AuthConfig, error) {
	// Start with global auth config.
	mergedAuthConfig := auth.CopyGlobalAuthConfig(&atmosConfig.Auth)

	// If stack or component are missing, use global auth config only.
	if info.Stack == "" || info.ComponentFromArg == "" {
		return mergedAuthConfig, nil
	}

	// Get component configuration from stack.
	// Use nil AuthManager and disable functions to avoid circular dependency.
	componentConfig, err := configFetcher(&ExecuteDescribeComponentParams{
		Component:            info.ComponentFromArg,
		Stack:                info.Stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false, // Critical: avoid circular dependency with YAML functions that need auth.
		Skip:                 nil,
		AuthManager:          nil, // Critical: no AuthManager yet, we're determining which identity to use.
	})
	if err != nil {
		// If component doesn't exist, exit immediately before attempting authentication.
		// This prevents prompting for identity when the component is invalid.
		if errors.Is(err, errUtils.ErrInvalidComponent) {
			return nil, err
		}
		// For other errors (e.g., permission issues), continue with global auth config.
		log.Debug("Falling back to global auth config after component auth lookup error",
			"error", err, "stack", info.Stack, "component", info.ComponentFromArg)
		return mergedAuthConfig, nil
	}

	// Merge component-specific auth with global auth.
	return auth.MergeComponentAuthFromConfig(&atmosConfig.Auth, componentConfig, atmosConfig, cfg.AuthSectionName)
}

// storeAutoDetectedIdentity stores the authenticated identity from AuthManager back into
// info.Identity when it was auto-detected (empty). This allows hooks and nested operations
// to reuse the identity without re-prompting for credentials.
// The chain is ordered from base to final identity, so we take the last element.
func storeAutoDetectedIdentity(authManager auth.AuthManager, info *schema.ConfigAndStacksInfo) {
	if authManager == nil || info.Identity != "" {
		return
	}

	chain := authManager.GetChain()
	if len(chain) > 0 {
		info.Identity = chain[len(chain)-1]
		log.Debug("Stored authenticated identity", "identity", info.Identity)
	} else {
		log.Debug("Auth chain empty, identity not updated")
	}
}

// mergeGlobalAuthConfig deep-merges global auth config from atmosConfig into component section.
// Returns the merged auth section map. Also updates componentSection["auth"] to prevent
// postProcessTemplatesAndYamlFunctions from overwriting with empty auth.
func mergeGlobalAuthConfig(atmosConfig *schema.AtmosConfiguration, componentSection map[string]any) map[string]any {
	globalAuthSection := buildGlobalAuthSection(atmosConfig)
	componentAuthSection := getComponentAuthSection(componentSection)

	// If both are empty, return empty.
	if len(globalAuthSection) == 0 && len(componentAuthSection) == 0 {
		return map[string]any{}
	}

	// Deep-merge: global auth is base, component auth overrides.
	mergedAuth, err := m.Merge(atmosConfig, []map[string]any{globalAuthSection, componentAuthSection})
	if err != nil {
		return handleMergeError(componentSection, globalAuthSection, componentAuthSection)
	}

	// Update componentSection["auth"] so postProcessTemplatesAndYamlFunctions doesn't overwrite.
	componentSection[cfg.AuthSectionName] = mergedAuth
	return mergedAuth
}

// buildGlobalAuthSection builds the global auth section from atmosConfig.
func buildGlobalAuthSection(atmosConfig *schema.AtmosConfiguration) map[string]any {
	globalAuthSection := map[string]any{}

	if len(atmosConfig.Auth.Providers) > 0 {
		globalAuthSection["providers"] = atmosConfig.Auth.Providers
	}
	if len(atmosConfig.Auth.Identities) > 0 {
		globalAuthSection["identities"] = atmosConfig.Auth.Identities
	}
	if atmosConfig.Auth.Logs.Level != "" || atmosConfig.Auth.Logs.File != "" {
		globalAuthSection["logs"] = map[string]any{
			"level": atmosConfig.Auth.Logs.Level,
			"file":  atmosConfig.Auth.Logs.File,
		}
	}
	if atmosConfig.Auth.Keyring.Type != "" {
		globalAuthSection["keyring"] = atmosConfig.Auth.Keyring
	}

	return globalAuthSection
}

// getComponentAuthSection extracts the component's auth section (may be empty).
func getComponentAuthSection(componentSection map[string]any) map[string]any {
	if existingAuth, ok := componentSection[cfg.AuthSectionName].(map[string]any); ok {
		return existingAuth
	}
	return map[string]any{}
}

// handleMergeError handles merge failures by returning fallback auth config.
func handleMergeError(componentSection, globalAuthSection, componentAuthSection map[string]any) map[string]any {
	// If merge fails, return component auth as-is (defensive).
	if len(componentAuthSection) > 0 {
		componentSection[cfg.AuthSectionName] = componentAuthSection
		return componentAuthSection
	}
	// If no component auth, return global auth.
	if len(globalAuthSection) > 0 {
		componentSection[cfg.AuthSectionName] = globalAuthSection
		return globalAuthSection
	}
	return map[string]any{}
}
