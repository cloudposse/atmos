package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

const generatedPathErrorFormat = "%w: %s"

var (
	errGeneratedPath                 = errors.New("unsupported generated path")
	errDuplicateGeneratedPath        = errors.New("duplicate generated path")
	errBackendGeneratedPath          = errors.New("unsupported backend generated path")
	errDuplicateBackendGeneratedPath = errors.New("duplicate backend generated path")
)

func generatedFiles(ctx context.Context, logger log.Logger, cfg *config.TerragruntConfig, result *model.Resolution) error {
	result.GeneratedPolicies = make(map[string]string)
	for _, generated := range cfg.GenerateConfigs {
		if generated.Disable && (generated.IfDisabledStr == "" || generated.IfDisabledStr == "skip") {
			continue
		}
		if !filepath.IsLocal(generated.Path) || generated.Path == "." {
			return fmt.Errorf(generatedPathErrorFormat, errGeneratedPath, generated.Path)
		}
		path := filepath.Clean(generated.Path)
		if _, exists := result.GeneratedPolicies[path]; exists {
			return fmt.Errorf(generatedPathErrorFormat, errDuplicateGeneratedPath, generated.Path)
		}
		policy := generated.IfExistsStr
		if generated.Disable {
			policy = generated.IfDisabledStr
			result.Generated[path] = ""
		} else {
			content, err := atmosbridge.RenderGenerated(ctx, logger, &generated)
			if err != nil {
				return err
			}
			result.Generated[path] = string(content)
		}
		result.GeneratedPolicies[path] = policy
	}
	return generatedBackend(ctx, logger, cfg, result)
}

func generatedBackend(ctx context.Context, logger log.Logger, cfg *config.TerragruntConfig, result *model.Resolution) error {
	if cfg.RemoteState == nil || cfg.RemoteState.Generate == nil {
		return nil
	}
	generated := cfg.RemoteState.Generate
	if !filepath.IsLocal(generated.Path) || generated.Path == "." {
		return fmt.Errorf(generatedPathErrorFormat, errBackendGeneratedPath, generated.Path)
	}
	path := filepath.Clean(generated.Path)
	if _, exists := result.GeneratedPolicies[path]; exists {
		return fmt.Errorf(generatedPathErrorFormat, errDuplicateBackendGeneratedPath, generated.Path)
	}
	content, err := atmosbridge.RenderBackend(ctx, logger, cfg.RemoteState.Config, result.Backend)
	if err != nil {
		return err
	}
	result.Generated[path] = string(content)
	result.GeneratedPolicies[path] = generated.IfExists
	// Preserving an existing backend could initialize a different state than the recorded identity.
	// Reject collisions rather than guessing the identity of arbitrary existing backend HCL.
	if generated.IfExists == "skip" {
		result.GeneratedPolicies[path] = "error"
	}
	result.BackendGenerated = true
	return nil
}
