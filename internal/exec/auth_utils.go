package exec

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetComponentAuthConfig extracts component-specific auth config from stack and merges it with global auth config.
// This allows components to define their own auth identities and defaults in stack configurations.
// Returns the merged auth config that should be used for authentication.
func GetComponentAuthConfig(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
) (*schema.AuthConfig, error) {
	defer perf.Track(atmosConfig, "exec.GetComponentAuthConfig")()

	// Start with global auth config from atmos.yaml.
	mergedAuthConfig := copyGlobalAuthConfig(&atmosConfig.Auth)

	// Return global config if no stack specified (e.g., auth commands).
	if stack == "" || component == "" {
		return mergedAuthConfig, nil
	}

	// Use ExecuteDescribeComponent to get the component configuration from the correct stack.
	// This reuses existing logic for stack matching (name_pattern, name_template) without reimplementing it.
	// We use AuthManager: nil and ProcessYamlFunctions: false to avoid circular dependency
	// (we need auth config to create AuthManager, but ExecuteDescribeComponent might need AuthManager for YAML functions).
	componentConfig, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false, // Critical: avoid circular dependency with YAML functions that need auth.
		Skip:                 nil,
		AuthManager:          nil, // Critical: no AuthManager yet, we're determining which identity to use.
	})
	if err != nil {
		// Component not found or error - return global config.
		return mergedAuthConfig, nil
	}

	// Extract and merge component auth config.
	componentAuthSection, ok := componentConfig[cfg.AuthSectionName].(map[string]any)
	if !ok || componentAuthSection == nil {
		return mergedAuthConfig, nil
	}

	return mergeComponentAuthConfig(atmosConfig, mergedAuthConfig, componentAuthSection)
}

// mergeComponentAuthConfig merges component-level auth config with global auth config.
// Returns the merged AuthConfig with component overrides applied.
func mergeComponentAuthConfig(
	atmosConfig *schema.AtmosConfiguration,
	globalAuthConfig *schema.AuthConfig,
	componentAuthSection map[string]any,
) (*schema.AuthConfig, error) {
	// Convert global auth config to map for deep merging.
	globalAuthMap, err := authConfigToMap(globalAuthConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrConvertComponentAuth, err)
	}

	// Deep merge global and component auth configs.
	// Component config takes precedence and can override parts of identities/providers (not just whole objects).
	mergedMap, err := merge.Merge(atmosConfig, []map[string]any{globalAuthMap, componentAuthSection})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrMergeComponentAuth, err)
	}

	// Convert merged map back to AuthConfig struct.
	var finalAuthConfig schema.AuthConfig
	if err := mapstructure.Decode(mergedMap, &finalAuthConfig); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrDecodeComponentAuth, err)
	}

	// Preserve IdentityCaseMap from global config.
	// IdentityCaseMap is runtime metadata that gets lost during map conversion
	// because it has mapstructure:"-" tag. We need to preserve it from the global config.
	if globalAuthConfig.IdentityCaseMap != nil {
		finalAuthConfig.IdentityCaseMap = globalAuthConfig.IdentityCaseMap
	}

	return &finalAuthConfig, nil
}

// copyGlobalAuthConfig creates a deep copy of global auth config.
// Copies all fields: providers, identities, logs, keyring, and identity case map.
func copyGlobalAuthConfig(globalAuth *schema.AuthConfig) *schema.AuthConfig {
	if globalAuth == nil {
		return &schema.AuthConfig{}
	}

	config := &schema.AuthConfig{
		Logs:    globalAuth.Logs,
		Keyring: globalAuth.Keyring,
	}

	if globalAuth.Providers != nil {
		config.Providers = make(map[string]schema.Provider, len(globalAuth.Providers))
		for k := range globalAuth.Providers {
			config.Providers[k] = globalAuth.Providers[k]
		}
	}

	if globalAuth.Identities != nil {
		config.Identities = make(map[string]schema.Identity, len(globalAuth.Identities))
		for k := range globalAuth.Identities {
			config.Identities[k] = globalAuth.Identities[k]
		}
	}

	if globalAuth.IdentityCaseMap != nil {
		config.IdentityCaseMap = make(map[string]string, len(globalAuth.IdentityCaseMap))
		for k, v := range globalAuth.IdentityCaseMap {
			config.IdentityCaseMap[k] = v
		}
	}

	return config
}

// authConfigToMap converts AuthConfig struct to map[string]any for deep merging.
// Uses mapstructure to convert struct fields according to mapstructure tags.
func authConfigToMap(authConfig *schema.AuthConfig) (map[string]any, error) {
	if authConfig == nil {
		return make(map[string]any), nil
	}

	var result map[string]any
	if err := mapstructure.Decode(authConfig, &result); err != nil {
		return nil, err
	}

	return result, nil
}
