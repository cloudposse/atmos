package terraform

import (
	"context"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/generator/providers"
	"github.com/cloudposse/atmos/pkg/generator/required_providers"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfbackend "github.com/cloudposse/atmos/pkg/terraform/backend"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// GenerateConfigFiles generates backend, custom, provider, and version configuration.
func (s *Operations) GenerateConfigFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "terraform.Operations.GenerateConfigFiles")()

	if err := GenerateBackendConfig(atmosConfig, info, workingDir); err != nil {
		return err
	}
	if err := s.host.GenerateFilesForComponent(atmosConfig, info, workingDir); err != nil {
		return err
	}
	if err := GenerateProviderOverrides(atmosConfig, info, workingDir); err != nil {
		return err
	}
	// Generate required_providers (terraform_override.tf.json) for version pinning (DEV-3124).
	return GenerateRequiredProviders(atmosConfig, info, workingDir)
}

// GenerateBackendConfig delegates backend-file generation using the resolved configuration and working directory.
func GenerateBackendConfig(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "terraform.GenerateBackendConfig")()

	return tfbackend.GenerateFile(atmosConfig, info, workingDir)
}

// GenerateProviderOverrides writes `providers_override.tf.json` from the component's `providers`
// section, and removes a previously generated file when that section is no longer configured.
func GenerateProviderOverrides(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "terraform.GenerateProviderOverrides")()

	// Let registered provider-config contributors (e.g. an emulator binding) deep-merge
	// provider fragments UNDER the explicit `providers:` section before generation.
	genCtx := generator.NewGeneratorContext(atmosConfig, info, workingDir)
	if merged, err := generator.ApplyProviderContributors(context.Background(), genCtx); err != nil {
		return err
	} else if merged != nil {
		info.ComponentProvidersSection = merged
	}

	// Generate `providers_override.tf.json` file if the `providers` section is configured.
	if len(info.ComponentProvidersSection) > 0 {
		providerOverrideFileName := filepath.Join(workingDir, providers.DefaultFilenameConst)

		log.Debug("Writing the provider overrides to file.", "file", providerOverrideFileName)

		if !info.DryRun {
			providerOverrides := map[string]any{"provider": tfoutput.ProcessProviderAliases(info.ComponentProvidersSection)}
			err := u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, planFileMode)
			return err
		}
	} else if !info.DryRun {
		// Remove a stale file, otherwise an earlier stack's providers apply to this one.
		return generator.RemoveGenerated(workingDir, providers.DefaultFilenameConst)
	}
	return nil
}

// GenerateRequiredProviders generates the terraform_override.tf.json file with required_version
// and required_providers blocks from stack configuration (DEV-3124).
func GenerateRequiredProviders(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "exec.GenerateRequiredProviders")()

	// Skip if no required_version or required_providers configured, removing a stale file
	// so an earlier stack's version pins don't apply to this one.
	if info.RequiredVersion == "" && len(info.RequiredProviders) == 0 {
		if info.DryRun {
			return nil
		}
		return generator.RemoveGenerated(workingDir, required_providers.DefaultFilenameConst)
	}

	requiredProvidersFileName := filepath.Join(workingDir, required_providers.DefaultFilenameConst)

	log.Debug("Writing the required_providers to file.", "file", requiredProvidersFileName)

	if info.DryRun {
		return nil
	}

	// Create generator context.
	genCtx := generator.NewGeneratorContext(atmosConfig, info, workingDir)

	// Generate and write using the generator package.
	return generator.Generate(context.Background(), required_providers.Name, genCtx, generator.NewFileWriter())
}
