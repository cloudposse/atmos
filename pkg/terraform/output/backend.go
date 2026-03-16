package output

import (
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// filePermission is the standard file permission for generated terraform files.
const filePermission = 0o644

// BackendGenerator handles backend and provider file generation.
type BackendGenerator interface {
	// GenerateBackendIfNeeded generates backend.tf.json if auto-generation is enabled.
	GenerateBackendIfNeeded(config *ComponentConfig, component, stack string, authContext *schema.AuthContext) error
	// GenerateProvidersIfNeeded generates providers_override.tf.json if providers are configured.
	GenerateProvidersIfNeeded(config *ComponentConfig, authContext *schema.AuthContext) error
}

// defaultBackendGenerator is the default implementation of BackendGenerator.
type defaultBackendGenerator struct{}

// GenerateBackendIfNeeded generates backend.tf.json if auto-generation is enabled.
func (g *defaultBackendGenerator) GenerateBackendIfNeeded(config *ComponentConfig, component, stack string, authContext *schema.AuthContext) error {
	defer perf.Track(nil, "output.defaultBackendGenerator.GenerateBackendIfNeeded")()

	if !config.AutoGenerateBackend {
		return nil
	}

	// Validate backend configuration.
	if err := ValidateBackendConfig(config, component, stack); err != nil {
		return err
	}

	backendFileName := filepath.Join(config.ComponentPath, "backend.tf.json")
	log.Debug("Writing backend config", "file", backendFileName)

	backendConfig, err := generateBackendConfig(config.BackendType, config.Backend, config.Workspace, authContext)
	if err != nil {
		return errUtils.Build(errUtils.ErrBackendFileGeneration).
			WithCause(err).
			WithExplanationf("Failed to generate backend for %s.", GetComponentInfo(component, stack)).
			Err()
	}

	if err := u.WriteToFileAsJSON(backendFileName, backendConfig, filePermission); err != nil {
		return errUtils.Build(errUtils.ErrBackendFileGeneration).
			WithCause(err).
			WithExplanationf("Failed to write backend file for %s.", GetComponentInfo(component, stack)).
			Err()
	}

	log.Debug("Wrote backend config", "file", backendFileName)
	return nil
}

// GenerateProvidersIfNeeded generates providers_override.tf.json if providers are configured.
func (g *defaultBackendGenerator) GenerateProvidersIfNeeded(config *ComponentConfig, authContext *schema.AuthContext) error {
	defer perf.Track(nil, "output.defaultBackendGenerator.GenerateProvidersIfNeeded")()

	if len(config.Providers) == 0 {
		return nil
	}

	providerFileName := filepath.Join(config.ComponentPath, "providers_override.tf.json")
	log.Debug("Writing provider overrides", "file", providerFileName)

	providerOverrides := generateProviderOverrides(config.Providers, authContext)
	if err := u.WriteToFileAsJSON(providerFileName, providerOverrides, filePermission); err != nil {
		return errUtils.Build(errUtils.ErrProviderFileGeneration).
			WithCause(err).
			WithExplanationf("Failed to write provider override file to %s.", providerFileName).
			Err()
	}

	log.Debug("Wrote provider overrides", "file", providerFileName)
	return nil
}

// generateBackendConfig generates the backend configuration for terraform.
// This matches the logic in internal/exec/utils.go:generateComponentBackendConfig.
// Supports Terraform Cloud backend with workspace token replacement.
func generateBackendConfig(backendType string, backendConfig map[string]any, terraformWorkspace string, _ *schema.AuthContext) (map[string]any, error) {
	defer perf.Track(nil, "output.generateBackendConfig")()

	if backendType == "" {
		return nil, errUtils.ErrBackendTypeRequired
	}

	if backendType == "cloud" {
		backendConfigFinal := backendConfig

		if terraformWorkspace != "" {
			// Process template tokens in the backend config (e.g., {terraform_workspace}).
			backendConfigStr, err := u.ConvertToYAML(backendConfig)
			if err != nil {
				return nil, err
			}

			ctx := schema.Context{
				TerraformWorkspace: terraformWorkspace,
			}

			backendConfigStrReplaced := cfg.ReplaceContextTokens(ctx, backendConfigStr)

			backendConfigFinal, err = u.UnmarshalYAML[schema.AtmosSectionMapType](backendConfigStrReplaced)
			if err != nil {
				return nil, err
			}
		}

		return map[string]any{
			"terraform": map[string]any{
				"cloud": backendConfigFinal,
			},
		}, nil
	}

	return map[string]any{
		"terraform": map[string]any{
			"backend": map[string]any{
				backendType: backendConfig,
			},
		},
	}, nil
}

// generateProviderOverrides generates the provider override configuration.
// This matches the logic in internal/exec/utils.go:generateComponentProviderOverrides.
func generateProviderOverrides(providerOverrides map[string]any, _ *schema.AuthContext) map[string]any {
	defer perf.Track(nil, "output.generateProviderOverrides")()

	return map[string]any{
		"provider": ProcessProviderAliases(providerOverrides),
	}
}

// ProcessProviderAliases groups providers that use dot notation (e.g., "aws.alias") into arrays.
// This converts the Atmos shorthand notation into the Terraform JSON provider format.
// For example, {"aws": {...}, "aws.use1": {...}} becomes {"aws": [{...}, {...}]}.
func ProcessProviderAliases(providerOverrides map[string]any) map[string]any {
	if len(providerOverrides) == 0 {
		return providerOverrides
	}

	// Find all base names that have aliased configurations (with dot notation).
	baseNamesWithAliases := make(map[string]bool)
	for key := range providerOverrides {
		if idx := strings.Index(key, "."); idx > 0 {
			baseName := key[:idx]
			baseNamesWithAliases[baseName] = true
		}
	}

	// If no aliased providers, return as-is.
	if len(baseNamesWithAliases) == 0 {
		return providerOverrides
	}

	result := make(map[string]any, len(providerOverrides))

	// Build arrays for base providers that have aliases.
	for baseName := range baseNamesWithAliases {
		var configs []any

		// Add the base config first (if it exists).
		if config, exists := providerOverrides[baseName]; exists {
			configs = append(configs, config)
		}

		// Add aliased configs in sorted order for determinism.
		var aliasedKeys []string
		for key := range providerOverrides {
			if strings.HasPrefix(key, baseName+".") {
				aliasedKeys = append(aliasedKeys, key)
			}
		}
		sort.Strings(aliasedKeys)
		for _, key := range aliasedKeys {
			configs = append(configs, providerOverrides[key])
		}

		result[baseName] = configs
	}

	// Copy over providers that don't have aliases.
	for key, config := range providerOverrides {
		// Skip dot-notation keys (already processed above).
		if strings.Contains(key, ".") {
			continue
		}
		// Skip base providers that have aliases (already processed above).
		if baseNamesWithAliases[key] {
			continue
		}
		result[key] = config
	}

	return result
}
