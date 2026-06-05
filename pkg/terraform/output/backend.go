package output

import (
	"maps"
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
//
// When an aliased entry (e.g., "aws.use1") does not explicitly set the `alias` field
// inside its configuration block, Atmos automatically derives it from the portion of
// the key after the first dot. An explicit `alias` value always wins — Atmos will not
// overwrite it. Non-map configuration values are passed through unchanged.
//
// The input map is not mutated; aliased blocks are cloned before being modified.
func ProcessProviderAliases(providerOverrides map[string]any) map[string]any {
	defer perf.Track(nil, "output.ProcessProviderAliases")()

	if len(providerOverrides) == 0 {
		return providerOverrides
	}

	baseNamesWithAliases := collectAliasedBaseNames(providerOverrides)

	// If no aliased providers, return as-is.
	if len(baseNamesWithAliases) == 0 {
		return providerOverrides
	}

	result := make(map[string]any, len(providerOverrides))

	// Build arrays for base providers that have aliases.
	for baseName := range baseNamesWithAliases {
		result[baseName] = buildAliasedProviderConfigs(baseName, providerOverrides)
	}

	// Copy over providers that are neither dot-notation keys nor bases with aliases.
	copyNonAliasedProviders(result, providerOverrides, baseNamesWithAliases)

	return result
}

// collectAliasedBaseNames returns the set of base provider names (prefix before
// the first dot) that have at least one dot-notation aliased entry.
func collectAliasedBaseNames(providerOverrides map[string]any) map[string]bool {
	baseNames := make(map[string]bool)
	for key := range providerOverrides {
		if idx := strings.Index(key, "."); idx > 0 {
			baseNames[key[:idx]] = true
		}
	}
	return baseNames
}

// buildAliasedProviderConfigs returns the ordered slice of provider blocks for
// a base provider that has aliases: the bare base block first (if present),
// followed by alias blocks in sorted key order. Each alias block has its
// `alias` field auto-derived from the key suffix when not set explicitly.
func buildAliasedProviderConfigs(baseName string, providerOverrides map[string]any) []any {
	var configs []any

	// Add the base config first (if it exists).
	if config, exists := providerOverrides[baseName]; exists {
		configs = append(configs, config)
	}

	// Collect aliased keys for this base provider and sort for deterministic output.
	var aliasedKeys []string
	prefix := baseName + "."
	for key := range providerOverrides {
		if strings.HasPrefix(key, prefix) {
			aliasedKeys = append(aliasedKeys, key)
		}
	}
	sort.Strings(aliasedKeys)

	for _, key := range aliasedKeys {
		// Derive alias from the suffix after the first dot (e.g., "aws.use1" -> "use1").
		aliasName := key[len(prefix):]
		configs = append(configs, withDerivedAlias(providerOverrides[key], aliasName))
	}

	return configs
}

// copyNonAliasedProviders copies every provider from src into dst that is not
// a dot-notation key and is not a base name of an aliased group (those are
// handled separately by buildAliasedProviderConfigs).
//
// Convention: any "." in a provider key is treated as the alias shorthand
// `<base>.<alias>` and is therefore always routed through the alias grouping
// (never copied verbatim). Terraform provider names don't contain dots in
// practice, but if a future provider name legitimately did, it would be folded
// into alias processing rather than surfaced as a top-level key.
func copyNonAliasedProviders(dst, src map[string]any, baseNamesWithAliases map[string]bool) {
	for key, config := range src {
		if strings.Contains(key, ".") {
			continue
		}
		if baseNamesWithAliases[key] {
			continue
		}
		dst[key] = config
	}
}

// withDerivedAlias returns the provider block with `alias` set to aliasName when
// the block is a map and does not already define `alias`. An explicit alias value
// (including an empty string) always wins. Non-map blocks are returned unchanged.
// The input block is never mutated — a shallow copy is returned when a change is made.
func withDerivedAlias(block any, aliasName string) any {
	cfg, ok := block.(map[string]any)
	if !ok {
		return block
	}
	if _, hasAlias := cfg["alias"]; hasAlias {
		return block
	}

	cloned := maps.Clone(cfg)
	cloned["alias"] = aliasName
	return cloned
}
