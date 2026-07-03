package exec

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	logKeyComponent = "component"
	logKeyStack     = "stack"
)

// nestedAuthManagerCache memoizes per-component AuthManagers resolved for nested terraform.state /
// terraform.output references during one process. Many components in a stack reference other
// components that share a single identity; without this cache each distinct target re-runs the full
// auth cycle (credential writes, file locks, keyring rebuilds) inside resolveAuthManagerForNestedComponent.
// Keyed by parent chain + auth-section fingerprint (see buildComponentAuthCacheKey), a provably-safe
// proxy for "same identity" because identities are global and only referenced by components. Reset via
// ResetNestedAuthManagerCache (also cleared by ResetStateCache); never reset in production.
// See docs/fixes/2026-06-22-dedupe-nested-component-auth.md.
var nestedAuthManagerCache sync.Map

// ResetNestedAuthManagerCache clears the nested-component AuthManager cache.
// Exported for tests to guarantee isolation between test functions; never called in production.
func ResetNestedAuthManagerCache() {
	defer perf.Track(nil, "exec.ResetNestedAuthManagerCache")()

	nestedAuthManagerCache.Range(func(key, _ any) bool {
		nestedAuthManagerCache.Delete(key)
		return true
	})
}

// buildComponentAuthCacheKey keys a per-component AuthManager by the parent auth chain plus a
// deterministic JSON fingerprint of the component's auth section. Because identities are defined
// globally and only referenced by components, "same auth section" is a safe, provable proxy for
// "same identity" — it never merges components whose sections differ. The parent chain disambiguates
// an inherited identity from an auto-detected one. Returns cacheable=false when the section cannot be
// serialized (e.g. a channel/func value), so callers resolve without caching. Shared by
// describeStacksProcessor and the nested terraform.state path so the two cannot drift.
func buildComponentAuthCacheKey(parent auth.AuthManager, authSection map[string]any) (string, bool) {
	fingerprint, err := json.Marshal(authSection)
	if err != nil {
		return "", false
	}
	var parentChain string
	if parent != nil {
		parentChain = strings.Join(parent.GetChain(), ">")
	}
	return parentChain + "\x00" + string(fingerprint), true
}

// resolveCachedComponentAuthManager returns a memoized AuthManager for authSection when one was
// already resolved this process, otherwise it calls resolve and caches a successful, non-nil result.
// resolve is injected so tests can substitute a counting fake; production passes createComponentAuthManager.
func resolveCachedComponentAuthManager(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	component, stack string,
	parentAuthManager auth.AuthManager,
	authSection map[string]any,
	resolve componentAuthManagerResolver,
) (auth.AuthManager, error) {
	cacheKey, cacheable := buildComponentAuthCacheKey(parentAuthManager, authSection)
	if cacheable {
		if cached, ok := nestedAuthManagerCache.Load(cacheKey); ok {
			log.Debug(
				"Reusing cached component-specific AuthManager",
				logKeyComponent, component,
				logKeyStack, stack,
			)
			return cached.(auth.AuthManager), nil
		}
	}

	resolved, err := resolve(atmosConfig, componentConfig, component, stack, parentAuthManager)
	if err != nil {
		return resolved, err
	}
	if cacheable && resolved != nil {
		nestedAuthManagerCache.Store(cacheKey, resolved)
	}
	return resolved, nil
}

// hasDefaultIdentity checks if the auth section contains at least one identity with default: true.
func hasDefaultIdentity(authSection map[string]any) bool {
	identities, ok := authSection["identities"].(map[string]any)
	if !ok || identities == nil {
		return false
	}

	for _, identityConfig := range identities {
		identityMap, ok := identityConfig.(map[string]any)
		if !ok {
			continue
		}

		if defaultVal, ok := identityMap["default"].(bool); ok && defaultVal {
			return true
		}
	}

	return false
}

