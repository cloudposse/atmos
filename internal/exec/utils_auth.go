package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
