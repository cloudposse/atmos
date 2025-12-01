package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	logKeyComponent = "component"
	logKeyStack     = "stack"
)

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
		log.Debug("Could not get component config for auth resolution, using parent AuthManager",
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
		log.Debug("Component has no auth config, inheriting parent AuthManager",
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
		log.Debug("Component auth config has no default identity, inheriting parent AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
		)
		return parentAuthManager, nil
	}

	// Component has auth config with default identity, create component-specific AuthManager.
	log.Debug("Component has auth config with default identity, creating component-specific AuthManager",
		logKeyComponent, component,
		logKeyStack, stack,
	)

	return createComponentAuthManager(atmosConfig, componentConfig, component, stack, parentAuthManager)
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
		log.Debug("Could not merge component auth config, using parent AuthManager",
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
			log.Debug("Inheriting identity from parent AuthManager for component",
				logKeyComponent, component,
				logKeyStack, stack,
				"inheritedIdentity", identityName,
				"chain", chain,
			)
		}
	}

	// Create and authenticate new AuthManager with merged config.
	// Use inherited identity from parent, or empty string to auto-detect from component's defaults.
	componentAuthManager, err := auth.CreateAndAuthenticateManager(
		identityName,     // Inherited from parent, or empty to trigger auto-detection
		mergedAuthConfig, // Merged component + global auth
		cfg.IdentityFlagSelectValue,
	)
	if err != nil {
		log.Debug("Auth does not exist for the component, using parent AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
			"error", err,
		)
		return parentAuthManager, fmt.Errorf("%w: failed to create component-specific AuthManager: %w", errUtils.ErrAuthConsole, err)
	}

	// Successfully created component-specific AuthManager.
	if componentAuthManager != nil {
		chain := componentAuthManager.GetChain()
		log.Debug("Created component-specific AuthManager",
			logKeyComponent, component,
			logKeyStack, stack,
			"identityChain", chain,
		)
	}

	return componentAuthManager, nil
}
