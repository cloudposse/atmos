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
	if componentAuthSection, ok := componentConfig[cfg.AuthSectionName].(map[string]any); ok && componentAuthSection != nil {
		// Convert global auth config to map for deep merging.
		globalAuthMap, err := authConfigToMap(mergedAuthConfig)
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

		return &finalAuthConfig, nil
	}

	return mergedAuthConfig, nil
}

// copyGlobalAuthConfig creates a copy of global auth config.
func copyGlobalAuthConfig(globalAuth *schema.AuthConfig) *schema.AuthConfig {
	config := &schema.AuthConfig{}
	if globalAuth.Identities != nil {
		config.Identities = make(map[string]schema.Identity)
		for k, v := range globalAuth.Identities {
			config.Identities[k] = v
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