// resolveAuthManagerForNestedComponent determines which AuthManager to use for a nested component evaluation.
//
// Logic:
//  1. Gets component config WITHOUT processing templates/functions (to avoid circular dependency)
//  2. Checks if component has auth configuration defined
//  3. If component has auth config:
//     - Merges component auth with global auth
//     - Creates and authenticates a new AuthManager with merged config
//     - Returns the new AuthManager (component-specific authentication)
//  4. If component has no auth config:
//     - Returns the provided parentAuthManager (inherits parent's authentication)
//
// This enables each nested component to optionally override authentication settings
// while defaulting to the parent's AuthManager if no override is specified.
//
// Parameters:
//   - atmosConfig: Global Atmos configuration
//   - component: Component name to check for auth config
//   - stack: Stack name
//   - parentAuthManager: AuthManager from parent level (may be nil)
//
// Returns:
//   - AuthManager to use for this component (may be new or inherited)
//   - error if component config retrieval or auth creation fails.
func resolveAuthManagerForNestedComponent(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	parentAuthManager auth.AuthManager,
) (auth.AuthManager, error) {
	// Get component configuration WITHOUT processing templates/functions.
	componentConfig, err := getComponentConfigForAuthResolution(component, stack)
	if err != nil {
		log.Debug(
			"Could not get component config for auth resolution, using parent AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
			"error", err,
		)
		return parentAuthManager, err
	}

	// Check if component has auth section defined.
	authSection, hasAuthSection := componentConfig[cfg.AuthSectionName].(map[string]any)
	if !hasAuthSection || authSection == nil {
		// Component has no auth config, inherit parent's AuthManager.
		log.Debug(
			"Component has no auth config, inheriting parent AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
		)
		return parentAuthManager, nil
	}

	// Check if component's auth config has a default identity.
	// Only create component-specific AuthManager if there's a default identity.
	// This prevents interactive selector from showing for component auth overrides.
	hasDefault := hasDefaultIdentity(authSection)
	if !hasDefault {
		log.Debug(
			"Component auth config has no default identity, inheriting parent AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
		)
		return parentAuthManager, nil
	}

	// Component has auth config with default identity, create (or reuse) a component-specific AuthManager.
	log.Debug(
		"Component has auth config with default identity, creating component-specific AuthManager",
		logKeyComponent, component,
		logKeyStack, stack,
	)

	return resolveCachedComponentAuthManager(
		atmosConfig, componentConfig, component, stack, parentAuthManager, authSection, createComponentAuthManager,
	)
}

// getComponentConfigForAuthResolution retrieves component configuration without processing
// templates or YAML functions to avoid circular dependencies.
func getComponentConfigForAuthResolution(component, stack string) (map[string]any, error) {
	componentConfig, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false, // Critical: avoid triggering template processing
		ProcessYamlFunctions: false, // Critical: avoid circular dependency with YAML functions
		Skip:                 nil,
		AuthManager:          nil, // Critical: no AuthManager yet, we're determining auth for this component
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get component config for auth resolution: %w", errUtils.ErrDescribeComponent, err)
	}
	return componentConfig, nil
}

// createComponentAuthManager merges component auth with global auth and creates a new AuthManager.
func createComponentAuthManager(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	component string,
	stack string,
	parentAuthManager auth.AuthManager,
) (auth.AuthManager, error) {
	// Merge component auth with global auth.
	mergedAuthConfig, err := auth.MergeComponentAuthFromConfig(
		&atmosConfig.Auth,
		componentConfig,
		atmosConfig,
		cfg.AuthSectionName,
	)
	if err != nil {
		log.Debug(
			"Could not merge component auth config, using parent AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
			"error", err,
		)
		return parentAuthManager, fmt.Errorf("%w: failed to merge component auth config: %w", errUtils.ErrAuthManager, err)
	}

	// Determine identity to use for component authentication.
	// If parent AuthManager exists and is authenticated, inherit its identity.
	// This ensures that when user explicitly specifies --identity flag, it propagates to nested components.
	var identityName string
	if parentAuthManager != nil {
		chain := parentAuthManager.GetChain()
		if len(chain) > 0 {
			// Last element in chain is the authenticated identity.
			identityName = chain[len(chain)-1]
			log.Debug(
				"Inheriting identity from parent AuthManager for component",
				logKeyComponent, component,
				logKeyStack, stack,
				"inheritedIdentity", identityName,
				"chain", chain,
			)
		}
	}

	// Create and authenticate new AuthManager with merged config.
	// Use inherited identity from parent, or empty string to auto-detect from component's defaults.
	// Use the stack-aware variant so the target component's stack is threaded into manager
	// construction: stack-scoped identities (e.g. kind: <target>/emulator) need it to resolve
	// their endpoint and populate the in-process auth context read by `!terraform.state`.
	componentAuthManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfigForStack(
		identityName,     // Inherited from parent, or empty to trigger auto-detection
		mergedAuthConfig, // Merged component + global auth
		cfg.IdentityFlagSelectValue,
		atmosConfig, // Enable stack-level auth default loading
		stack,       // Target component's stack, for stack-scoped (emulator) identities
	)
	if err != nil {
		log.Debug(
			"Auth does not exist for the component, using parent AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
			"error", err,
		)
		return parentAuthManager, fmt.Errorf("%w: failed to create component-specific AuthManager: %w", errUtils.ErrAuthConsole, err)
	}

	// Successfully created component-specific AuthManager.
	if componentAuthManager != nil {
		chain := componentAuthManager.GetChain()
		log.Debug(
			"Created component-specific AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
			"identityChain", chain,
		)
	}

	return componentAuthManager, nil
}
