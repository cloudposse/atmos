package output

import (
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
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
// Note: workspace parameter is reserved for future Terraform Cloud backend support.
func generateBackendConfig(backendType string, backendConfig map[string]any, _ string, _ *schema.AuthContext) (map[string]any, error) {
	defer perf.Track(nil, "output.generateBackendConfig")()

	// Validate that backendType is not empty.
	if backendType == "" {
		return nil, errUtils.ErrBackendTypeRequired
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
		"provider": providerOverrides,
	}
}
