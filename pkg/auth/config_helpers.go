package auth

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CopyGlobalAuthConfig creates a deep copy of global auth config.
// Copies all fields: providers, identities, logs, keyring, and identity case map.
func CopyGlobalAuthConfig(globalAuth *schema.AuthConfig) *schema.AuthConfig {
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

// AuthConfigToMap converts AuthConfig struct to map[string]any for deep merging.
// Uses mapstructure to convert struct fields according to mapstructure tags.
func AuthConfigToMap(authConfig *schema.AuthConfig) (map[string]any, error) {
	if authConfig == nil {
		return make(map[string]any), nil
	}

	var result map[string]any
	if err := mapstructure.Decode(authConfig, &result); err != nil {
		return nil, fmt.Errorf("%w: auth config struct to map: %w", errUtils.ErrEncode, err)
	}

	return result, nil
}

// MergeComponentAuthConfig merges component-level auth config with global auth config.
// Returns the merged AuthConfig with component overrides applied.
func MergeComponentAuthConfig(
	atmosConfig *schema.AtmosConfiguration,
	globalAuthConfig *schema.AuthConfig,
	componentAuthSection map[string]any,
) (*schema.AuthConfig, error) {
	// Convert global auth config to map for deep merging.
	globalAuthMap, err := AuthConfigToMap(globalAuthConfig)
	if err != nil {
		return nil, err
	}

	// Deep merge global and component auth configs.
	// Component config takes precedence and can override parts of identities/providers (not just whole objects).
	mergedMap, err := merge.Merge(atmosConfig, []map[string]any{globalAuthMap, componentAuthSection})
	if err != nil {
		return nil, fmt.Errorf("%w: global and component auth configs: %w", errUtils.ErrMerge, err)
	}

	// Convert merged map back to AuthConfig struct.
	var finalAuthConfig schema.AuthConfig
	if err := mapstructure.Decode(mergedMap, &finalAuthConfig); err != nil {
		return nil, fmt.Errorf("%w: auth config map to struct: %w", errUtils.ErrDecode, err)
	}

	// Preserve IdentityCaseMap from global config.
	// IdentityCaseMap is runtime metadata that gets lost during map conversion
	// because it has mapstructure:"-" tag. We need to preserve it from the global config.
	if globalAuthConfig.IdentityCaseMap != nil {
		finalAuthConfig.IdentityCaseMap = globalAuthConfig.IdentityCaseMap
	}

	return &finalAuthConfig, nil
}

// MergeComponentAuthFromConfig merges component-specific auth config from component configuration
// with global auth config. This allows components to define their own auth identities and defaults
// in stack configurations.
//
// Parameters:
//   - globalAuth: Global auth configuration from atmos.yaml
//   - componentConfig: The full component configuration map (from ExecuteDescribeComponent or similar)
//   - atmosConfig: AtmosConfiguration for merge settings
//   - authSectionName: The name of the auth section in component config (typically "auth")
//
// Returns:
//   - Merged AuthConfig with component overrides applied
//   - Global auth config if no component auth section found
func MergeComponentAuthFromConfig(
	globalAuth *schema.AuthConfig,
	componentConfig map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	authSectionName string,
) (*schema.AuthConfig, error) {
	// Start with a copy of global auth config.
	mergedAuthConfig := CopyGlobalAuthConfig(globalAuth)

	// Return global config if no component config provided.
	if componentConfig == nil {
		return mergedAuthConfig, nil
	}

	// Extract auth section from component config.
	componentAuthSection, ok := componentConfig[authSectionName].(map[string]any)
	if !ok || componentAuthSection == nil {
		return mergedAuthConfig, nil
	}

	// Merge component auth with global auth.
	return MergeComponentAuthConfig(atmosConfig, mergedAuthConfig, componentAuthSection)
}
