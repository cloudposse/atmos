// Package backend generates Terraform backend configuration files.
package backend

import (
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Config builds Terraform backend configuration, including Terraform Cloud workspace tokens.
func Config(backendType string, backendConfig map[string]any, terraformWorkspace string) (map[string]any, error) {
	defer perf.Track(nil, "backend.Config")()

	// Validate that backendType is not empty to avoid generating invalid backend config.
	// An empty backendType would result in invalid JSON like: {"terraform": {"backend": {"": {}}}}.
	if backendType == "" {
		return nil, errUtils.ErrBackendTypeRequired
	}

	// Generate backend config file for Terraform Cloud.
	// https://developer.hashicorp.com/terraform/cli/cloud/settings
	if backendType == "cloud" {
		backendConfigFinal, err := cloudConfig(backendConfig, terraformWorkspace)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"terraform": map[string]any{
				"cloud": backendConfigFinal,
			},
		}, nil
	}

	// Generate backend config file for all other Terraform backends.
	return map[string]any{
		"terraform": map[string]any{
			"backend": map[string]any{
				backendType: backendConfig,
			},
		},
	}, nil
}

// cloudConfig processes workspace template tokens in Terraform Cloud configuration.
func cloudConfig(backendConfig map[string]any, terraformWorkspace string) (map[string]any, error) {
	if terraformWorkspace == "" {
		return backendConfig, nil
	}
	// Process template tokens in the backend config.
	backendConfigStr, err := u.ConvertToYAML(backendConfig)
	if err != nil {
		return nil, err
	}
	ctx := schema.Context{TerraformWorkspace: terraformWorkspace}
	backendConfigStrReplaced := cfg.ReplaceContextTokens(ctx, backendConfigStr)
	return u.UnmarshalYAML[schema.AtmosSectionMapType](backendConfigStrReplaced)
}
